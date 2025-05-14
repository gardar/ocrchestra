package pdfocr

import (
	"fmt"
	"regexp"
	"strings"
)

// detectPDFLayers attempts to find layer names in the raw PDF data.
func detectPDFLayers(pdfData []byte) ([]string, error) {
	if len(pdfData) == 0 {
		return nil, fmt.Errorf("empty PDF data")
	}

	content := string(pdfData)
	ocgPatterns := []string{
		`/Type\s*/OCG\s*/Name\s*\(([^)]+)\)`,
		`/Title\s*\(([^)]+)\)`,
		`/OCG\s*<<[^>]*?/Name\s*\(([^)]+)\)`,
		`<</Type/OCG/Name\(([^)]+)\)`,
		`/OCProperties.*?/OCGs\s*\[\s*.*?/Name\s*\(([^)]+)\)`,
		`/Name\s*\(([^)]+)\)[\s\S]{1,50}/Type\s*/OCG`,
	}

	var layers []string
	for _, pattern := range ocgPatterns {
		// Use a panic recovery block for regex operations
		var regex *regexp.Regexp
		func() {
			defer func() {
				if r := recover(); r != nil {
					// Log recovery but continue with other patterns
					fmt.Printf("Warning: Regex pattern caused error: %v\n", pattern)
				}
			}()
			regex = regexp.MustCompile(pattern)
		}()

		if regex != nil {
			matches := regex.FindAllStringSubmatch(content, -1)
			for _, match := range matches {
				if len(match) >= 2 {
					layers = append(layers, unescapePDFString(match[1]))
				}
			}
		}
	}

	// Check if any are UTF-16 BOM
	for i, layer := range layers {
		if len(layer) >= 2 && layer[0] == '\xfe' && layer[1] == '\xff' {
			decoded, err := decodeUTF16BE([]byte(layer))
			if err == nil {
				layers[i] = decoded
			}
		}
	}

	// Deduplicate
	unique := make([]string, 0, len(layers))
	seen := make(map[string]bool)
	for _, l := range layers {
		if !seen[l] {
			seen[l] = true
			unique = append(unique, l)
		}
	}
	return unique, nil
}

// LayerCheckResult contains the results of checking for OCR layers
type LayerCheckResult struct {
	Layers       []string // All detected layers
	HasOCRLayer  bool     // True if the specified OCR layer exists
	OCRLayerName string   // Name of the detected OCR layer (if any)
	Warnings     []string // Any warnings about potential OCR layers
}

// CheckExistingOCRLayers checks for existing OCR layers in a PDF
func CheckExistingOCRLayers(pdfData []byte, ocrLayerName string) (LayerCheckResult, error) {
	result := LayerCheckResult{}

	// Detect existing layers
	layers, err := detectPDFLayers(pdfData)
	if err != nil {
		return result, fmt.Errorf("cannot analyze layers: %w", err)
	}

	result.Layers = layers

	// Create a regex to match layer names with page numbers - more lenient pattern
	// This accounts for potential formatting issues in the PDF layer names
	pageLayerPattern := regexp.MustCompile(fmt.Sprintf(`^%s\s*\(Page\s*\d+.*`, regexp.QuoteMeta(ocrLayerName)))

	// Check for OCR layers
	for _, layer := range layers {
		// Check for exact match
		if layer == ocrLayerName {
			result.HasOCRLayer = true
			result.OCRLayerName = layer
			break
		}

		// Check for page-specific match with more lenient pattern
		if pageLayerPattern.MatchString(layer) {
			result.HasOCRLayer = true
			result.OCRLayerName = layer
			break
		}

		// Check for other potential OCR layers
		if strings.Contains(strings.ToLower(layer), "ocr") &&
			!strings.HasPrefix(layer, ocrLayerName) {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Existing layer detected that might contain OCR: %s", layer))
		}
	}

	return result, nil
}

// OCRDetectionResult contains comprehensive OCR detection information
type OCRDetectionResult struct {
	HasOCR      bool // True if any OCR is detected by any method
	HasLayerOCR bool // True if OCR layers are detected

	LayerInfo LayerCheckResult // Details from layer detection

	Warnings []string // Warnings from any detection method
}

// DetectOCR performs OCR detection using available methods
func DetectOCR(pdfData []byte, config OCRConfig) (OCRDetectionResult, error) {
	result := OCRDetectionResult{}

	// Check for OCR layers
	layerResult, err := CheckExistingOCRLayers(pdfData, config.LayerName)
	if err != nil {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Layer detection error: %v", err))
	} else {
		result.LayerInfo = layerResult
		result.HasLayerOCR = layerResult.HasOCRLayer
		result.Warnings = append(result.Warnings, layerResult.Warnings...)

		// If we have warnings about potential OCR layers but none were detected,
		// we should still flag these as potential OCR
		if !result.HasLayerOCR && len(layerResult.Warnings) > 0 {
			for _, warning := range layerResult.Warnings {
				if strings.Contains(warning, "might contain OCR") {
					// We don't set HasLayerOCR to true here as it wasn't an exact match
					// But we do add it to the warnings so the user can make an informed decision
					result.Warnings = append(result.Warnings,
						"Potential OCR layers were detected")
					break
				}
			}
		}
	}

	// For now, HasOCR is the same as HasLayerOCR
	// This will be expanded when new detection methods are added
	result.HasOCR = result.HasLayerOCR

	return result, nil
}
