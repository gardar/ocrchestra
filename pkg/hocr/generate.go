package hocr

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed templates/hocr.tmpl
var templateFS embed.FS

// GenerateHOCRDocument creates an hOCR HTML document from the HOCR struct
// Uses the embedded template to generate a complete HTML document
func GenerateHOCRDocument(doc *HOCR) (string, error) {
	// Set up the template with a helper function
	tmpl, err := template.New("hocr.tmpl").Funcs(template.FuncMap{
		"trim": strings.TrimSpace,
	}).ParseFS(templateFS, "templates/hocr.tmpl")
	if err != nil {
		return "", fmt.Errorf("error parsing hOCR template: %w", err)
	}

	// Render the template with the hOCR data
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, doc); err != nil {
		return "", fmt.Errorf("error rendering hOCR template: %w", err)
	}

	return buf.String(), nil
}
