package gdocai

import (
	"strings"

	"cloud.google.com/go/documentai/apiv1/documentaipb"
)

// textFromProto extracts the full text from a Document AI proto
func textFromProto(doc *documentaipb.Document) string {
	if doc == nil {
		return ""
	}
	return doc.Text
}

// textFromLayout extracts text from a layout's text anchor segments
func textFromLayout(layout *documentaipb.Document_Page_Layout, fullText string) string {
	if layout == nil || layout.TextAnchor == nil {
		return ""
	}
	runes := []rune(fullText)
	result := strings.Builder{}
	totalRunes := len(runes)

	for _, seg := range layout.TextAnchor.TextSegments {
		start := int(seg.StartIndex)
		end := int(seg.EndIndex)
		if start < 0 {
			start = 0
		}
		if end > totalRunes {
			end = totalRunes
		}
		if start > end {
			start = end
		}
		result.WriteString(string(runes[start:end]))
	}
	return result.String()
}
