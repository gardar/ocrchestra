package gdocai

import (
	"cloud.google.com/go/documentai/apiv1/documentaipb"
	"github.com/gardar/ocrchestra/pkg/hocr"
)

// Document represents the primary result of OCR processing
// It composes all the different components together
type Document struct {
	Raw        *RawDocument        // Original Document AI response
	Structured *StructuredDocument // Processed document structure
	Text       *TextContent        // Full text content
	Hocr       *HocrContent        // hOCR representation
	FormFields *FormData           // Extracted form fields
}

// RawDocument is a thin wrapper around the Google Document AI response
type RawDocument struct {
	*documentaipb.Document
}

// StructuredDocument represents the document content in a hierarchical structure
type StructuredDocument struct {
	Pages []*Page // Collection of pages
}

// TextContent wraps the full text of the document
type TextContent struct {
	Content string // The full text content of the document
}

// HocrContent wraps the hOCR data
type HocrContent struct {
	Content *hocr.HOCR // The hOCR representation
	HTML    string
}

// FormData contains extracted form fields from the document
type FormData struct {
	Fields map[string]interface{} // Map of field names to values
}

// Page represents a single page in the document with its structural elements
type Page struct {
	DocumentaiObject *documentaipb.Document_Page // Original Document AI page
	DocumentText     string                      // Source document text
	Text             string                      // Text for this specific page
	PageNumber       int                         // Page number (1-based)

	FormFields []*FormField // Form fields found on this page
	Lines      []*Line      // Text lines on this page
	Paragraphs []*Paragraph // Paragraphs on this page
	Blocks     []*Block     // Layout blocks on this page
	Tokens     []*Token     // Individual tokens/words on this page
}

// Block represents a block of content on a page
type Block struct {
	DocumentaiObject *documentaipb.Document_Page_Block // Original Document AI block
	PageNumber       int                               // Parent page number
	Paragraphs       []*Paragraph                      // Child paragraphs in this block
	Text             string                            // Text content of this block
}

// Paragraph represents a paragraph within a block
type Paragraph struct {
	DocumentaiObject *documentaipb.Document_Page_Paragraph // Original Document AI paragraph
	PageNumber       int                                   // Parent page number
	Lines            []*Line                               // Child lines in this paragraph
	Text             string                                // Text content of this paragraph
}

// Line represents a line of text within a paragraph
type Line struct {
	DocumentaiObject *documentaipb.Document_Page_Line // Original Document AI line
	PageNumber       int                              // Parent page number
	Tokens           []*Token                         // Child tokens in this line
	Text             string                           // Text content of this line
}

// Token represents a word or token within a line
type Token struct {
	DocumentaiObject *documentaipb.Document_Page_Token // Original Document AI token
	PageNumber       int                               // Parent page number
	Text             string                            // Text content of this token
}

// FormField represents a detected form field from a Form Document
type FormField struct {
	DocumentaiObject *documentaipb.Document_Page_FormField // Original Document AI form field
	DocumentText     string                                // Source document text
	FieldName        string                                // Extracted field name
	FieldValue       string                                // Extracted field value
}
