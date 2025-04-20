package gdocai

import (
	"fmt"
	"strings"

	"cloud.google.com/go/documentai/apiv1/documentaipb"
	"github.com/gardar/ocrchestra/pkg/hocr"
)

// CreateHOCRStruct converts a Document AI proto directly to the HOCR struct
func CreateHOCRStruct(docProto *documentaipb.Document) (*hocr.HOCR, error) {
	// Process each page into HOCR pages
	var hocrPages []hocr.Page
	for _, page := range docProto.Pages {
		ocrPage, err := CreateHOCRPage(page, docProto.Text, int(page.PageNumber))
		if err != nil {
			return nil, err
		}
		hocrPages = append(hocrPages, ocrPage)
	}

	// Create the HOCR document with the pages
	result, err := CreateHOCRDocument(docProto, hocrPages...)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// CreateHOCRDocument creates an HOCR document structure, optionally with pages
// If docProto is nil, default values will be used for document properties
// If pages are provided, they will be added to the document
func CreateHOCRDocument(docProto *documentaipb.Document, pages ...hocr.Page) (*hocr.HOCR, error) {
	// Default values
	docLang := "unknown"
	pageCount := len(pages)

	// If we have a proto document, use its properties
	if docProto != nil {
		langFromDoc := getDocumentLanguage(docProto)
		if langFromDoc != "" {
			docLang = langFromDoc
		}
		// Only use proto page count if no pages were provided
		if pageCount == 0 && docProto.Pages != nil {
			pageCount = len(docProto.Pages)
		}
	}

	result := &hocr.HOCR{
		Title:    "Document OCR",
		Language: docLang,
		Metadata: map[string]string{
			"ocr-system":          "Document AI OCR",
			"ocr-number-of-pages": fmt.Sprintf("%d", pageCount),
			"ocr-capabilities":    "ocrp_lang ocr_page ocr_carea ocr_par ocr_line ocrx_word",
			"ocr-langs":           docLang,
		},
		Pages: make([]hocr.Page, 0, len(pages)),
	}

	// Add any provided pages
	if len(pages) > 0 {
		result.Pages = append(result.Pages, pages...)

		// Update document with language information from all pages
		updateDocumentLanguages(result)
	}

	return result, nil
}

// CreateHOCRPage converts a single Document AI page to an HOCR page
func CreateHOCRPage(page *documentaipb.Document_Page, fullText string, pageNumber int) (hocr.Page, error) {
	ocrPage := hocr.Page{
		ID:         fmt.Sprintf("page_%d", pageNumber),
		PageNumber: pageNumber,
		Metadata:   make(map[string]string),
	}

	// Extract page language if available
	if len(page.DetectedLanguages) > 0 {
		ocrPage.Lang = page.DetectedLanguages[0].LanguageCode
	}

	// Set bounding box if available
	pageBox := getHocrBoundingBox(page.Layout, page.Dimension)
	if pageBox != "" {
		if bbox := hocr.ParseBoundingBoxFromTitle(pageBox); bbox != nil {
			ocrPage.BBox = *bbox
		}

		// Extract image name if present
		props := hocr.ParseTitle(pageBox)
		if image, ok := props["image"]; ok && len(image) > 0 {
			ocrPage.ImageName = image[0]
		}
	}

	// Track which lines are assigned to avoid duplication
	assignedLines := make(map[string]bool)

	// Convert content areas (using ocr_carea class)
	for aidx, area := range page.Blocks {
		ocrArea := hocr.Area{
			ID:       fmt.Sprintf("carea_%d_%d", pageNumber, aidx),
			Metadata: make(map[string]string),
		}

		// Set bounding box
		areaBox := getHocrBoundingBox(area.Layout, page.Dimension)
		if areaBox != "" {
			if bbox := hocr.ParseBoundingBoxFromTitle(areaBox); bbox != nil {
				ocrArea.BBox = *bbox
			}
		}

		// Process paragraphs within this area
		for pidx, para := range page.Paragraphs {
			// Skip if paragraph is not within this area
			if !isElementInParent(para.Layout, area.Layout, fullText) {
				continue
			}

			ocrParagraph := hocr.Paragraph{
				ID:       fmt.Sprintf("par_%d_%d_%d", pageNumber, aidx, pidx),
				Metadata: make(map[string]string),
			}

			paraBox := getHocrBoundingBox(para.Layout, page.Dimension)
			if paraBox != "" {
				if bbox := hocr.ParseBoundingBoxFromTitle(paraBox); bbox != nil {
					ocrParagraph.BBox = *bbox
				}
			}

			// Process lines within this paragraph
			for lidx, line := range page.Lines {
				// Skip if line is not within this paragraph
				if !isElementInParent(line.Layout, para.Layout, fullText) {
					continue
				}

				// Mark line as assigned to avoid duplication
				lineKey := getLayoutKey(line.Layout)
				assignedLines[lineKey] = true

				ocrLine := convertLineFromProto(line, page, fullText, pageNumber, aidx, pidx, lidx)
				ocrParagraph.Lines = append(ocrParagraph.Lines, ocrLine)
			}

			ocrParagraph.Words = nil // Words should be in lines
			ocrArea.Paragraphs = append(ocrArea.Paragraphs, ocrParagraph)
		}

		ocrPage.Areas = append(ocrPage.Areas, ocrArea)
	}

	// Process paragraphs not assigned to any block or area
	for pidx, para := range page.Paragraphs {
		// Check if this paragraph is already assigned to a block or area
		isAssigned := false
		for _, block := range page.Blocks {
			if isElementInParent(para.Layout, block.Layout, fullText) {
				isAssigned = true
				break
			}
		}

		if isAssigned {
			continue
		}

		ocrParagraph := hocr.Paragraph{
			ID:       fmt.Sprintf("par_%d_direct_%d", pageNumber, pidx),
			Metadata: make(map[string]string),
		}

		paraBox := getHocrBoundingBox(para.Layout, page.Dimension)
		if paraBox != "" {
			if bbox := hocr.ParseBoundingBoxFromTitle(paraBox); bbox != nil {
				ocrParagraph.BBox = *bbox
			}
		}

		// Process lines within this paragraph
		for lidx, line := range page.Lines {
			// Skip if line is not within this paragraph
			if !isElementInParent(line.Layout, para.Layout, fullText) {
				continue
			}

			// Mark line as assigned to avoid duplication
			lineKey := getLayoutKey(line.Layout)
			assignedLines[lineKey] = true

			ocrLine := convertLineFromProto(line, page, fullText, pageNumber, 0, pidx, lidx)
			ocrParagraph.Lines = append(ocrParagraph.Lines, ocrLine)
		}

		ocrParagraph.Words = nil // Words should be in lines
		ocrPage.Paragraphs = append(ocrPage.Paragraphs, ocrParagraph)
	}

	// Add unassigned lines directly to the page
	for lidx, line := range page.Lines {
		lineKey := getLayoutKey(line.Layout)
		if !assignedLines[lineKey] {
			ocrLine := convertLineFromProto(line, page, fullText, pageNumber, 0, 0, lidx)
			ocrPage.Lines = append(ocrPage.Lines, ocrLine)
		}
	}

	return ocrPage, nil
}

// updateDocumentLanguages collects all languages used in the document and updates metadata
func updateDocumentLanguages(result *hocr.HOCR) {
	// Collect all languages used in the document
	allLangs := make(map[string]bool)
	allLangs[result.Language] = true

	for _, page := range result.Pages {
		if page.Lang != "" {
			allLangs[page.Lang] = true
		}

		// Check Areas
		for _, area := range page.Areas {
			if area.Lang != "" {
				allLangs[area.Lang] = true
			}

			// Check Paragraphs in Areas
			for _, para := range area.Paragraphs {
				if para.Lang != "" {
					allLangs[para.Lang] = true
				}

				// Check Lines in Paragraphs
				for _, line := range para.Lines {
					if line.Lang != "" {
						allLangs[line.Lang] = true
					}

					// Check Words in Lines
					for _, word := range line.Words {
						if word.Lang != "" {
							allLangs[word.Lang] = true
						}
					}
				}

				// Check Words directly in Paragraphs
				for _, word := range para.Words {
					if word.Lang != "" {
						allLangs[word.Lang] = true
					}
				}
			}

			// Check Lines directly in Areas
			for _, line := range area.Lines {
				if line.Lang != "" {
					allLangs[line.Lang] = true
				}

				// Check Words in Lines
				for _, word := range line.Words {
					if word.Lang != "" {
						allLangs[word.Lang] = true
					}
				}
			}

			// Check Words directly in Areas
			for _, word := range area.Words {
				if word.Lang != "" {
					allLangs[word.Lang] = true
				}
			}
		}

		// Check Paragraphs directly in Page
		for _, para := range page.Paragraphs {
			if para.Lang != "" {
				allLangs[para.Lang] = true
			}

			// Check Lines in Paragraphs
			for _, line := range para.Lines {
				if line.Lang != "" {
					allLangs[line.Lang] = true
				}

				// Check Words in Lines
				for _, word := range line.Words {
					if word.Lang != "" {
						allLangs[word.Lang] = true
					}
				}
			}

			// Check Words directly in Paragraphs
			for _, word := range para.Words {
				if word.Lang != "" {
					allLangs[word.Lang] = true
				}
			}
		}

		// Check Lines directly in Page
		for _, line := range page.Lines {
			if line.Lang != "" {
				allLangs[line.Lang] = true
			}

			// Check Words in Lines
			for _, word := range line.Words {
				if word.Lang != "" {
					allLangs[word.Lang] = true
				}
			}
		}
	}

	// Build language list for metadata
	var langsList []string
	for lang := range allLangs {
		if lang != "" && lang != "unknown" {
			langsList = append(langsList, lang)
		}
	}

	if len(langsList) > 0 {
		result.Metadata["ocr-langs"] = strings.Join(langsList, ", ")
	}
}

// getHocrBoundingBox converts Document AI coordinates to hOCR coordinates
// Takes normalized vertices (0-1) and scales them to actual pixel dimensions
func getHocrBoundingBox(layout *documentaipb.Document_Page_Layout, dimension *documentaipb.Document_Page_Dimension) string {
	if layout == nil || layout.BoundingPoly == nil || dimension == nil || len(layout.BoundingPoly.NormalizedVertices) < 4 {
		return ""
	}
	vertices := layout.BoundingPoly.NormalizedVertices
	minX := int(vertices[0].X*dimension.Width + 0.5)
	minY := int(vertices[0].Y*dimension.Height + 0.5)
	maxX := int(vertices[2].X*dimension.Width + 0.5)
	maxY := int(vertices[2].Y*dimension.Height + 0.5)
	return fmt.Sprintf("bbox %d %d %d %d", minX, minY, maxX, maxY)
}

// getDocumentLanguage finds the most common language in the document
// by counting language occurrences across all elements
func getDocumentLanguage(doc *documentaipb.Document) string {
	// Create a frequency count of all languages in the document
	langCount := make(map[string]int)

	// Process all pages and tokens
	for _, page := range doc.Pages {
		// Process page languages
		for _, lang := range page.DetectedLanguages {
			langCount[lang.LanguageCode]++
		}

		// Process token languages
		for _, token := range page.Tokens {
			for _, lang := range token.DetectedLanguages {
				langCount[lang.LanguageCode]++
			}
		}
	}

	// Find the most frequent language
	var mostCommonLang string
	var highestCount int

	for lang, count := range langCount {
		if count > highestCount {
			highestCount = count
			mostCommonLang = lang
		}
	}

	return mostCommonLang
}

// Helper function to check if an element is contained within a parent
func isElementInParent(elementLayout, parentLayout *documentaipb.Document_Page_Layout, fullText string) bool {
	if elementLayout == nil || parentLayout == nil ||
		elementLayout.TextAnchor == nil || parentLayout.TextAnchor == nil ||
		len(elementLayout.TextAnchor.TextSegments) == 0 || len(parentLayout.TextAnchor.TextSegments) == 0 {
		return false
	}

	elementStart := elementLayout.TextAnchor.TextSegments[0].StartIndex
	elementEnd := elementLayout.TextAnchor.TextSegments[0].EndIndex
	parentStart := parentLayout.TextAnchor.TextSegments[0].StartIndex
	parentEnd := parentLayout.TextAnchor.TextSegments[0].EndIndex

	return elementStart >= parentStart && elementEnd <= parentEnd
}

// Helper function to generate a unique key for a layout
func getLayoutKey(layout *documentaipb.Document_Page_Layout) string {
	if layout == nil || layout.TextAnchor == nil || len(layout.TextAnchor.TextSegments) == 0 {
		return ""
	}
	return fmt.Sprintf("%d-%d", layout.TextAnchor.TextSegments[0].StartIndex,
		layout.TextAnchor.TextSegments[0].EndIndex)
}

// Convert a proto line to an OCR line
func convertLineFromProto(line *documentaipb.Document_Page_Line, page *documentaipb.Document_Page,
	fullText string, pageNum, blockIdx, paraIdx, lineIdx int) hocr.Line {

	ocrLine := hocr.Line{
		ID:       fmt.Sprintf("line_%d_%d_%d_%d", pageNum, blockIdx, paraIdx, lineIdx),
		Metadata: make(map[string]string),
	}

	// Extract line bounding box
	lineBox := getHocrBoundingBox(line.Layout, page.Dimension)
	if lineBox != "" {
		if bbox := hocr.ParseBoundingBoxFromTitle(lineBox); bbox != nil {
			ocrLine.BBox = *bbox
		}

		// Extract baseline if present
		props := hocr.ParseTitle(lineBox)
		if baseline, ok := props["baseline"]; ok && len(baseline) > 0 {
			ocrLine.Baseline = strings.Join(baseline, " ")
		}
	}

	// Extract line language
	if len(line.DetectedLanguages) > 0 {
		ocrLine.Lang = line.DetectedLanguages[0].LanguageCode
	}

	// Find tokens that belong to this line
	for tidx, token := range page.Tokens {
		if !isElementInParent(token.Layout, line.Layout, fullText) {
			continue
		}

		// Clean token text
		tokenText := textFromLayout(token.Layout, fullText)
		cleanText := strings.TrimSpace(tokenText)
		cleanText = strings.ReplaceAll(cleanText, "\n", " ")
		cleanText = strings.ReplaceAll(cleanText, "\r", "")

		// Trim trailing space if the token has a detected break
		if token.DetectedBreak != nil &&
			token.DetectedBreak.Type != documentaipb.Document_Page_Token_DetectedBreak_TYPE_UNSPECIFIED {
			runesTok := []rune(cleanText)
			if len(runesTok) > 0 {
				last := runesTok[len(runesTok)-1]
				if last == ' ' || last == '\n' || last == '\r' || last == '\t' {
					cleanText = string(runesTok[:len(runesTok)-1])
				}
			}
		}

		word := hocr.Word{
			ID:       fmt.Sprintf("word_%d_%d_%d_%d_%d", pageNum, blockIdx, paraIdx, lineIdx, tidx),
			Text:     cleanText,
			Metadata: make(map[string]string),
		}

		// Extract word bounding box
		tokenBox := getHocrBoundingBox(token.Layout, page.Dimension)
		if tokenBox != "" {
			if bbox := hocr.ParseBoundingBoxFromTitle(tokenBox); bbox != nil {
				word.BBox = *bbox
			}
		}

		// Extract confidence
		if token.Layout != nil {
			word.Confidence = float64(token.Layout.Confidence * 100)
		}

		// Extract language
		if len(token.DetectedLanguages) > 0 {
			word.Lang = token.DetectedLanguages[0].LanguageCode
		}

		ocrLine.Words = append(ocrLine.Words, word)
	}

	return ocrLine
}
