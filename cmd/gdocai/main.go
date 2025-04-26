// gdocai is a command-line tool for processing documents with Google Document AI and applying the OCR to the documents.
//
// This tool extracts text, form fields, custom extractor fields, and hOCR data from PDFs using Google's Document AI API.
// It supports applying the OCR text from Google's Document AI to create searchable PDFs and can optionally extract a hOCR document,
// form field data, custom extractor data, and text.
//
// Configuration:
//
// The tool requires a YAML configuration file with Google Document AI settings:
//
//	project_id: "your-gcp-project-id"
//	location: "us"
//	processor_id: "your-processor-id"
//
// Usage:
//
//	gdocai -config config.yml -pdf input.pdf [options]
//
// Required flags:
//
//	-config string  Path to the YAML configuration file
//	-pdf string     Path to the input PDF file (required if -pdfs is not defined)
//	-pdfs string    Comma separated list of input PDF files to process as a single document (required if -pdf is not defined)
//
// Output options (at least one required):
//
//	-text string             Path to save OCR text output
//	-hocr string             Path to save HOCR output
//	-form-fields string      Path to save form fields JSON
//	-extractor-fields string Path to save custom extractor fields JSON
//	-images string           Directory to save page images
//	-output string           Path to save the PDF with OCR applied
//
// Debug options:
//
//	-debug-api string   Path to save raw API response as JSON
//	-debug-doc string   Path to save transformed Document object as JSON
//
// Authentication:
//
// The tool uses the GOOGLE_APPLICATION_CREDENTIALS environment variable
// for authentication with Google Cloud.
//
// Example:
//
//	export GOOGLE_APPLICATION_CREDENTIALS=/path/to/credentials.json
//	gdocai -config config.yml -pdf document.pdf -text document.txt -hocr document.hocr -output document_ocr.pdf
//	gdocai -config config.yml -pdfs page1.pdf,page2.pdf,page3.pdf -output combo_document_ocr.pdf
//	gdocai -config config.yml -pdf form.pdf -form-fields fields.json -extractor-fields entities.json

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/gardar/ocrchestra/pkg/gdocai"
	"github.com/gardar/ocrchestra/pkg/pdfocr"
)

type yamlConfig struct {
	ProjectID   string `yaml:"project_id"`
	Location    string `yaml:"location"`
	ProcessorID string `yaml:"processor_id"`
}

// loadConfig reads a YAML file and converts it to our Google Document AI config
func loadConfig(path string) (*gdocai.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var yc yamlConfig
	if err := yaml.Unmarshal(data, &yc); err != nil {
		return nil, err
	}
	return &gdocai.Config{
		ProjectID:   yc.ProjectID,
		Location:    yc.Location,
		ProcessorID: yc.ProcessorID,
	}, nil
}

func main() {
	// Required flags.
	configPath := flag.String("config", "", "Path to the config YAML file (required)")
	pdfPath := flag.String("pdf", "", "Path to the input PDF file (required if -pdfs not specified)")
	pdfPaths := flag.String("pdfs", "", "Comma-separated list of PDF files to process as individual pages (required if -pdf not specified)")

	// Output flags
	textPath := flag.String("text", "", "Path to save OCR text output")
	hocrPath := flag.String("hocr", "", "Path to save HOCR output")
	debugAPIPath := flag.String("debug-api", "", "Path to save API response as JSON for debugging purposes")
	debugDocPath := flag.String("debug-doc", "", "Path to save transformed Document object as JSON for debugging purposes")
	formFieldsPath := flag.String("form-fields", "", "Path to save form fields JSON")
	extractorFieldsPath := flag.String("extractor-fields", "", "Path to save custom extractor fields JSON")
	imagesDir := flag.String("images", "", "Directory to save images returned by Document AI API for each processed page")
	pdfOcrPath := flag.String("output", "", "Path to save the PDF with OCR applied")

	flag.Parse()

	// Create a map of provided flags to validate
	providedFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		providedFlags[f.Name] = true
	})

	// Validate that config is provided
	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "Error: -config flag is required")
		fmt.Fprintln(os.Stderr, "Usage:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Validate that either pdf or pdfs flag is provided (but not both)
	if (*pdfPath == "" && *pdfPaths == "") || (*pdfPath != "" && *pdfPaths != "") {
		fmt.Fprintln(os.Stderr, "Error: Either -pdf or -pdfs flag must be provided (but not both)")
		fmt.Fprintln(os.Stderr, "Usage:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Validate that provided output flags have values
	hasError := false
	validateFlag := func(name string, value string) {
		if providedFlags[name] && value == "" {
			fmt.Fprintf(os.Stderr, "Error: -%s flag requires a value\n", name)
			hasError = true
		}
	}

	validateFlag("text", *textPath)
	validateFlag("hocr", *hocrPath)
	validateFlag("debug-api", *debugAPIPath)
	validateFlag("debug-doc", *debugDocPath)
	validateFlag("form-fields", *formFieldsPath)
	validateFlag("extractor-fields", *extractorFieldsPath)
	validateFlag("images", *imagesDir)
	validateFlag("output", *pdfOcrPath)

	if hasError {
		fmt.Fprintln(os.Stderr, "Usage:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Check if at least one output flag is provided
	hasOutputFlag := providedFlags["text"] || providedFlags["hocr"] ||
		providedFlags["debug-api"] || providedFlags["debug-doc"] ||
		providedFlags["form-fields"] || providedFlags["extractor-fields"] ||
		providedFlags["images"] || providedFlags["output"]

	if !hasOutputFlag {
		fmt.Fprintln(os.Stderr, "Error: At least one output flag must be provided (-text, -hocr, -debug-api, -debug-doc, -form-fields, -images, or -output)")
		fmt.Fprintln(os.Stderr, "Usage:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Load config from file.
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Process the document based on input flags
	ctx := context.Background()
	var doc *gdocai.Document
	var hocrHTML string

	if *pdfPath != "" {
		// Process a single PDF file
		fmt.Println("Processing single PDF file:", *pdfPath)

		// Read PDF bytes from disk.
		pdfBytes, err := os.ReadFile(*pdfPath)
		if err != nil {
			log.Fatalf("Failed to read PDF file: %v", err)
		}

		// Process the PDF using Google Document AI.
		doc, hocrHTML, err = gdocai.DocumentHOCR(ctx, pdfBytes, cfg)
		if err != nil {
			log.Fatalf("Error processing document: %v", err)
		}
	} else {
		// Process multiple PDF files as individual pages
		pathsList := strings.Split(*pdfPaths, ",")
		if len(pathsList) == 0 {
			log.Fatalf("No PDF files specified with -pdfs")
		}

		fmt.Printf("Processing %d PDF files as separate pages\n", len(pathsList))

		// Read each PDF file
		var pdfPageBytes [][]byte
		for i, path := range pathsList {
			// Trim any whitespace
			path = strings.TrimSpace(path)
			if path == "" {
				continue
			}

			fmt.Printf("Reading page %d: %s\n", i+1, path)
			pageBytes, err := os.ReadFile(path)
			if err != nil {
				log.Fatalf("Failed to read PDF file %s: %v", path, err)
			}
			pdfPageBytes = append(pdfPageBytes, pageBytes)
		}

		if len(pdfPageBytes) == 0 {
			log.Fatalf("No valid PDF files found in the provided list")
		}

		// Process the PDFs using DocumentHOCRFromPages
		doc, hocrHTML, err = gdocai.DocumentHOCRFromPages(ctx, pdfPageBytes, cfg)
		if err != nil {
			log.Fatalf("Error processing documents: %v", err)
		}
	}

	// Write OCR text output if flag is provided.
	if *textPath != "" {
		if err := os.WriteFile(*textPath, []byte(doc.Text.Content), 0644); err != nil {
			log.Fatalf("Failed to write text output: %v", err)
		}
		fmt.Println("Document text saved to:", *textPath)
	}

	// Write hOCR output if flag is provided.
	if *hocrPath != "" {
		if err := os.WriteFile(*hocrPath, []byte(hocrHTML), 0644); err != nil {
			log.Fatalf("Failed to write HOCR output: %v", err)
		}
		fmt.Println("Rendered HOCR output saved to:", *hocrPath)
	}

	// Write API response JSON if flag is provided.
	if *debugAPIPath != "" {
		// Note: When using DocumentHOCRFromPages, the Raw.Document field may be nil
		if doc.Raw != nil && doc.Raw.Document != nil {
			apiJSON, err := gdocai.ToJSON(doc.Raw.Document)
			if err != nil {
				log.Fatalf("Failed to convert API response to JSON: %v", err)
			}
			if err := os.WriteFile(*debugAPIPath, []byte(apiJSON), 0644); err != nil {
				log.Fatalf("Failed to write API response JSON: %v", err)
			}
			fmt.Println("API response JSON saved to:", *debugAPIPath)
		} else {
			fmt.Println("Warning: Raw API response not available when processing multiple PDF files")
		}
	}

	// Write transformed Document JSON if flag is provided.
	if *debugDocPath != "" {
		debugJSON, err := gdocai.ToJSON(doc)
		if err != nil {
			log.Fatalf("Failed to convert transformed document to JSON: %v", err)
		}
		if err := os.WriteFile(*debugDocPath, []byte(debugJSON), 0644); err != nil {
			log.Fatalf("Failed to write transformed document JSON: %v", err)
		}
		fmt.Println("Transformed document JSON saved to:", *debugDocPath)
	}

	// Write form fields JSON if flag is provided.
	if *formFieldsPath != "" {
		formFieldsJSON, err := gdocai.ToJSON(doc.FormFields.Fields)
		if err != nil {
			log.Fatalf("Failed to convert form fields to JSON: %v", err)
		}
		if err := os.WriteFile(*formFieldsPath, []byte(formFieldsJSON), 0644); err != nil {
			log.Fatalf("Failed to write form fields JSON: %v", err)
		}
		fmt.Println("Form fields JSON saved to:", *formFieldsPath)
	}

	// Write custom extractor fields JSON if flag is provided.
	if *extractorFieldsPath != "" {
		extractorFieldsJSON, err := gdocai.ToJSON(doc.CustomExtractorFields.Fields)
		if err != nil {
			log.Fatalf("Failed to convert custom extractor fields to JSON: %v", err)
		}
		if err := os.WriteFile(*extractorFieldsPath, []byte(extractorFieldsJSON), 0644); err != nil {
			log.Fatalf("Failed to write custom extractor fields JSON: %v", err)
		}
		fmt.Println("Custom extractor fields JSON saved to:", *extractorFieldsPath)
	}

	// Extract and write out images for each page if flag is provided.
	if *imagesDir != "" {
		// Ensure output directory exists.
		if err := os.MkdirAll(*imagesDir, 0755); err != nil {
			log.Fatalf("Failed to create images directory: %v", err)
		}

		// Check if we have structured pages to extract images from
		if doc.Structured != nil && doc.Structured.Pages != nil {
			// Iterate over each internal page in the document.
			for i, page := range doc.Structured.Pages {
				imgBytes, err := gdocai.ExtractImageFromPage(page)
				if err != nil {
					log.Printf("Skipping page %d: %v", i+1, err)
					continue
				}
				imagePath := filepath.Join(*imagesDir, fmt.Sprintf("page_%d.png", i+1))
				if err := os.WriteFile(imagePath, imgBytes, 0644); err != nil {
					log.Printf("Failed to write image for page %d: %v", i+1, err)
					continue
				}
				fmt.Printf("Saved image for page %d to %s\n", i+1, imagePath)
			}
		} else {
			fmt.Println("Warning: No page images available to extract")
		}
	}

	// Generate a new OCR'ed PDF if flag is provided.
	if *pdfOcrPath != "" {
		if doc.Hocr != nil && doc.Hocr.Content != nil {
			var ocrPdfBytes []byte
			var err error

			if *pdfPath != "" && (*pdfPaths == "" || !providedFlags["pdfs"]) {
				// Single PDF case - use ApplyOCR to modify the existing PDF
				fmt.Println("Creating searchable PDF by applying OCR to existing PDF...")

				// Read the original PDF
				pdfBytes, err := os.ReadFile(*pdfPath)
				if err != nil {
					log.Fatalf("Failed to read PDF file: %v", err)
				}

				// Apply OCR to the PDF
				ocrPdfBytes, err = pdfocr.ApplyOCR(pdfBytes, doc.Hocr.Content, pdfocr.DefaultConfig())
				if err != nil {
					log.Fatalf("Failed to apply OCR to PDF: %v", err)
				}
			} else {
				// Multiple PDFs case - create a new PDF from page images
				fmt.Println("Creating new searchable PDF from Document AI page images...")

				// Get images from Document AI results (in memory only)
				var pageImages [][]byte

				if doc.Structured != nil && doc.Structured.Pages != nil {
					for i, page := range doc.Structured.Pages {
						imgBytes, err := gdocai.ExtractImageFromPage(page)
						if err != nil {
							log.Fatalf("Failed to get image data for page %d: %v", i+1, err)
						}
						pageImages = append(pageImages, imgBytes)
						fmt.Printf("Using image data for page %d (%d bytes)\n", i+1, len(imgBytes))
					}
				} else {
					log.Fatalf("No page image data available in the document structure")
				}

				// Verify we have images for all pages
				if len(pageImages) == 0 {
					log.Fatalf("No page image data was found")
				}

				fmt.Printf("Assembling PDF with %d pages...\n", len(pageImages))

				// Use AssembleWithOCR to create a new PDF from images
				ocrConfig := pdfocr.DefaultConfig()
				ocrPdfBytes, err = pdfocr.AssembleWithOCR(doc.Hocr.Content, pageImages, ocrConfig)
				if err != nil {
					log.Fatalf("Failed to create PDF from images: %v", err)
				}
			}

			// Write the final PDF
			if err := os.WriteFile(*pdfOcrPath, ocrPdfBytes, 0644); err != nil {
				log.Fatalf("Failed to write OCR'ed PDF: %v", err)
			}
			fmt.Println("OCR'ed PDF saved to:", *pdfOcrPath)
		} else {
			log.Fatalf("HOCR content not available for creating searchable PDF")
		}
	}
}
