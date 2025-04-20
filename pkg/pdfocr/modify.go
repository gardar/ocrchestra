package pdfocr

import (
	"bytes"
	"io"

	"codeberg.org/go-pdf/fpdf"
	"codeberg.org/go-pdf/fpdf/contrib/gofpdi"

	"github.com/gardar/ocrchestra/pkg/hocr"
)

// modifyExistingPDF imports pages from an existing PDF and overlays OCR text layer.
func modifyExistingPDF(
	inputPDFData []byte,
	hOCRData hocr.HOCR,
	startFromPage int,
	debug bool,
	layerName string,
	fontConfig FontConfig,
) ([]byte, error) {

	pdf := fpdf.New("P", "pt", "", "")
	importer := gofpdi.NewImporter()
	rs := io.ReadSeeker(bytes.NewReader(inputPDFData))

	for i, page := range hOCRData.Pages {
		targetPage := i + startFromPage

		// Calculate the actual page number in the PDF
		actualPageNum := i + 1 // 1-based page number in the resulting PDF

		pdf.AddPageFormat("P", fpdf.SizeType{Wd: page.BBox.X2, Ht: page.BBox.Y2})

		tpl := importer.ImportPageFromStream(pdf, &rs, targetPage, "/MediaBox")
		importer.UseImportedTemplate(pdf, tpl, 0, 0, page.BBox.X2, 0)

		identity := func(x, y float64) (float64, float64) {
			return x, y
		}

		// Pass the page number to drawOCRLayer
		drawOCRLayer(pdf, page, debug, layerName, actualPageNum, identity, fontConfig)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
