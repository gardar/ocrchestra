// Package pdfocr provides functionality for adding OCR text layers from hOCR to PDF documents.
//
// This package enables creating OCR enhanced searchable PDFs either by modifying existing PDFs or
// by assembling new PDFs from images with an OCR text layer. It works with hOCR data
// (either raw HTML or parsed structures) to position text accurately within the PDF.
//
// The resulting PDFs have OCR text precisely positioned over the original document. This text is:
// - Fully searchable
// - Selectable with mouse drag operations
// - Can be toggled on/off in compatible PDF readers, allowing users to view just the OCR layer
//
// Key Features:
//
// - Apply OCR text layers to existing PDFs, making them searchable and text selectable
// - Create new PDFs from images with OCR text layers
// - Detect existing OCR layers to prevent duplication
// - Position text with precise bounding boxes matching the original content
//
// Main Functions:
//
// - ApplyOCR: Adds OCR text layer to an existing PDF
// - AssembleWithOCR: Creates a new PDF from images with OCR text layer
package pdfocr

import (
	"fmt"

	"github.com/gardar/ocrchestra/pkg/hocr"
)

// AssembleWithOCR is a high-level function for creating a PDF from images
// and applying the HOCR text overlay.
// It accepts either raw HOCR data ([]byte) or a parsed HOCR struct (*hocr.HOCR).
func AssembleWithOCR(
	hocrInput interface{},
	imagesData [][]byte,
	config OCRConfig,
) ([]byte, error) {
	// Handle different input types for HOCR
	var hocrStruct hocr.HOCR
	var err error

	switch h := hocrInput.(type) {
	case []byte:
		// Parse raw HOCR data
		hocrStruct, err = hocr.ParseHOCR(h)
		if err != nil {
			return nil, fmt.Errorf("failed to parse HOCR data: %w", err)
		}
	case *hocr.HOCR:
		// Use the provided struct directly
		if h == nil {
			return nil, fmt.Errorf("HOCR struct is nil")
		}
		hocrStruct = *h
	default:
		return nil, fmt.Errorf("unsupported HOCR input type: %T", hocrInput)
	}

	// Validate inputs
	if len(hocrStruct.Pages) == 0 {
		return nil, fmt.Errorf("HOCR data contains no pages")
	}

	if len(imagesData) == 0 {
		return nil, fmt.Errorf("no image data provided")
	}
	if config.StartPage < 1 {
		return nil, fmt.Errorf("start page must be at least 1, got %d", config.StartPage)
	}

	// Check if we have enough images for hOCR pages
	if len(imagesData) < len(hocrStruct.Pages) {
		return nil, fmt.Errorf("not enough images (%d) for HOCR pages (%d)",
			len(imagesData), len(hocrStruct.Pages))
	}

	// Validate image formats
	for i, imgData := range imagesData {
		if len(imgData) == 0 {
			return nil, fmt.Errorf("image %d is empty", i+1)
		}
		imageType, err := detectImageType(imgData)
		if err != nil {
			return nil, fmt.Errorf("image %d has invalid format: %w", i+1, err)
		}
		if config.Debug {
			fmt.Printf("Image %d is of type: %s\n", i+1, imageType)
		}
	}

	// Build the PDF from images
	finalPDF, err := createPDFFromImage(
		hocrStruct,
		imagesData,
		config.StartPage,
		config.Debug,
		config.LayerName,
		config.Font,
	)
	if err != nil {
		return nil, fmt.Errorf("error creating PDF from images: %w", err)
	}
	return finalPDF, nil
}

// ApplyOCR is a high-level function for taking an existing PDF and applying hOCR overlays.
// It performs validation and safety checks.
// It accepts either raw hOCR data ([]byte) or a parsed hOCR struct (*hocr.HOCR).
func ApplyOCR(
	inputPDFData []byte,
	hocrInput interface{},
	config OCRConfig,
) ([]byte, error) {
	// Handle different input types for hOCR
	var hocrStruct hocr.HOCR
	var err error

	switch h := hocrInput.(type) {
	case []byte:
		// Parse raw hOCR data
		hocrStruct, err = hocr.ParseHOCR(h)
		if err != nil {
			return nil, fmt.Errorf("failed to parse HOCR data: %w", err)
		}
	case *hocr.HOCR:
		// Use the provided struct directly
		if h == nil {
			return nil, fmt.Errorf("HOCR struct is nil")
		}
		hocrStruct = *h
	default:
		return nil, fmt.Errorf("unsupported HOCR input type: %T", hocrInput)
	}

	// Validate inputs
	if len(inputPDFData) == 0 {
		return nil, fmt.Errorf("input PDF data is empty")
	}
	if len(hocrStruct.Pages) == 0 {
		return nil, fmt.Errorf("HOCR data contains no pages")
	}
	if config.StartPage < 1 {
		return nil, fmt.Errorf("start page must be at least 1, got %d", config.StartPage)
	}

	// Display PDF structure debug if requested
	if config.DumpPDF {
		dumpPDFStructure(inputPDFData, 2000, config.Logger)
	}

	// Check for existing layers
	layerResult, err := checkExistingOCRLayers(inputPDFData, config.LayerName)
	if err != nil {
		return nil, fmt.Errorf("layer detection failed: %w", err)
	}

	// Display layer information if available
	if len(layerResult.Layers) > 0 {
		fmt.Println("Existing layers detected in PDF:")
		for i, layer := range layerResult.Layers {
			fmt.Printf("  %d. %s\n", i+1, layer)
		}

		// Additional debug information if requested
		if config.Debug {
			fmt.Println("Debug: Raw layer names with byte representation:")
			for i, layer := range layerResult.Layers {
				fmt.Printf("  %d. %q\n", i+1, layer)
			}
		}
	}

	// Report any warnings from layer detection
	for _, warning := range layerResult.Warnings {
		fmt.Println("Warning:", warning)
	}

	// Enforce safety check unless force override is requested
	if layerResult.HasOCRLayer && !config.Force {
		return nil, fmt.Errorf("file already has OCR (layer '%s') – use -force to reapply",
			layerResult.OCRLayerName)
	} else if layerResult.HasOCRLayer {
		fmt.Println("Warning: file already has OCR; reapplying due to -force will result in duplicate OCR data")
	}

	// Proceed with PDF modification
	finalPDF, err := modifyExistingPDF(
		inputPDFData,
		hocrStruct,
		config.StartPage,
		config.Debug,
		config.LayerName,
		config.Font,
	)
	if err != nil {
		return nil, fmt.Errorf("error modifying existing PDF: %w", err)
	}

	return finalPDF, nil
}
