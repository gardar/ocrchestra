package pdfocr

import (
	"fmt"

	"codeberg.org/go-pdf/fpdf"
	"golang.org/x/text/encoding/charmap"

	"github.com/gardar/ocrchestra/pkg/hocr"
)

// drawOCRLayer draws the OCR text onto a layer in a pdf page.
// The pageNum parameter is used to create unique layer names for each page.
func drawOCRLayer(
	pdf *fpdf.Fpdf,
	page hocr.Page,
	debug bool,
	layerName string,
	pageNum int,
	transform func(x, y float64) (float64, float64),
	fontConfig FontConfig,
) error {
	// Format layer name with page number if not already included
	formattedLayerName := layerName
	if pageNum > 0 {
		formattedLayerName = fmt.Sprintf("%s (Page %d)", layerName, pageNum)
	}

	layer := pdf.AddLayer(formattedLayerName, true)
	pdf.BeginLayer(layer)
	pdf.SetFont(fontConfig.Name, fontConfig.Style, fontConfig.Size)

	if debug {
		pdf.SetTextColor(255, 0, 0) // highlight text in red
	} else {
		pdf.SetAlpha(0.0, "Normal") // hide text from normal view
	}

	encodingErrors := 0
	wordCount := 0

	// Process words from areas
	for _, area := range page.Areas {
		// Words directly under area
		for _, word := range area.Words {
			drawWord(pdf, word, transform, fontConfig, debug, &encodingErrors)
			wordCount++
		}

		// Words in lines under area
		for _, line := range area.Lines {
			for _, word := range line.Words {
				drawWord(pdf, word, transform, fontConfig, debug, &encodingErrors)
				wordCount++
			}
		}

		// Process words from paragraphs under area
		for _, paragraph := range area.Paragraphs {
			// Words directly under paragraph
			for _, word := range paragraph.Words {
				drawWord(pdf, word, transform, fontConfig, debug, &encodingErrors)
				wordCount++
			}

			// Words in lines under paragraph
			for _, line := range paragraph.Lines {
				for _, word := range line.Words {
					drawWord(pdf, word, transform, fontConfig, debug, &encodingErrors)
					wordCount++
				}
			}
		}
	}

	// Process words from paragraphs directly under page
	for _, paragraph := range page.Paragraphs {
		// Words directly under paragraph
		for _, word := range paragraph.Words {
			drawWord(pdf, word, transform, fontConfig, debug, &encodingErrors)
			wordCount++
		}

		// Words in lines under paragraph
		for _, line := range paragraph.Lines {
			for _, word := range line.Words {
				drawWord(pdf, word, transform, fontConfig, debug, &encodingErrors)
				wordCount++
			}
		}
	}

	// Process words from lines directly under page
	for _, line := range page.Lines {
		for _, word := range line.Words {
			drawWord(pdf, word, transform, fontConfig, debug, &encodingErrors)
			wordCount++
		}
	}

	pdf.EndLayer()

	// Report encoding errors if more than a threshold
	if wordCount > 0 && encodingErrors > 0 && encodingErrors > wordCount/10 {
		return fmt.Errorf("character encoding issues in %d of %d words",
			encodingErrors, wordCount)
	}

	return nil
}

// drawWord renders a single word onto the PDF layer
func drawWord(pdf *fpdf.Fpdf, word hocr.Word, transform func(x, y float64) (float64, float64),
	fontConfig FontConfig, debug bool, encodingErrors *int) {

	x, y := transform(word.BBox.X1, word.BBox.Y1)
	x2, _ := transform(word.BBox.X2, word.BBox.Y1)
	wordWidth := x2 - x

	// Convert text to ISO-8859-1 to avoid PDF encoding issues
	latin1, err := charmap.ISO8859_1.NewEncoder().String(word.Text)
	if err != nil {
		// Track encoding errors but continue
		*encodingErrors++
		latin1 = word.Text // fallback to raw text
	}

	strWidth := pdf.GetStringWidth(latin1)
	if strWidth > 0 {
		scale := wordWidth / strWidth
		pdf.SetFontSize(fontConfig.Size * scale)
	}

	fontSize, _ := pdf.GetFontSize()
	y += fontSize * fontConfig.AscentRatio

	pdf.Text(x, y, latin1)
	pdf.SetFontSize(fontConfig.Size)

	if debug {
		height := word.BBox.Y2 - word.BBox.Y1
		pdf.Rect(x, y-(fontSize*fontConfig.AscentRatio), wordWidth, height, "D")
	}
}
