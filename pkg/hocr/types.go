package hocr

// HOCR represents the entire hOCR document structure
type HOCR struct {
	Title       string            // Document title
	Description string            // Document description
	Language    string            // Document language
	Metadata    map[string]string // Additional metadata
	Pages       []Page            // Pages in the document
}

// Page is one page of recognized text
// Corresponds to hOCR element with class: 'ocr_page'
type Page struct {
	ID         string            // Unique identifier
	Title      string            // Original title attribute
	PageNumber int               // Page number in document
	ImageName  string            // Source image filename
	Lang       string            // Language code for this page
	BBox       BoundingBox       // Page coordinates
	Areas      []Area            // Content areas (columns)
	Paragraphs []Paragraph       // Paragraphs directly under page
	Lines      []Line            // Lines directly under page (no parent)
	Metadata   map[string]string // Other page properties
}

// Class assign 'ocr_page' to 'Page' struct
func (Page) Class() string { return "ocr_page" }

// Area represents a content area (column or region)
// Corresponds to hOCR element with class: 'ocr_carea'
type Area struct {
	ID         string            // Unique identifier
	Lang       string            // Language code
	BBox       BoundingBox       // Area coordinates
	Paragraphs []Paragraph       // Paragraphs in this area
	Lines      []Line            // Text lines directly under area
	Words      []Word            // Words directly under area (no line parent)
	Metadata   map[string]string // Other area properties
}

// Class assign 'ocr_carea' to 'Area' struct
func (Area) Class() string { return "ocr_carea" }

// Paragraph represents a paragraph within an area or block
// Corresponds to hOCR element with class: 'ocr_par'
type Paragraph struct {
	ID       string            // Unique identifier
	Lang     string            // Language code
	BBox     BoundingBox       // Paragraph coordinates
	Lines    []Line            // Text lines in this paragraph
	Words    []Word            // Words directly under paragraph (no line parent)
	Metadata map[string]string // Other paragraph properties
}

// Class assign 'ocr_par' to 'Paragraph' struct
func (Paragraph) Class() string { return "ocr_par" }

// Line represents a line of text
// Corresponds to hOCR element with class: 'ocr_line'
type Line struct {
	ID       string            // Unique identifier
	Lang     string            // Language code
	BBox     BoundingBox       // Line coordinates
	Baseline string            // Baseline information
	Words    []Word            // Words in this line
	Metadata map[string]string // Other line properties
}

// Class assign 'ocr_line' to 'Line' struct
func (Line) Class() string { return "ocr_line" }

// Word is a recognized word with bounding box
// Corresponds to hOCR element with class: 'ocrx_word'
type Word struct {
	ID         string            // Unique identifier
	Text       string            // The actual text content
	BBox       BoundingBox       // Word coordinates
	Confidence float64           // Recognition confidence (0-100)
	Lang       string            // Language code
	Metadata   map[string]string // Other word properties
}

// Class assign 'ocrx_word' to 'Word' struct
func (Word) Class() string { return "ocrx_word" }

// BoundingBox represents a rectangle in the document
// Used to store hOCR 'bbox' property values
type BoundingBox struct {
	X1 float64 // Left coordinate
	Y1 float64 // Top coordinate
	X2 float64 // Right coordinate
	Y2 float64 // Bottom coordinate
}

// NewBoundingBox creates a bounding box from coordinates
// This is a convenience constructor function that creates a bounding box
// from the x1, y1, x2, y2 coordinates commonly found in hOCR 'bbox' properties.
// x1, y1 represent the top-left corner, while x2, y2 represent the bottom-right corner.
func NewBoundingBox(x1, y1, x2, y2 float64) BoundingBox {
	return BoundingBox{
		X1: x1,
		Y1: y1,
		X2: x2,
		Y2: y2,
	}
}
