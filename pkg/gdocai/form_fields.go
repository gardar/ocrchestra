package gdocai

import (
	"strings"

	"cloud.google.com/go/documentai/apiv1/documentaipb"
)

// ExtractFormFields combines form fields from all pages into a single map
// Handles duplicate keys by converting to arrays of values
func ExtractFormFields(docProto *documentaipb.Document) map[string]interface{} {
	fields := make(map[string]interface{})

	for _, page := range docProto.Pages {
		for _, field := range page.FormFields {
			key := strings.TrimSpace(textFromLayout(field.FieldName, docProto.Text))
			key = strings.TrimSuffix(key, ":")
			value := strings.TrimSpace(textFromLayout(field.FieldValue, docProto.Text))

			if key == "" {
				continue
			}

			// Handle duplicate keys by converting to arrays
			if existing, exists := fields[key]; exists {
				switch v := existing.(type) {
				case string:
					if v != value {
						fields[key] = []string{v, value}
					}
				case []string:
					fields[key] = append(v, value)
				}
			} else {
				fields[key] = value
			}
		}
	}

	return fields
}
