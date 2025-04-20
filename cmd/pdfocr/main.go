// pdfocr is a command-line tool for creating searchable PDFs with OCR text layers.
//
// This tool can either enhance existing PDFs with OCR text layers or create new PDFs
// from images with embedded OCR text. It uses HOCR data to position text accurately within
// the document at the exact position of each recognized word.
//
// Usage:
//
//	pdfocr -hocr document.hocr [options]
//
// Required flags:
//
//	-hocr string      Path to HOCR file
//	-output string    Output PDF path
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
//	-overwrite        Overwrite output file if it exists
//	-debug-pdf        Dump PDF structure for debugging
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
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/gardar/ocrchestra/pkg/pdfocr"
)

func main() {
	hocrPath := flag.String("hocr", "", "Path to a multi-page HOCR file")
	imageDirPath := flag.String("image-dir", "", "Directory containing images")
	pdfPath := flag.String("pdf", "", "Path to an existing PDF to add OCR layer to")
	pdfOcrPath := flag.String("output", "", "Output PDF path")
	startPage := flag.Int("start-page", 1, "Start applying OCR from this page number (1-based index)")
	debug := flag.Bool("debug", false, "Enable debug mode")
	force := flag.Bool("force", false, "Force reapply OCR even if an OCR layer is already detected")
	overwriteOutput := flag.Bool("overwrite", false, "Overwrite the output PDF if it already exists")
	dumpPDF := flag.Bool("debug-pdf", false, "Dump PDF structure for debugging")
	flag.Parse()

	if *hocrPath == "" {
		fmt.Println("Error: Must provide -hocr path")
		os.Exit(1)
	}
	if *imageDirPath == "" && *pdfPath == "" {
		fmt.Println("Error: Must provide either -image-dir or -input-pdf")
		os.Exit(1)
	}

	if _, err := os.Stat(*pdfOcrPath); err == nil {
		if !*overwriteOutput {
			fmt.Printf("Output file %s already exists. Use -overwrite to overwrite.\n", *pdfOcrPath)
			os.Exit(1)
		}
		os.Remove(*pdfOcrPath)
	}

	// Build the OCRConfig
	config := pdfocr.OCRConfig{
		Debug:     *debug,
		Force:     *force,
		StartPage: *startPage,
		DumpPDF:   *dumpPDF,
		Font:      pdfocr.DefaultFont,
	}

	// Read and parse hOCR
	hOCR, err := os.ReadFile(*hocrPath)
	if err != nil {
		fmt.Printf("Failed to read HOCR file: %v\n", err)
		os.Exit(1)
	}

	// Either create a new PDF from images or modify an existing PDF
	var finalPDF []byte
	if *imageDirPath != "" {
		// Create new PDF from images
		imagePaths, err := filepath.Glob(filepath.Join(*imageDirPath, "*"))
		if err != nil {
			fmt.Printf("Error accessing image directory: %v\n", err)
			os.Exit(1)
		}
		sort.Strings(imagePaths)
		fmt.Printf("Found %d image files in %s\n", len(imagePaths), *imageDirPath)

		// Read all images into memory
		var imagesData [][]byte
		for _, imgPath := range imagePaths {
			imgBytes, err := os.ReadFile(imgPath)
			if err != nil {
				fmt.Printf("Failed to read image %s: %v\n", imgPath, err)
				os.Exit(1)
			}
			imagesData = append(imagesData, imgBytes)
		}

		// Assemble the OCR'd PDF
		finalPDF, err = pdfocr.AssembleWithOCR(hOCR, imagesData, config)
		if err != nil {
			fmt.Printf("Error creating PDF from images: %v\n", err)
			os.Exit(1)
		}

	} else {
		// Modify an existing PDF
		inputData, err := os.ReadFile(*pdfPath)
		if err != nil {
			fmt.Printf("Failed to read input PDF: %v\n", err)
			os.Exit(1)
		}

		// Apply the OCR layer to the PDF
		finalPDF, err = pdfocr.ApplyOCR(inputData, hOCR, config)
		if err != nil {
			fmt.Printf("Error applying OCR to existing PDF: %v\n", err)
			os.Exit(1)
		}
	}

	if *imageDirPath != "" && *force {
		fmt.Println("Warning: -force is only applicable when -pdf is set. Ignoring -force.")
	}

	// Write final PDF to disk
	if err := os.WriteFile(*pdfOcrPath, finalPDF, 0666); err != nil {
		fmt.Printf("Failed to write output PDF: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ OCR-enhanced PDF created:", *pdfOcrPath)
}
