package gdocai

import (
	"sort"

	"cloud.google.com/go/documentai/apiv1/documentaipb"
	"github.com/gardar/ocrchestra/pkg/hocr"
)

// DocumentFromProto converts a Document AI response into our structure
func DocumentFromProto(doc *documentaipb.Document) *Document {
	// Extract the text content
	text := textFromProto(doc)

	// Extract form fields
	formFields := ExtractFormFields(doc)

	// Extract custom extractor fields
	customExtractorFields := ExtractCustomExtractorFields(doc)

	// Create the structured document with pages
	structuredDoc := &StructuredDocument{
		Pages: createPagesFromProtoDoc(doc),
	}

	// Create the wrapper for the raw document
	rawDoc := &RawDocument{
		Document: doc,
	}

	// Create form data wrapper
	formData := &FormData{
		Fields: formFields,
	}

	// Create custom extractor data wrapper
	customExtractorData := &CustomExtractorData{
		Fields: customExtractorFields,
	}

	// Create text content wrapper
	textContent := &TextContent{
		Content: text,
	}

	// Create HOCR content
	hocrStruct, _ := CreateHOCRStruct(doc)
	generatedHTML, _ := hocr.GenerateHOCRDocument(hocrStruct)

	hocrContent := &HocrContent{
		Content: hocrStruct,
		HTML:    generatedHTML,
	}

	// Assemble the full document
	return &Document{
		Raw:                   rawDoc,
		Structured:            structuredDoc,
		Text:                  textContent,
		Hocr:                  hocrContent,
		FormFields:            formData,
		CustomExtractorFields: customExtractorData,
	}
}

// createPagesFromProtoDoc transforms the raw Document AI pages into structured format
// This builds the hierarchy of blocks, paragraphs, lines and tokens
func createPagesFromProtoDoc(doc *documentaipb.Document) []*Page {
	var result []*Page

	// Process each page in the document
	for _, page := range doc.Pages {
		pageNum := int(page.PageNumber)
		docAiPage := &Page{
			DocumentaiObject: page,
			DocumentText:     doc.Text,
			PageNumber:       pageNum,
			Text:             textFromLayout(page.Layout, doc.Text),
		}

		// Collect form fields
		docAiPage.FormFields = make([]*FormField, 0, len(page.FormFields))
		for _, formField := range page.FormFields {
			docAiPage.FormFields = append(docAiPage.FormFields, &FormField{
				DocumentaiObject: formField,
				DocumentText:     doc.Text,
				FieldName:        textFromLayout(formField.FieldName, doc.Text),
				FieldValue:       textFromLayout(formField.FieldValue, doc.Text),
			})
		}

		// Collect tokens (words)
		docAiPage.Tokens = make([]*Token, 0, len(page.Tokens))
		for _, token := range page.Tokens {
			txt := textFromLayout(token.Layout, doc.Text)
			// Trim trailing whitespace if the token has a detected break
			if token.DetectedBreak != nil &&
				token.DetectedBreak.Type != documentaipb.Document_Page_Token_DetectedBreak_TYPE_UNSPECIFIED {
				runesTok := []rune(txt)
				if len(runesTok) > 0 {
					last := runesTok[len(runesTok)-1]
					if last == ' ' || last == '\n' || last == '\r' || last == '\t' {
						txt = string(runesTok[:len(runesTok)-1])
					}
				}
			}

			docAiToken := &Token{
				DocumentaiObject: token,
				PageNumber:       pageNum,
				Text:             txt,
			}

			docAiPage.Tokens = append(docAiPage.Tokens, docAiToken)
		}

		// Collect lines
		docAiPage.Lines = make([]*Line, 0, len(page.Lines))
		for _, line := range page.Lines {
			docAiLine := &Line{
				DocumentaiObject: line,
				PageNumber:       pageNum,
				Text:             textFromLayout(line.Layout, doc.Text),
			}

			docAiPage.Lines = append(docAiPage.Lines, docAiLine)
		}

		// Collect paragraphs
		docAiPage.Paragraphs = make([]*Paragraph, 0, len(page.Paragraphs))
		for _, paragraph := range page.Paragraphs {
			docAiParagraph := &Paragraph{
				DocumentaiObject: paragraph,
				PageNumber:       pageNum,
				Text:             textFromLayout(paragraph.Layout, doc.Text),
			}

			docAiPage.Paragraphs = append(docAiPage.Paragraphs, docAiParagraph)
		}

		// Collect blocks
		docAiPage.Blocks = make([]*Block, 0, len(page.Blocks))
		for _, block := range page.Blocks {
			docAiBlock := &Block{
				DocumentaiObject: block,
				PageNumber:       pageNum,
				Text:             textFromLayout(block.Layout, doc.Text),
			}

			docAiPage.Blocks = append(docAiPage.Blocks, docAiBlock)
		}

		// Build hierarchy: lines -> paragraphs -> blocks
		for _, line := range docAiPage.Lines {
			line.Tokens = getChildElements(line, docAiPage.Tokens)
		}

		for _, paragraph := range docAiPage.Paragraphs {
			paragraph.Lines = getChildElements(paragraph, docAiPage.Lines)
		}

		for _, block := range docAiPage.Blocks {
			block.Paragraphs = getChildElements(block, docAiPage.Paragraphs)
		}

		result = append(result, docAiPage)
	}

	// Sort pages by number if there are multiple
	if len(result) > 1 && result[0].PageNumber > 0 {
		sort.Slice(result, func(i, j int) bool {
			return result[i].PageNumber < result[j].PageNumber
		})
	}

	return result
}

// getChildElements is a generic function that finds all child elements that are contained within a parent element
// It works with any struct types that have a DocumentaiObject field with a Layout and TextAnchor
func getChildElements[P any, C any](parent P, children []C) []C {
	// Get the parent's layout from any supported type
	var parentLayout *documentaipb.Document_Page_Layout

	switch p := any(parent).(type) {
	case *Line:
		if p.DocumentaiObject != nil {
			parentLayout = p.DocumentaiObject.Layout
		}
	case *Paragraph:
		if p.DocumentaiObject != nil {
			parentLayout = p.DocumentaiObject.Layout
		}
	case *Block:
		if p.DocumentaiObject != nil {
			parentLayout = p.DocumentaiObject.Layout
		}
	default:
		return nil
	}

	// Validate parent layout
	if parentLayout == nil || parentLayout.TextAnchor == nil || len(parentLayout.TextAnchor.TextSegments) == 0 {
		return nil
	}

	// Get parent text range
	parentStartIndex := parentLayout.TextAnchor.TextSegments[0].StartIndex
	parentEndIndex := parentLayout.TextAnchor.TextSegments[0].EndIndex

	var result []C
	for _, child := range children {
		// Get child layout
		var childLayout *documentaipb.Document_Page_Layout

		switch c := any(child).(type) {
		case *Token:
			if c.DocumentaiObject != nil {
				childLayout = c.DocumentaiObject.Layout
			}
		case *Line:
			if c.DocumentaiObject != nil {
				childLayout = c.DocumentaiObject.Layout
			}
		case *Paragraph:
			if c.DocumentaiObject != nil {
				childLayout = c.DocumentaiObject.Layout
			}
		default:
			continue
		}

		// Validate child layout
		if childLayout == nil || childLayout.TextAnchor == nil || len(childLayout.TextAnchor.TextSegments) == 0 {
			continue
		}

		// Check if child is within parent range
		childStartIndex := childLayout.TextAnchor.TextSegments[0].StartIndex
		childEndIndex := childLayout.TextAnchor.TextSegments[0].EndIndex

		if childStartIndex >= parentStartIndex && childEndIndex <= parentEndIndex {
			result = append(result, child)
		}
	}

	return result
}
