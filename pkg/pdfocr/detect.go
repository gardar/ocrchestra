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

// checkExistingOCRLayers checks for existing OCR layers in a PDF
func checkExistingOCRLayers(pdfData []byte, ocrLayerName string) (LayerCheckResult, error) {
	result := LayerCheckResult{}

	// Detect existing layers
	layers, err := detectPDFLayers(pdfData)
	if err != nil {
		return result, fmt.Errorf("cannot analyze layers: %w", err)
	}

	result.Layers = layers

	// Create a regex to match layer names with page numbers
	// Pattern will match: "Base Layer Name" or "Base Layer Name (Page X)"
	pageLayerPattern := regexp.MustCompile(fmt.Sprintf(`^%s(\s*\(Page\s*\d+\))?$`, regexp.QuoteMeta(ocrLayerName)))

	// Check for OCR layers
	for _, layer := range layers {
		// Check for exact match or page-specific match
		if layer == ocrLayerName || pageLayerPattern.MatchString(layer) {
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
