// pdfocr is a command-line tool for creating searchable PDFs with OCR text layers.
//
// This tool can either enhance existing PDFs with OCR text layers or create new PDFs
// from images with embedded OCR text. It uses HOCR data to position text accurately within
// the document at the exact position of each recognized word.
//
// Usage:
//
//	pdfocr -hocr document.hocr [options]
//	pdfocr -pdf document.pdf -check-ocr
//
// Required flags:
//
//	-hocr string      Path to hOCR file (required except for -check-ocr)
//	-output string    Output PDF path (required except for -check-ocr)
//
// Input options (one required):
//
//	-pdf string       Path to existing PDF to enhance with OCR
//	-image-dir string Directory containing page images to build a new PDF
//
// Processing options:
//
//	-start-page int   Start applying OCR from this page (default 1)
//	-debug            Enable debug mode (shows OCR bounding boxes)
//	-force            Force reapply OCR even if layer exists
//	-strict           Error out when OCR detection fails or OCR already exists (unless Force is used)
//	-overwrite        Overwrite output file if it exists
//	-debug-pdf        Dump PDF structure for debugging
//	-check-ocr        Check if the PDF already has OCR and exit
//
// Exit codes:
//
//	0 - Success (no warnings or errors)
//	1 - Error (operation failed)
//	2 - Success with warnings (including OCR already detected)
//	3 - Error: OCR already detected in strict mode
//
// Examples:
//
// Add OCR layer to existing PDF:
//
//	pdfocr -hocr document.hocr -pdf document.pdf -output document_searchable.pdf
//
// Create PDF from image directory with OCR:
//
//	pdfocr -hocr document.hocr -image-dir ./page_images -output document_searchable.pdf
//
// Check if a PDF already has OCR:
//
//	pdfocr -pdf document.pdf -check-ocr
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gardar/ocrchestra/pkg/pdfocr"
)

// Exit code constants for the CLI
const (
	exitSuccess          = 0 // Success with no warnings
	exitError            = 1 // Error, operation failed
	exitSuccessWithWarns = 2 // Success but with warnings
	exitStrictOCRFailure = 3 // OCR already present in strict mode
)

// warningWriter captures warnings written to the logger
type warningWriter struct {
	buf    bytes.Buffer
	target io.Writer // Usually os.Stdout
}

func newWarningWriter(target io.Writer) *warningWriter {
	return &warningWriter{
		target: target,
	}
}

func (w *warningWriter) Write(p []byte) (n int, err error) {
	// Write to both the buffer (for tracking) and the target (for display)
	w.buf.Write(p)
	return w.target.Write(p)
}

// HasWarnings checks if any warnings were logged
func (w *warningWriter) HasWarnings() bool {
	return strings.Contains(w.buf.String(), "Warning:")
}

// HasOCRWarning specifically checks if OCR already exists warning was logged
func (w *warningWriter) HasOCRWarning() bool {
	return strings.Contains(w.buf.String(), "already has OCR")
}

func main() {
	// Define command-line flags
	hocrPath := flag.String("hocr", "", "Path to a multi-page HOCR file")
	imageDirPath := flag.String("image-dir", "", "Directory containing images")
	pdfPath := flag.String("pdf", "", "Path to an existing PDF to add OCR layer to")
	pdfOcrPath := flag.String("output", "", "Output PDF path")
	startPage := flag.Int("start-page", 1, "Start applying OCR from this page number (1-based index)")
	debug := flag.Bool("debug", false, "Enable debug mode")
	force := flag.Bool("force", false, "Force reapply OCR even if an OCR layer is already detected")
	strict := flag.Bool("strict", false, "Error out when OCR detection fails or OCR already exists (unless Force is used)")
	overwriteOutput := flag.Bool("overwrite", false, "Overwrite the output PDF if it already exists")
	dumpPDF := flag.Bool("debug-pdf", false, "Dump PDF structure for debugging")
	checkOCR := flag.Bool("check-ocr", false, "Check if the PDF already has OCR and exit")

	// Update the usage to include the exit codes
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -hocr document.hocr -pdf document.pdf -output document_searchable.pdf\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -pdf document.pdf -check-ocr\n\n", os.Args[0])

		fmt.Fprintf(flag.CommandLine.Output(), "Options:\n")
		flag.PrintDefaults()

		fmt.Fprintf(flag.CommandLine.Output(), "\nExit Codes:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  %d - Success\n", exitSuccess)
		fmt.Fprintf(flag.CommandLine.Output(), "  %d - Error\n", exitError)
		fmt.Fprintf(flag.CommandLine.Output(), "  %d - Success with warnings (including OCR already detected)\n", exitSuccessWithWarns)
		fmt.Fprintf(flag.CommandLine.Output(), "  %d - Error: OCR already detected in strict mode\n", exitStrictOCRFailure)

		fmt.Fprintf(flag.CommandLine.Output(), "\nExamples:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -hocr document.hocr -pdf document.pdf -output document_searchable.pdf\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -hocr document.hocr -image-dir ./page_images -output document_searchable.pdf\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -pdf document.pdf -check-ocr\n", os.Args[0])
	}

	flag.Parse()

	// Mode for checking OCR
	if *checkOCR {
		handleCheckOCRMode(pdfPath, debug, dumpPDF)
		return // Don't proceed further
	}

	// Handle normal OCR application mode
	handleOCRApplicationMode(hocrPath, imageDirPath, pdfPath, pdfOcrPath, startPage,
		debug, force, strict, overwriteOutput, dumpPDF)
}

// handleCheckOCRMode handles the OCR detection mode
func handleCheckOCRMode(pdfPath *string, debug, dumpPDF *bool) {
	if *pdfPath == "" {
		fmt.Println("Error: Must provide -pdf for OCR checking")
		os.Exit(exitError)
	}

	// Read the input PDF
	inputData, err := os.ReadFile(*pdfPath)
	if err != nil {
		fmt.Printf("Failed to read input PDF: %v\n", err)
		os.Exit(exitError)
	}

	// Create a warning writer to capture messages
	warningCapture := newWarningWriter(os.Stdout)

	// Configure OCR detection
	config := pdfocr.DefaultConfig()
	config.Debug = *debug
	config.DumpPDF = *dumpPDF
	config.Logger = warningCapture

	// Perform OCR detection
	ocrResult, err := pdfocr.DetectOCR(inputData, config)
	if err != nil {
		fmt.Printf("Error during OCR detection: %v\n", err)
		os.Exit(exitError)
	}

	// Display the results
	fmt.Printf("OCR Detection Results for %s:\n", *pdfPath)
	fmt.Printf("Has OCR: %v\n", ocrResult.HasOCR)

	if ocrResult.HasLayerOCR && ocrResult.LayerInfo.OCRLayerName != "" {
		fmt.Printf("OCR Layer: %s\n", ocrResult.LayerInfo.OCRLayerName)
	}

	if len(ocrResult.LayerInfo.Layers) > 0 {
		fmt.Println("\nDetected Layers:")
		for i, layer := range ocrResult.LayerInfo.Layers {
			fmt.Printf("  %d. %s\n", i+1, layer)
		}
	}

	if len(ocrResult.Warnings) > 0 {
		fmt.Println("\nWarnings:")
		for _, warning := range ocrResult.Warnings {
			fmt.Printf("  - %s\n", warning)
		}
	}

	// Exit with appropriate code based on OCR detection
	if ocrResult.HasOCR {
		os.Exit(exitSuccessWithWarns)
	} else {
		os.Exit(exitSuccess)
	}
}

// handleOCRApplicationMode handles the main OCR application mode
func handleOCRApplicationMode(hocrPath, imageDirPath, pdfPath, pdfOcrPath *string, startPage *int,
	debug, force, strict, overwriteOutput, dumpPDF *bool) {

	// Validate required flags
	if *hocrPath == "" {
		fmt.Println("Error: Must provide -hocr path")
		os.Exit(exitError)
	}
	if *imageDirPath == "" && *pdfPath == "" {
		fmt.Println("Error: Must provide either -image-dir or -pdf")
		os.Exit(exitError)
	}
	if *pdfOcrPath == "" {
		fmt.Println("Error: Must provide -output path")
		os.Exit(exitError)
	}

	if _, err := os.Stat(*pdfOcrPath); err == nil {
		if !*overwriteOutput {
			fmt.Printf("Output file %s already exists. Use -overwrite to overwrite.\n", *pdfOcrPath)
			os.Exit(exitError)
		}
		os.Remove(*pdfOcrPath)
	}

	// Create a warning writer to capture warnings
	warningCapture := newWarningWriter(os.Stdout)

	// Build the OCRConfig
	config := pdfocr.DefaultConfig()
	config.Debug = *debug
	config.Force = *force
	config.Strict = *strict
	config.StartPage = *startPage
	config.DumpPDF = *dumpPDF
	config.Logger = warningCapture

	// Read and parse hOCR
	hOCR, err := os.ReadFile(*hocrPath)
	if err != nil {
		fmt.Printf("Failed to read HOCR file: %v\n", err)
		os.Exit(exitError)
	}

	// Either create a new PDF from images or modify an existing PDF
	var finalPDF []byte
	if *imageDirPath != "" {
		// Create new PDF from images
		imagePaths, err := filepath.Glob(filepath.Join(*imageDirPath, "*"))
		if err != nil {
			fmt.Printf("Error accessing image directory: %v\n", err)
			os.Exit(exitError)
		}
		sort.Strings(imagePaths)
		fmt.Printf("Found %d image files in %s\n", len(imagePaths), *imageDirPath)

		// Read all images into memory
		var imagesData [][]byte
		for _, imgPath := range imagePaths {
			imgBytes, err := os.ReadFile(imgPath)
			if err != nil {
				fmt.Printf("Failed to read image %s: %v\n", imgPath, err)
				os.Exit(exitError)
			}
			imagesData = append(imagesData, imgBytes)
		}

		// Assemble the OCR'd PDF
		finalPDF, err = pdfocr.AssembleWithOCR(hOCR, imagesData, config)
		if err != nil {
			fmt.Printf("Error creating PDF from images: %v\n", err)
			os.Exit(exitError)
		}

	} else {
		// Modify an existing PDF
		inputData, err := os.ReadFile(*pdfPath)
		if err != nil {
			fmt.Printf("Failed to read input PDF: %v\n", err)
			os.Exit(exitError)
		}

		// Apply the OCR layer to the PDF
		finalPDF, err = pdfocr.ApplyOCR(inputData, hOCR, config)
		if err != nil {
			// Special handling for OCR already detected in strict mode
			if strings.Contains(err.Error(), "already has OCR") && *strict {
				fmt.Printf("Error: %v\n", err)
				os.Exit(exitStrictOCRFailure)
			}
			fmt.Printf("Error applying OCR to existing PDF: %v\n", err)
			os.Exit(exitError)
		}
	}

	// Warning for potentially conflicting flag combinations
	if *imageDirPath != "" && *force {
		fmt.Println("Note: -force is only applicable when -pdf is set. Ignoring -force for image input.")
	}
	if *imageDirPath != "" && *strict {
		fmt.Println("Note: -strict is only applicable when -pdf is set. Ignoring -strict for image input.")
	}

	// Write final PDF to disk
	if err := os.WriteFile(*pdfOcrPath, finalPDF, 0666); err != nil {
		fmt.Printf("Failed to write output PDF: %v\n", err)
		os.Exit(exitError)
	}
	fmt.Println("✅ OCR-enhanced PDF created:", *pdfOcrPath)

	// Exit with appropriate code based on warnings
	if warningCapture.HasOCRWarning() {
		fmt.Println("Note: Completed with OCR warnings - existing OCR was detected")
		os.Exit(exitSuccessWithWarns)
	} else if warningCapture.HasWarnings() {
		fmt.Println("Note: Completed with warnings")
		os.Exit(exitSuccessWithWarns)
	} else {
		os.Exit(exitSuccess)
	}
}
