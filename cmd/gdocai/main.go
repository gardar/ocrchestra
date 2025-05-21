// gdocai is a command-line tool for processing documents with Google Document AI and applying the OCR to the documents.
//
// This tool extracts text, form fields, custom extractor fields, and hOCR data from PDFs using Google's Document AI API.
// It supports applying the OCR text from Google's Document AI to create searchable PDFs and can optionally extract a hOCR document,
// form field data, custom extractor data, and text.
//
// Configuration:
//
// The tool can be configured using either a YAML configuration file or environment variables:
//
// YAML Configuration (via -config flag):
//
//	project_id: "your-gcp-project-id"
//	location: "us"
//	processor_id: "your-processor-id"
//
// Environment Variables:
//
//	GDOCAI_PROJECT_ID: Your GCP project ID
//	GDOCAI_LOCATION: Document AI API location (e.g., "us")
//	GDOCAI_PROCESSOR_ID: Your Document AI processor ID
//
// If both config file and environment variables are provided, values from the config file take precedence.
//
// Usage:
//
//	gdocai -config config.yml -pdf input.pdf [options]
//	# Or using environment variables:
//	GDOCAI_PROJECT_ID=your-project GDOCAI_LOCATION=us GDOCAI_PROCESSOR_ID=your-processor gdocai -pdf input.pdf [options]
//
// Required configuration (via one of these methods):
//
//	-config string  Path to the YAML configuration file
//	# OR environment variables:
//	GDOCAI_PROJECT_ID, GDOCAI_LOCATION, GDOCAI_PROCESSOR_ID
//
// Required input flags:
//
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
// Field placeholder support in output path:
//
//	The -output flag supports placeholders that use extracted field values from the document.
//	Format:
//	  @{field_name} - Use the value of field_name
//	  @{field_name:default_value} - Set default value if field is not detected
//	  @{field_name} or @{field_name:default_value} - Auto-detect source
//	  @{form_field.field_name} - Explicitly use form fields
//	  @{extractor_field.field_name} - Explicitly use custom extractor fields
//
//	Examples:
//	  -output "invoice-@{invoice_number:unknown}-@{date}.pdf"
//	  -output "client-@{form_field.client_name}-@{extractor_field.document_id}.pdf"
//
//	Field Resolution Order:
//	  1. If a field exists in both sources, a warning is shown and form fields take precedence
//	  2. If not found in the primary source, the other source is checked
//	  3. If still not found, the default value is used
//
//	Nested fields can be accessed with dot notation: @{address.city}
//
//	Filename Sanitization:
//	  All extracted field values used in output filenames are automatically sanitized to ensure
//	  they're compatible with filesystems. This includes:
//	    - Converting to lowercase
//	    - Removing path traversal components
//	    - Converting invalid filename characters to underscores
//	    - Handling Windows reserved names
//	    - Truncating overly long filenames
//	    - Removing problematic leading/trailing characters
//	    - Replacing control characters
//	    - Providing a default name if empty after sanitization
//
// OCR Detection:
//
//	-strict               Exit with error code 3 if OCR is already detected in the PDF
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
//	gdocai -config config.yml -pdf invoice.pdf -output "invoice-@{number:unknown}-@{client}.pdf"
//	gdocai -config config.yml -pdfs page1.pdf,page2.pdf,page3.pdf -output combo_document_ocr.pdf
//	gdocai -config config.yml -pdf form.pdf -form-fields fields.json -extractor-fields entities.json
//
// Using environment variables instead of config file:
//
//	export GDOCAI_PROJECT_ID=your-gcp-project-id
//	export GDOCAI_LOCATION=us
//	export GDOCAI_PROCESSOR_ID=your-processor-id
//	gdocai -pdf document.pdf -output document_ocr.pdf

package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/anyascii/go"
	"gopkg.in/yaml.v3"

	"github.com/gardar/ocrchestra/pkg/gdocai"
	"github.com/gardar/ocrchestra/pkg/pdfocr"
)

const (
	ExitCodeSuccess          = 0 // Normal successful execution
	ExitCodeError            = 1 // General error
	ExitCodeSuccessWithWarns = 2 // Success but with warnings (including OCR already detected)
	ExitCodeStrictOCRFailure = 3 // OCR already present in strict mode
)

type yamlConfig struct {
	ProjectID   string `yaml:"project_id"`
	Location    string `yaml:"location"`
	ProcessorID string `yaml:"processor_id"`
}

// warningWriter captures warnings written to the logger
type warningWriter struct {
	buf    bytes.Buffer
	target io.Writer // Usually os.Stdout
}

func newWarningWriter(target io.Writer) *warningWriter {
	return &warningWriter{
		target: target,
	}
}

func (w *warningWriter) Write(p []byte) (n int, err error) {
	// Write to both the buffer (for tracking) and the target (for display)
	w.buf.Write(p)
	return w.target.Write(p)
}

// HasWarnings checks if any warnings were logged
func (w *warningWriter) HasWarnings() bool {
	return strings.Contains(w.buf.String(), "Warning:")
}

// HasOCRWarning specifically checks if OCR already exists warning was logged
func (w *warningWriter) HasOCRWarning() bool {
	return strings.Contains(w.buf.String(), "already has OCR")
}

// PlaceholderData holds data available for placeholder substitution
type PlaceholderData struct {
	FormFields            map[string]interface{}
	CustomExtractorFields map[string]interface{}
}

// processPlaceholders takes a string with placeholders in the format:
// "@{field_name}" or "@{field_name:default_value}" - Uses prioritization rules
// "@{form_field.field_name}" - Explicitly use form fields
// "@{extractor_field.field_name}" - Explicitly use custom extractor fields
//
// It searches for values according to the specified source or using the
// prioritization rules, and if not found, uses the provided default value.
func processPlaceholders(inputStr string, data *PlaceholderData) (string, error) {
	// Regular expression to match placeholder patterns with optional source prefix and default value
	re := regexp.MustCompile(`@\{(?:(form_field|extractor_field)\.)?([^:}]+)(?::([^}]*))?\}`)

	result := re.ReplaceAllStringFunc(inputStr, func(match string) string {
		// Extract source, field name and default value from the match
		submatches := re.FindStringSubmatch(match)

		source := ""
		fieldName := ""
		defaultValue := ""

		if len(submatches) > 1 {
			source = submatches[1] // This will be "form_field", "extractor_field", or "" (for auto)
		}
		if len(submatches) > 2 {
			fieldName = strings.TrimSpace(submatches[2])
		}
		if len(submatches) > 3 && submatches[3] != "" {
			defaultValue = submatches[3]
		}

		// If explicit source is specified, only check that source
		if source == "form_field" {
			if value := lookupFieldValue(fieldName, data.FormFields); value != "" {
				return value
			}
			return defaultValue
		} else if source == "extractor_field" {
			if value := lookupFieldValue(fieldName, data.CustomExtractorFields); value != "" {
				return value
			}
			return defaultValue
		}

		// No explicit source, use prioritization rules:
		// 1. Check if exists in both - if so, log a warning and use form fields
		formValue := lookupFieldValue(fieldName, data.FormFields)
		customValue := lookupFieldValue(fieldName, data.CustomExtractorFields)

		if formValue != "" && customValue != "" {
			fmt.Printf("Warning: Field '%s' found in both form fields and custom extractor fields. Using form field value.\n", fieldName)
			return formValue
		}

		// 2. Check form fields first
		if formValue != "" {
			return formValue
		}

		// 3. Check custom extractor fields
		if customValue != "" {
			return customValue
		}

		// 4. If still not found, use default value
		return defaultValue
	})

	return result, nil
}

// lookupFieldValue attempts to find a field value in a map, potentially
// navigating nested maps using dot notation (e.g., "address.city")
func lookupFieldValue(fieldPath string, data map[string]interface{}) string {
	// Handle dot notation for nested fields
	parts := strings.Split(fieldPath, ".")

	// Start with the root data
	var current interface{} = data

	// Navigate through the parts of the path
	for _, part := range parts {
		// Check if current is a map
		if currentMap, ok := current.(map[string]interface{}); ok {
			var exists bool
			current, exists = currentMap[part]
			if !exists {
				return "" // Field not found
			}
		} else {
			return "" // Not a map, can't go deeper
		}
	}

	// Convert the final value to string
	switch v := current.(type) {
	case string:
		return v
	case []string:
		if len(v) > 0 {
			return v[0]
		}
		return ""
	case int, int64, float64:
		return fmt.Sprintf("%v", v)
	case bool:
		return fmt.Sprintf("%v", v)
	case map[string]interface{}:
		// If it's a map with a special _value key, use that
		if value, ok := v["_value"].(string); ok {
			return value
		}
		return "" // Can't convert a map to string
	default:
		// Try a generic string conversion
		return fmt.Sprintf("%v", v)
	}
}

// sanitizeFilename ensures a string can be safely used as a filename
// by transliterating Unicode characters to ASCII, enforcing lowercase,
// removing path traversal components, and replacing invalid characters
func sanitizeFilename(filename string) string {
	// If filename is empty, return a default name
	if strings.TrimSpace(filename) == "" {
		return "unnamed"
	}

	// Transliterate Unicode characters to ASCII equivalents
	filename = anyascii.Transliterate(filename)

	// Convert to lowercase
	filename = strings.ToLower(filename)

	// First remove any path traversal components
	// This is explicit even though we also handle slashes in the next step
	filename = strings.ReplaceAll(filename, "../", "")
	filename = strings.ReplaceAll(filename, "..\\", "")

	// Replace control characters (ASCII 0-31) and other problematic characters with underscores
	controlChars := regexp.MustCompile(`[\x00-\x1F\x7F<>:"/\\|?*]`)
	sanitized := controlChars.ReplaceAllString(filename, "_")

	// Collapse multiple underscores into one
	multipleUnderscores := regexp.MustCompile(`_+`)
	sanitized = multipleUnderscores.ReplaceAllString(sanitized, "_")

	// Trim leading/trailing underscores, spaces, and periods
	sanitized = strings.Trim(sanitized, "_ .")

	// Handle Windows reserved names (CON, PRN, AUX, NUL, COM1-9, LPT1-9)
	// We'll add an underscore prefix to any reserved name
	reservedNames := map[string]bool{
		"con": true, "prn": true, "aux": true, "nul": true,
		"com1": true, "com2": true, "com3": true, "com4": true, "com5": true,
		"com6": true, "com7": true, "com8": true, "com9": true,
		"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true,
		"lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
	}

	// Check if the name (without extension) is reserved
	// First separate filename and extension
	ext := filepath.Ext(sanitized)
	baseName := strings.TrimSuffix(sanitized, ext)

	// Check if base is a reserved name
	if reservedNames[baseName] {
		baseName = "_" + baseName
		sanitized = baseName + ext
	}

	// Ensure the name isn't empty after all sanitization
	if sanitized == "" {
		sanitized = "unnamed"
	}

	// Truncate if too long (safe limit for most filesystems)
	maxLength := 240 // Leave some room for extensions
	if len(sanitized) > maxLength {
		// If we have an extension, preserve it
		if ext != "" {
			// Truncate the base name part, preserving the extension
			baseName = sanitized[:maxLength-len(ext)]
			sanitized = baseName + ext
		} else {
			sanitized = sanitized[:maxLength]
		}

		// Make sure we don't truncate in the middle of a utf-8 character
		for !utf8.ValidString(sanitized) && len(sanitized) > 0 {
			sanitized = sanitized[:len(sanitized)-1]
		}
	}

	return sanitized
}

// safelyLogPath safely logs filenames
func safelyLogPath(originalPath, processedPath string) {
	// Truncate long paths for logging to avoid filling logs
	const maxDisplayLength = 100
	displayOriginal := originalPath
	displayProcessed := processedPath

	if len(originalPath) > maxDisplayLength {
		displayOriginal = originalPath[:maxDisplayLength] + "..."
	}
	if len(processedPath) > maxDisplayLength {
		displayProcessed = processedPath[:maxDisplayLength] + "..."
	}

	// Log the path transformation
	fmt.Printf("Placeholders in output path processed: %s -> %s\n", displayOriginal, displayProcessed)
}

// loadConfig reads configuration from a YAML file and/or environment variables
// and converts it to our Google Document AI config
func loadConfig(path string) (*gdocai.Config, error) {
	// Initialize configuration with environment variables (if they exist)
	config := &gdocai.Config{
		ProjectID:   os.Getenv("GDOCAI_PROJECT_ID"),
		Location:    os.Getenv("GDOCAI_LOCATION"),
		ProcessorID: os.Getenv("GDOCAI_PROCESSOR_ID"),
	}

	// If a config file path is provided, load and use it (overriding env vars)
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var yc yamlConfig
		if err := yaml.Unmarshal(data, &yc); err != nil {
			return nil, err
		}

		// Override with values from YAML if they're not empty
		if yc.ProjectID != "" {
			config.ProjectID = yc.ProjectID
		}
		if yc.Location != "" {
			config.Location = yc.Location
		}
		if yc.ProcessorID != "" {
			config.ProcessorID = yc.ProcessorID
		}
	}

	// Ensure we have the required configuration values
	if config.ProjectID == "" {
		return nil, fmt.Errorf("project_id not provided in config file or GDOCAI_PROJECT_ID environment variable")
	}
	if config.Location == "" {
		return nil, fmt.Errorf("location not provided in config file or GDOCAI_LOCATION environment variable")
	}
	if config.ProcessorID == "" {
		return nil, fmt.Errorf("processor_id not provided in config file or GDOCAI_PROCESSOR_ID environment variable")
	}

	return config, nil
}

// checkPDFForOCR checks if a PDF already has OCR
// Exits if in strict mode and OCR is found
// Returns true if OCR detected (for reporting)
func checkPDFForOCR(pdfBytes []byte, config pdfocr.OCRConfig) bool {
	ocrResult, err := pdfocr.DetectOCR(pdfBytes, config)
	if err != nil {
		fmt.Printf("Warning: OCR detection failed: %v\n", err)
		return false
	}

	if ocrResult.HasOCR {
		fmt.Printf("Warning: Document already has OCR\n")

		// In strict mode without force, exit with error
		if config.Strict && !config.Force {
			fmt.Printf("Error: Document already has OCR and strict mode is enabled\n")
			os.Exit(ExitCodeStrictOCRFailure)
		}

		return true
	}

	return false
}

func main() {
	// Override the flag usage message to include additional information
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -config config.yml -pdf input.pdf [options]\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -pdf input.pdf [options] # Using environment variables for config\n\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "Options:\n")
		flag.PrintDefaults()

		fmt.Fprintf(flag.CommandLine.Output(), "\nEnvironment Variables for Configuration:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  GDOCAI_PROJECT_ID     - Google Cloud project ID\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  GDOCAI_LOCATION       - Document AI API location (e.g., \"us\")\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  GDOCAI_PROCESSOR_ID   - Document AI processor ID\n")

		fmt.Fprintf(flag.CommandLine.Output(), "\nExit Codes:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  %d - Success\n", ExitCodeSuccess)
		fmt.Fprintf(flag.CommandLine.Output(), "  %d - Error\n", ExitCodeError)
		fmt.Fprintf(flag.CommandLine.Output(), "  %d - Success with warnings (including OCR already detected)\n", ExitCodeSuccessWithWarns)
		fmt.Fprintf(flag.CommandLine.Output(), "  %d - Error: OCR already detected in strict mode\n", ExitCodeStrictOCRFailure)

		fmt.Fprintf(flag.CommandLine.Output(), "\nExamples:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -config config.yml -pdf document.pdf -text document.txt -output document_ocr.pdf\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -pdf invoice.pdf -output \"invoice-@{number:unknown}-@{client}.pdf\"\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -pdfs page1.pdf,page2.pdf,page3.pdf -output combined.pdf\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  GDOCAI_PROJECT_ID=your-project GDOCAI_LOCATION=us GDOCAI_PROCESSOR_ID=your-processor %s -pdf document.pdf -output document_ocr.pdf\n", os.Args[0])
	}

	// Configuration flags
	configPath := flag.String("config", "", "Path to the config YAML file (optional if using environment variables)")

	// Input flags
	pdfPath := flag.String("pdf", "", "Path to the input PDF file (required if -pdfs is not defined)")
	pdfPaths := flag.String("pdfs", "", "Comma separated list of input PDF files to process as a single document (required if -pdf is not defined)")

	// Output flags with detailed descriptions
	textPath := flag.String("text", "", "Path to save OCR text output")
	hocrPath := flag.String("hocr", "", "Path to save HOCR output")
	formFieldsPath := flag.String("form-fields", "", "Path to save form fields JSON")
	extractorFieldsPath := flag.String("extractor-fields", "", "Path to save custom extractor fields JSON")
	imagesDir := flag.String("images", "", "Directory to save images returned by Document AI API for each processed page")

	// OCR detection flag
	strict := flag.Bool("strict", false, "If set, exit with error code when OCR is already detected in the PDF")
	force := flag.Bool("force", false, "Force processing even if OCR is already detected")

	// Output flag with detailed description of placeholder support
	pdfOcrPath := flag.String("output", "",
		`Path to save the PDF with OCR applied. Supports field placeholders:
  @{field_name} or @{field_name:default_value} - Auto-detect source
  @{form_field.field_name} - Explicitly use form fields
  @{extractor_field.field_name} - Explicitly use custom extractor fields
Example: -output "invoice-@{invoice_number:unknown}-@{date}.pdf"
All filenames are sanitized: Unicode characters are transliterated to ASCII,
converted to lowercase, and invalid filename characters are replaced.`)

	// Debug options
	debugAPIPath := flag.String("debug-api", "", "Path to save raw API response as JSON for debugging")
	debugDocPath := flag.String("debug-doc", "", "Path to save transformed Document object as JSON for debugging")

	// Parse command line arguments
	flag.Parse()

	// Create a map of provided flags to validate
	providedFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		providedFlags[f.Name] = true
	})

	// Validate configuration is available (either via file or env vars)
	if *configPath == "" {
		// Check if we have env vars
		hasEnvConfig := os.Getenv("GDOCAI_PROJECT_ID") != "" &&
			os.Getenv("GDOCAI_LOCATION") != "" &&
			os.Getenv("GDOCAI_PROCESSOR_ID") != ""

		if !hasEnvConfig {
			fmt.Fprintln(os.Stderr, "Error: Either -config flag or environment variables (GDOCAI_PROJECT_ID, GDOCAI_LOCATION, GDOCAI_PROCESSOR_ID) must be provided")
			flag.Usage()
			os.Exit(ExitCodeError)
		}
	}

	// Validate that either pdf or pdfs flag is provided (but not both)
	if (*pdfPath == "" && *pdfPaths == "") || (*pdfPath != "" && *pdfPaths != "") {
		fmt.Fprintln(os.Stderr, "Error: Either -pdf or -pdfs flag must be provided (but not both)")
		flag.Usage()
		os.Exit(ExitCodeError)
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
		flag.Usage()
		os.Exit(ExitCodeError)
	}

	// Check if at least one output flag is provided
	hasOutputFlag := providedFlags["text"] || providedFlags["hocr"] ||
		providedFlags["debug-api"] || providedFlags["debug-doc"] ||
		providedFlags["form-fields"] || providedFlags["extractor-fields"] ||
		providedFlags["images"] || providedFlags["output"]

	if !hasOutputFlag {
		fmt.Fprintln(os.Stderr, "Error: At least one output flag must be provided (-text, -hocr, -debug-api, -debug-doc, -form-fields, -images, or -output)")
		flag.Usage()
		os.Exit(ExitCodeError)
	}

	// Create a warning writer to capture warnings
	warningCapture := newWarningWriter(os.Stdout)

	// Build the OCRConfig for any PDF processing that might occur
	pdfOcrConfig := pdfocr.OCRConfig{
		Debug:       false,
		Force:       *force,
		Strict:      *strict,
		StartPage:   1,
		DumpPDF:     false,
		Font:        pdfocr.DefaultFont,
		LogWarnings: true,
		LayerName:   "OCR Text",
		Logger:      warningCapture, // Use our custom writer to track warnings
	}

	// Load config from file and/or environment variables
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Process the document based on input flags
	ctx := context.Background()
	var doc *gdocai.Document
	var hocrHTML string
	var hasOCR bool

	if *pdfPath != "" {
		// Process a single PDF file
		fmt.Println("Processing single PDF file:", *pdfPath)

		// Read PDF bytes from disk.
		pdfBytes, err := os.ReadFile(*pdfPath)
		if err != nil {
			log.Fatalf("Failed to read PDF file: %v", err)
		}

		// Pre-check for OCR (exits if strict mode and OCR found)
		hasOCR = checkPDFForOCR(pdfBytes, pdfOcrConfig)

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

			// Check for OCR in this page
			ocrResult, err := pdfocr.DetectOCR(pageBytes, pdfOcrConfig)
			if err == nil && ocrResult.HasOCR {
				fmt.Printf("Warning: Page %d already has OCR\n", i+1)
				hasOCR = true

				// In strict mode without force, exit with error
				if *strict && !*force {
					fmt.Printf("Error: Page %d already has OCR and strict mode is enabled\n", i+1)
					os.Exit(ExitCodeStrictOCRFailure)
				}
			}

			// Add page to processing regardless (OCR check just sets warning flag)
			pdfPageBytes = append(pdfPageBytes, pageBytes)
		}

		// Process the PDFs using DocumentHOCRFromPages
		doc, hocrHTML, err = gdocai.DocumentHOCRFromPages(ctx, pdfPageBytes, cfg)
		if err != nil {
			log.Fatalf("Error processing documents: %v", err)
		}
	}

	// If OCR was detected, add to warning capture for proper exit code later
	if hasOCR {
		warningCapture.buf.WriteString("Warning: Document already has OCR\n")
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
		// Check if the output path contains placeholders
		if strings.Contains(*pdfOcrPath, "@{") {
			// Split the path into directory and filename parts
			dir, filenameWithPlaceholders := filepath.Split(*pdfOcrPath)

			// Create placeholder data from extracted fields
			placeholderData := &PlaceholderData{
				FormFields:            doc.FormFields.Fields,
				CustomExtractorFields: doc.CustomExtractorFields.Fields,
			}

			// Process the placeholders only in the filename part
			processedFilename, err := processPlaceholders(filenameWithPlaceholders, placeholderData)
			if err != nil {
				log.Fatalf("Failed to process output path placeholders: %v", err)
			}

			// Sanitize only the filename part
			processedFilename = sanitizeFilename(processedFilename)

			// Make sure the filename has the correct extension
			if !strings.HasSuffix(strings.ToLower(processedFilename), ".pdf") {
				processedFilename += ".pdf"
			}

			// Recombine with the original directory
			processedPath := filepath.Join(dir, processedFilename)

			// Notify the user about the placeholder substitution
			safelyLogPath(*pdfOcrPath, processedPath)

			// Update the output path
			*pdfOcrPath = processedPath
		}

		if doc.Hocr != nil && doc.Hocr.Content != nil {
			var ocrPdfBytes []byte
			var err error

			// Process based on input type
			if *pdfPath != "" && (*pdfPaths == "" || !providedFlags["pdfs"]) {
				// Single PDF case - use ApplyOCR to modify the existing PDF
				fmt.Println("Creating searchable PDF by applying OCR to existing PDF...")

				// Read the PDF
				pdfBytes, err := os.ReadFile(*pdfPath)
				if err != nil {
					log.Fatalf("Failed to read PDF file: %v", err)
				}

				// Apply OCR to the PDF
				ocrPdfBytes, err = pdfocr.ApplyOCR(pdfBytes, doc.Hocr.Content, pdfOcrConfig)
				if err != nil {
					// Special case for OCR already detected in strict mode
					if strings.Contains(err.Error(), "already has OCR") && *strict {
						fmt.Printf("Error: %v\n", err)
						os.Exit(ExitCodeStrictOCRFailure)
					}
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
				ocrPdfBytes, err = pdfocr.AssembleWithOCR(doc.Hocr.Content, pageImages, pdfOcrConfig)
				if err != nil {
					log.Fatalf("Failed to create PDF from images: %v", err)
				}
			}

			// Create output directory if it doesn't exist
			outputDir := filepath.Dir(*pdfOcrPath)
			if outputDir != "" && outputDir != "." {
				if err := os.MkdirAll(outputDir, 0755); err != nil {
					log.Fatalf("Failed to create output directory: %v", err)
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

	// Exit with appropriate code based on warning capture
	if warningCapture.HasOCRWarning() {
		fmt.Println("Note: Completed with OCR warnings - existing OCR was detected")
		os.Exit(ExitCodeSuccessWithWarns)
	} else if warningCapture.HasWarnings() {
		fmt.Println("Note: Completed with warnings")
		os.Exit(ExitCodeSuccessWithWarns)
	} else {
		os.Exit(ExitCodeSuccess)
	}
}
