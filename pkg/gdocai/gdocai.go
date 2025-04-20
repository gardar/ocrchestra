// Package gdocai provides a comprehensive integration with Google Document AI for document processing.
//
// This package's primary purpose is to process PDFs with Google Document AI and create searchable documents
// by precisely overlaying OCR text at the exact position of each recognized word. The package handles the
// complete workflow from raw PDF to a fully searchable document with the OCR text layer correctly positioned.
//
// The package converts Google Document AI's proprietary format into standard formats (plain text and HOCR),
// while maintaining the positional data needed to reconstruct document layout. It also extracts form fields
// from documents with form elements.
//
// Key Features:
//
// - Process PDFs with Google Document AI to extract text and structural information
// - Create searchable PDFs with transparent OCR text overlaid at precise positions
// - Extract form fields from documents with form elements
// - Convert Document AI output to standard HOCR format for interoperability
// - Access the full hierarchical structure of document content (blocks, paragraphs, lines, words)
// - Extract page images for further processing
//
// Main Functions:
//
// - ProcessDocument: Sends a document to Google Document AI for processing
// - DocumentFromProto: Converts Document AI response to a structured format
// - DocumentHOCR: Processes a document and returns the structured data plus hOCR HTML
// - DocumentHOCRFromPages: Processes multiple pages as a single document and returns the hOCR HTML
// - ExtractFormFields: Gets form fields from the document as a map
//
// Usage Requirements:
//
// - Google Cloud project with Document AI API enabled
// - Document AI processor configured for OCR
// - Authentication via GOOGLE_APPLICATION_CREDENTIALS environment variable
//
package gdocai

import (
	"context"
	"fmt"

	"github.com/gardar/ocrchestra/pkg/hocr"
)

// DocumentHOCR processes a PDF with Document AI and returns our structured Document.
// It handles the complete process from PDF bytes to a fully populated Document structure.
// It returns:
// - The internal Document representation
// - The HOCR HTML as a string
// - Any error encountered
func DocumentHOCR(ctx context.Context, pdfBytes []byte, cfg *Config) (*Document, string, error) {
	// Process the PDF using Google Document AI
	rawDoc, err := ProcessDocument(ctx, pdfBytes, cfg)
	if err != nil {
		return nil, "", fmt.Errorf("failed to process document: %w", err)
	}

	// Convert to our structure
	doc := DocumentFromProto(rawDoc)

	// Return the document and generated hOCR HTML
	return doc, doc.Hocr.HTML, nil
}

// DocumentHOCRFromPages processes multiple PDFs as individual pages
// and combines them into a single document.
//
// Note: Unlike DocumentHOCR, this function does not yet create a complete
// structured document. It creates a minimal structure with essential fields
// sufficient for HOCR generation and image extraction, but does not fully
// populate all hierarchical elements (blocks, paragraphs, etc.) that would
// be available when processing a single multi-page document with DocumentFromProto.
func DocumentHOCRFromPages(ctx context.Context, pagePdfBytesList [][]byte, cfg *Config) (*Document, string, error) {
	var hocrPages []hocr.Page
	var structuredPages []*Page
	var fullText string

	// Process each page individually
	for i, pageBytes := range pagePdfBytesList {
		// Process with Document AI
		pageDoc, err := ProcessDocument(ctx, pageBytes, cfg)
		if err != nil {
			return nil, "", fmt.Errorf("failed to process page %d: %w", i+1, err)
		}

		if len(pageDoc.Pages) != 1 {
			return nil, "", fmt.Errorf("expected 1 page in result for page %d, got %d", i+1, len(pageDoc.Pages))
		}

		// Create a structured page
		pageNum := i + 1
		docAiPage := &Page{
			DocumentaiObject: pageDoc.Pages[0],
			DocumentText:     pageDoc.Text,
			PageNumber:       pageNum,
			Text:             textFromLayout(pageDoc.Pages[0].Layout, pageDoc.Text),
		}
		structuredPages = append(structuredPages, docAiPage)

		// Append this page's text to the full text
		if i > 0 {
			fullText += "\n\n"
		}
		fullText += pageDoc.Text

		// Convert to HOCR page
		hocrPage, err := CreateHOCRPage(pageDoc.Pages[0], pageDoc.Text, pageNum)
		if err != nil {
			return nil, "", fmt.Errorf("failed to create HOCR page %d: %w", i+1, err)
		}

		hocrPages = append(hocrPages, hocrPage)
	}

	// Create combined document
	hocrDoc, err := CreateHOCRDocument(nil, hocrPages...)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create HOCR document: %w", err)
	}

	// Generate HTML
	hocrHTML, err := hocr.GenerateHOCRDocument(hocrDoc)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate HOCR HTML: %w", err)
	}

	// Create the Document structure with structured pages
	doc := &Document{
		Structured: &StructuredDocument{
			Pages: structuredPages,
		},
		Text: &TextContent{
			Content: fullText,
		},
		Hocr: &HocrContent{
			Content: hocrDoc,
			HTML:    hocrHTML,
		},
		FormFields: &FormData{
			Fields: make(map[string]interface{}),
		},
		// Other fields as needed
	}

	return doc, hocrHTML, nil
}
