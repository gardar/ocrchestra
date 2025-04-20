package pdfocr

import (
	"io"
)

// OCRConfig holds user options for applying OCR to PDF
type OCRConfig struct {
	Debug       bool      // Enable debug mode
	Force       bool      // Force reapply OCR even if layer already exists
	LayerName   string    // Base name of OCR layer (page number will be appended)
	StartPage   int       // Start applying OCR from this page number
	DumpPDF     bool      // Dump PDF structure for debugging
	LogWarnings bool      // Whether to print warnings
	Logger      io.Writer // Custom logger for warnings (nil = stdout)
	Font        FontConfig
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() OCRConfig {
	return OCRConfig{
		Debug:       false,
		Force:       false,
		LayerName:   "OCR Text", // Will be formatted as "OCR Text (Page X)" in the final PDF
		StartPage:   1,
		DumpPDF:     false,
		LogWarnings: true,
		Logger:      nil, // stdout
		Font:        DefaultFont,
	}
}

// FontConfig contains font settings for OCR text rendering
type FontConfig struct {
	Name        string  // Font name (e.g., "Helvetica")
	Style       string  // Font style ("", "B", "I", "BI")
	Size        float64 // Default font size
	AscentRatio float64 // Vertical positioning ratio
}

// DefaultFont sets the default font to Helvetica which is tried and tested for the OCR layer
var DefaultFont = FontConfig{
	Name:        "Helvetica",
	Style:       "",
	Size:        10,
	AscentRatio: 0.718,
}
