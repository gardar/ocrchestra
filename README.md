# OCRchestra

OCR pieces working together in harmony - A Go toolkit for handling OCR workflows.

> **⚠️ Alpha Status Notice**: This module/library is currently in alpha status. APIs may change or break without prior notice. Use in production at your own risk.

## Overview

OCRchestra is a collection of Go packages designed to make working with OCR (Optical Character Recognition) easier. It currently provides tools for:

- PDF OCR manipulation with selectable / searchable text layers.
- Working with hOCR format (HTML-based OCR result representation)
- Processing documents with Google Document AI and applying OCR.


## Installation

```bash
go get github.com/gardar/ocrchestra
```

## Requirements

- Go 1.18+
- For Google Document AI features:
  - Google Cloud account with Document AI API enabled
  - A Google Document AI processor (Document OCR, Custom extractor, Form Parser, etc.)
  - Environment variable `GOOGLE_APPLICATION_CREDENTIALS` pointing to your service account key file
  
  **Setting up Google Cloud credentials:**
  ```bash
  # Install Google Cloud CLI if you don't have it
  # https://cloud.google.com/sdk/docs/install
  
  # Login and generate application default credentials
  gcloud auth application-default login
  
  # This creates credentials at:
  # ~/.config/gcloud/application_default_credentials.json
  
  # The GOOGLE_APPLICATION_CREDENTIALS environment variable
  # will automatically use this file by default
  # To use a different credentials file:
  export GOOGLE_APPLICATION_CREDENTIALS="/path/to/credentials.json"
  ```
  
- For PDF manipulation:
  - No external dependencies (uses pure Go libraries)

## Command Line Tools

OCRchestra includes command-line utilities that provide quick access to OCR functionality without writing code.

### gdocai
The `gdocai` tool processes documents with Google Document AI and applies OCR to create searchable PDFs.

Key features:
- Process single PDFs or multiple PDF files as individual pages
- Extract OCR text, form fields, custom extractor fields, and hOCR data
- Create searchable PDFs by applying OCR text layers and optionally use extracted fields in the PDF name
- Save page images from processed documents
- Debug Document AI processing with detailed JSON output

The tool can be configured using either a YAML configuration file or environment variables, and uses the `GOOGLE_APPLICATION_CREDENTIALS` environment variable for authentication.

#### Configuration Options

**YAML Configuration** (via -config flag):
```yaml
project_id: "your-gcp-project-id"
location: "us"
processor_id: "your-processor-id"
```

**Environment Variables:**
```bash
GDOCAI_PROJECT_ID=your-gcp-project-id
GDOCAI_LOCATION=us
GDOCAI_PROCESSOR_ID=your-processor-id
```

If both config file and environment variables are provided, values from the config file take precedence.

#### Placeholder substitution

You can inject extracted fields into your output filenames. Supported syntax:

- `@{field_name}`
  Auto-detect source (form vs. custom extractor).
- `@{field_name:default_value}`
  As above, but if no value is found, uses `default_value`.
- `@{form_field.field_name}`
  Force use of a form-extracted field.
- `@{extractor_field.field_name}`
  Force use of a custom-extractor field.

#### OCR Detection

`gdocai` can detect if a PDF already has an OCR text layer before applying a new one. This helps prevent duplicate OCR layers which can cause issues with text search and selection in some PDF viewers.

- By default, when OCR is detected, `gdocai` will print a warning but continue processing, exiting with code 2 to indicate success with a warning
- The `-strict` flag can be used to make `gdocai` exit with an error (code 3) if OCR is already present, preventing duplicate OCR layers
- The `-force` flag can be used to ensure processing continues even when OCR is already detected
- If both `-strict` and `-force` flags are specified, `-force` takes precedence, allowing processing to continue
- OCR detection is performed for both single PDFs (with `-pdf`) and individual pages when using multiple source files (with `-pdfs`)

```
gdocai -config config.yml -pdf document.pdf -output document_searchable.pdf -strict
gdocai -config config.yml -pdf document.pdf -output document_searchable.pdf -force
```

#### Exit Codes

`gdocai` uses the following exit codes:

| Code | Meaning |
|------|---------|
| 0    | Success - normal operation completed without issues |
| 1    | Error - operation failed due to an error |
| 2    | Warning - OCR was detected but processing completed successfully |
| 3    | Error - OCR was detected in strict mode, processing terminated |


#### Examples
```bash
# Process a single PDF and create a searchable version (using YAML config)
gdocai -config config.yml -pdf document.pdf -output searchable.pdf

# Process a single PDF using environment variables instead
export GDOCAI_PROJECT_ID=your-project-id
export GDOCAI_LOCATION=us
export GDOCAI_PROCESSOR_ID=your-processor-id
gdocai -pdf document.pdf -output searchable.pdf

# Process multiple PDFs as separate pages in a single document
gdocai -config config.yml -pdfs "page1.pdf,page2.pdf,page3.pdf" -output combined.pdf

# One-liner with environment variables (useful in containers)
GDOCAI_PROJECT_ID=your-project GDOCAI_LOCATION=us GDOCAI_PROCESSOR_ID=your-processor gdocai -pdf invoice.pdf -output "invoice-@{invoice_number:unknown}.pdf"

# Use extracted fields in the output PDF name
gdocai -config cfg.yml -pdf invoice.pdf -output "invoice-@{invoice_number:unknown}-@{date}.pdf"

# Extract OCR text, hOCR, form fields, and custom extractor fields
gdocai -config config.yml -pdf form.pdf -text form.txt -hocr form.hocr -form-fields form.json -extractor-fields extractor.json

# Extract images from each page
gdocai -config config.yml -pdf document.pdf -images ./pages/

# Debug the Document AI processing
gdocai -config config.yml -pdf document.pdf -debug-api api_response.json -debug-doc document_structure.json
```

### pdfocr
The `pdfocr` tool creates searchable PDFs with OCR text layers, using hOCR data to position text accurately.

Key features:
- Enhance existing PDFs with OCR text layers
- Create new PDFs from images with embedded OCR text layer
- Position text at the exact location of each recognized word
- Debug mode to visualize OCR bounding boxes
- Detect existing OCR layers to prevent duplication

The tool works with hOCR files generated from any OCR system, including those produced by the `gdocai` tool.

#### OCR Detection

`pdfocr` can detect if a PDF already has an OCR text layer before adding a new one. This helps prevent duplicate OCR layers which can cause issues with text search and selection in some PDF viewers.

- By default, when OCR is detected, a warning is displayed but processing continues
- Use the `-strict` flag to make pdfocr exit with an error if OCR is already present
- Use the `-force` flag to apply OCR even when an existing layer is detected
- The `-strict` and `-force` flags can be combined in special cases: if both are specified, `-force` takes precedence, allowing OCR application regardless of detection results

```bash
# Exit with error if OCR is already present
pdfocr -hocr document.hocr -pdf document.pdf -output searchable.pdf -strict

# Force application of OCR even if already present
pdfocr -hocr document.hocr -pdf document.pdf -output searchable.pdf -force

# Force takes precedence over strict
pdfocr -hocr document.hocr -pdf document.pdf -output searchable.pdf -strict -force
```

#### Exit Codes

`pdfocr` uses the following exit codes:

| Code | Meaning |
|------|---------|
| 0    | Success - normal operation completed without issues |
| 1    | Error - operation failed due to an error |
| 2    | Warning - OCR was detected but processing completed successfully |
| 3    | Error - OCR was detected in strict mode, processing terminated |

#### Example
```bash
# Apply OCR layer to an existing PDF
pdfocr -hocr document.hocr -pdf document.pdf -output searchable.pdf

# Create a PDF from a directory of images
pdfocr -hocr document.hocr -image-dir ./page_images -output document_from_images.pdf

# Debug mode (shows bounding boxes)
pdfocr -hocr document.hocr -pdf document.pdf -output searchable.pdf -debug

# Force reapplication of OCR layer
pdfocr -hocr document.hocr -pdf document.pdf -output searchable.pdf -force
```

## Packages

### gdocai
The `gdocai` package provides comprehensive integration with Google Document AI for document processing:

- Process PDFs with Google Document AI to extract text and structural information
- Extract form fields from documents with form elements
- Extract custom fields from custom extractors with support for nested hierarchies
- Generate hOCR data for advanced OCR workflows
- Convert Document AI output to standard formats (plain text and hOCR)
- Access the full hierarchical structure of document content (blocks, paragraphs, lines, words)
- Extract page images for further processing
- Create searchable and selectable PDFs

Main functions include `DocumentHOCR` for processing complete documents, `DocumentHOCRFromPages` for processing multiple PDFs as a single document, and utilities for extracting form fields, custom extractor fields, and page images.

> **Note**: The structured document model in `gdocai` was initially inspired by Google's Document AI toolbox for Python. While the original implementation generated hOCR directly from this structured document, OCRchestra has evolved to feature a separate, standalone `hocr` package with its own data structures, parser, and renderer. This architectural change allows the `hocr` package to work independently from `gdocai`, providing greater flexibility for various OCR workflows.
#### Example
```go
import (
    "context"
    "os"
    
    "github.com/gardar/ocrchestra/pkg/gdocai"
    "github.com/gardar/ocrchestra/pkg/pdfocr"
)

// Configure Document AI
config := &gdocai.Config{
    ProjectID:   "your-gcp-project",
    Location:    "us",
    ProcessorID: "your-processor-id",
}

// Read the PDF file
pdfBytes, _ := os.ReadFile("document.pdf")

// Process with Google Document AI
doc, hocrHTML, _ := gdocai.DocumentHOCR(context.Background(), pdfBytes, config)

// Approach 1: Direct OCR application
// Apply OCR directly using the Document AI results
ocrPDF, _ := pdfocr.ApplyOCR(pdfBytes, doc.Hocr.Content, pdfocr.DefaultConfig())
os.WriteFile("searchable.pdf", ocrPDF, 0644)

// Approach 2: Step-by-step with intermediate files
// 1. Save the hOCR data
os.WriteFile("document.hocr", []byte(hocrHTML), 0644)

// 2. Extract images from each page
var images [][]byte
for i, page := range doc.Structured.Pages {
    imgBytes, _ := gdocai.ExtractImageFromPage(page)
    images = append(images, imgBytes)
    os.WriteFile(fmt.Sprintf("page_%d.png", i+1), imgBytes, 0644)
}

// 3. Create a searchable PDF from the hOCR and images
ocrPDFFromImages, _ := pdfocr.AssembleWithOCR(
    doc.Hocr.Content, 
    images, 
    pdfocr.DefaultConfig(),
)
os.WriteFile("searchable_from_images.pdf", ocrPDFFromImages, 0644)
```


### hocr
The `hocr` package implements parsing, manipulation, and generation of hOCR format data, an HTML-based standard for representing OCR results.

The package provides a complete object model representing the hOCR hierarchy:
- Document → Pages → Areas → Paragraphs → Lines → Words
- Each element has positioning data and optional metadata
- Bounding boxes and coordinates for all elements
- Support for language, confidence values, and other hOCR attributes

Main functions include `ParseHOCR` for converting hOCR HTML into structured data and `GenerateHOCRDocument` for creating valid hOCR HTML from the object model.
#### Example
```go
import "github.com/gardar/ocrchestra/pkg/hocr"

// Parse hOCR data into structured object model
hocrData, err := hocr.ParseHOCR(hocrBytes)
if err != nil {
    // Handle error
}

// Access the document structure
for _, page := range hocrData.Pages {
    fmt.Printf("Page %d has %d areas\n", page.PageNumber, len(page.Areas))
    
    // Navigate the hierarchical structure
    for _, area := range page.Areas {
        for _, paragraph := range area.Paragraphs {
            for _, line := range paragraph.Lines {
                for _, word := range line.Words {
                    // Access word text and position
                    fmt.Printf("Word: %s at position (%f,%f)-(%f,%f)\n",
                        word.Text, word.BBox.X1, word.BBox.Y1,
                        word.BBox.X2, word.BBox.Y2)
                }
            }
        }
    }
}

// Modify the data structure
hocrData.Pages[0].Areas[0].Paragraphs[0].Lines[0].Words[0].Text = "Modified"

// Generate hOCR HTML from the object model
html, err := hocr.GenerateHOCRDocument(&hocrData)
if err != nil {
    // Handle error
}
```

### pdfocr
The `pdfocr` package handles adding OCR text layers to PDFs from hOCR data.

It can either enhance existing PDFs with OCR text layers or create new PDFs from images with an OCR text layer. The resulting PDFs have text precisely positioned over the original document that is:
- Fully searchable
- Selectable with mouse drag operations
- Can be toggled on/off in compatible PDF readers

Main functions include `ApplyOCR` for adding OCR text to existing PDFs, `AssembleWithOCR` for creating new PDFs from images with OCR text layers and `DetectOCR` to detect if OCR has already been applied to a PDF.
#### Example
```go
import "github.com/gardar/ocrchestra/pkg/pdfocr"

// Add OCR layer to an existing PDF
config := pdfocr.DefaultConfig()
pdfWithOCR, err := pdfocr.ApplyOCR(pdfBytes, hocrData, config)
if err != nil {
    // Handle error
}

// Create a new PDF from images with OCR text layer
pdfDoc, err := pdfocr.AssembleWithOCR(hocrData, imageBytes, config)
if err != nil {
    // Handle error
}
```

## License

[Mozilla Public License 2.0](LICENSE)
