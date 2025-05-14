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
// - DetectOCR: Best effort detection if OCR has already been applied to PDF
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

	// Get the logger
	logger := getLogger(config)

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
			fmt.Fprintf(logger, "Image %d is of type: %s\n", i+1, imageType)
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

	// Get the logger
	logger := getLogger(config)

	// Display PDF structure debug if requested
	if config.DumpPDF {
		dumpPDFStructure(inputPDFData, 2000, logger)
	}

	// Collect all warnings and potential errors first
	var warnings []string
	var blockers []string // Conditions that would block in strict mode
	var hasOCR bool
	var ocrLayerName string
	var layerInfo LayerCheckResult

	// Check for existing OCR
	ocrResult, err := DetectOCR(inputPDFData, config)

	// Process detection results
	if err != nil {
		// OCR detection failed
		detectionErr := fmt.Sprintf("OCR detection failed: %v", err)
		warnings = append(warnings, detectionErr)
		blockers = append(blockers, detectionErr)
	} else {
		// Store layer info for possible display
		layerInfo = ocrResult.LayerInfo

		// Add detection warnings
		warnings = append(warnings, ocrResult.Warnings...)

		// Handle existing OCR detection
		if ocrResult.HasOCR {
			hasOCR = true
			ocrLayerName = ocrResult.LayerInfo.OCRLayerName

			// Format OCR message with layer name if available
			layerText := ""
			if ocrLayerName != "" {
				layerText = fmt.Sprintf(" (layer '%s')", ocrLayerName)
			}

			ocrMsg := fmt.Sprintf("file already has OCR%s", layerText)
			warnings = append(warnings, ocrMsg)
			blockers = append(blockers, ocrMsg)
		}
	}

	// Decide whether to proceed based on Force/Strict settings
	shouldProceed := true

	// If we have blockers and we're in strict mode without force, we should stop
	if len(blockers) > 0 && config.Strict && !config.Force {
		return nil, fmt.Errorf("%s - set Force option to override", blockers[0])
	}

	// Log warnings and info if we're proceeding
	if shouldProceed && config.LogWarnings {
		// If force is enabled, explain we're overriding potential issues
		if config.Force && (len(blockers) > 0) {
			fmt.Fprintln(logger, "Force mode enabled: proceeding regardless of OCR detection issues")
		}

		// Display layer information if available
		if len(layerInfo.Layers) > 0 {
			fmt.Fprintln(logger, "Existing layers detected in PDF:")
			for i, layer := range layerInfo.Layers {
				fmt.Fprintf(logger, "  %d. %s\n", i+1, layer)
			}

			// Additional debug information if requested
			if config.Debug {
				fmt.Fprintln(logger, "Debug: Raw layer names with byte representation:")
				for i, layer := range layerInfo.Layers {
					fmt.Fprintf(logger, "  %d. %q\n", i+1, layer)
				}
			}
		}

		// Log all warnings
		for _, warning := range warnings {
			fmt.Fprintln(logger, "Warning:", warning)
		}

		// Add guidance for warnings that would be errors in strict mode
		if len(blockers) > 0 && !config.Strict {
			fmt.Fprintln(logger, "Proceeding with OCR application (use Strict mode to prevent this)")
		}

		// If force is being used to override existing OCR, warn about duplication
		if hasOCR && config.Force {
			fmt.Fprintln(logger, "Proceeding due to Force mode (may result in duplicate OCR data)")
		}
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
