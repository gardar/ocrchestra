package hocr

import (
	"strings"
)

// ExtractHOCRText extracts all text from an HOCR document
// The text is ordered by page, with paragraphs separated by newlines
// and pages separated by double newlines
func ExtractHOCRText(hocrDoc *HOCR) string {
	var builder strings.Builder

	for _, page := range hocrDoc.Pages {
		// Track processed content to avoid duplication
		processedContent := make(map[string]bool)

		// Extract text from areas (which may contain paragraphs and lines)
		for _, area := range page.Areas {
			extractAreaText(&builder, area, processedContent)
		}

		// Extract text from paragraphs directly on the page
		for _, para := range page.Paragraphs {
			extractParagraphText(&builder, para, processedContent)
		}

		// Extract text from lines directly on the page
		for _, line := range page.Lines {
			lineKey := getLineKey(line)
			if !processedContent[lineKey] {
				extractLineText(&builder, line)
				processedContent[lineKey] = true
			}
		}

		// Add a page break
		builder.WriteString("\n\n")
	}

	return builder.String()
}

// extractAreaText processes text from an area, including its paragraphs and lines
func extractAreaText(builder *strings.Builder, area Area, processed map[string]bool) {
	// Process paragraphs in the area
	for _, para := range area.Paragraphs {
		extractParagraphText(builder, para, processed)
	}

	// Process lines directly in the area
	for _, line := range area.Lines {
		lineKey := getLineKey(line)
		if !processed[lineKey] {
			extractLineText(builder, line)
			processed[lineKey] = true
		}
	}

	// Process words directly in the area (rare, but possible)
	if len(area.Words) > 0 {
		for _, word := range area.Words {
			builder.WriteString(word.Text)
			builder.WriteString(" ")
		}
		builder.WriteString("\n")
	}
}

// extractParagraphText processes text from a paragraph and its lines
func extractParagraphText(builder *strings.Builder, para Paragraph, processed map[string]bool) {
	// Process lines in the paragraph
	for _, line := range para.Lines {
		lineKey := getLineKey(line)
		if !processed[lineKey] {
			extractLineText(builder, line)
			processed[lineKey] = true
		}
	}

	// Process words directly in the paragraph (if any)
	if len(para.Words) > 0 {
		for _, word := range para.Words {
			builder.WriteString(word.Text)
			builder.WriteString(" ")
		}
		builder.WriteString("\n")
	}
}

// extractLineText processes text from a line and its words
func extractLineText(builder *strings.Builder, line Line) {
	for _, word := range line.Words {
		builder.WriteString(word.Text)
		builder.WriteString(" ")
	}
	builder.WriteString("\n")
}

// getLineKey generates a unique key for a line to avoid duplication
func getLineKey(line Line) string {
	return line.ID
}
