package pdfocr

import (
	"bytes"
	"codeberg.org/go-pdf/fpdf"
	"fmt"
	"github.com/gardar/ocrchestra/pkg/hocr"
	"image"
	"strings"
)

// createPDFFromImage builds a new PDF from images with their corresponding OCR data.
// This function assumes inputs have been validated by the caller.
func createPDFFromImage(
	hOCRData hocr.HOCR,
	imagesData [][]byte,
	startFromPage int,
	debug bool,
	layerName string,
	fontConfig FontConfig,
) ([]byte, error) {
	startIdx := startFromPage - 1
	pdf := fpdf.New("P", "pt", "A4", "")

	for i := startIdx; i < len(hOCRData.Pages) && i < len(imagesData); i++ {
		page := hOCRData.Pages[i]
		w, h := page.BBox.X2, page.BBox.Y2

		// Calculate the actual page number (1-based, accounting for startFromPage)
		actualPageNum := i + 1 // 1-based page number in the resulting PDF

		// Add page with appropriate dimensions
		pdf.AddPageFormat("P", fpdf.SizeType{Wd: w, Ht: h})

		// Add image to page
		imageName := fmt.Sprintf("img%d", i)
		imageType, err := detectImageType(imagesData[i])
		if err != nil {
			// This should rarely happen since validation should be done at the higher level
			return nil, fmt.Errorf("failed to detect image type for image %d: %w", i, err)
		}

		opts := fpdf.ImageOptions{ReadDpi: false, ImageType: imageType}
		pdf.RegisterImageOptionsReader(imageName, opts, bytes.NewReader(imagesData[i]))
		pdf.ImageOptions(imageName, 0, 0, w, h, false, opts, 0, "")

		// Create transformation function for this page
		transform := func(x, y float64) (float64, float64) {
			return normalizeCoords(x, y, w, h, w, h)
		}

		// Add OCR layer with page number
		err = drawOCRLayer(pdf, page, debug, layerName, actualPageNum, transform, fontConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to draw OCR layer for page %d: %w", i+1, err)
		}
	}

	// Generate final PDF
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("failed to generate PDF: %w", err)
	}
	return buf.Bytes(), nil
}

// detectImageType tries to figure out whether the data is PNG, JPEG, etc.
func detectImageType(data []byte) (string, error) {
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to decode image config: %w", err)
	}
	return strings.ToUpper(format), nil
}
