package gdocai

import (
	"cloud.google.com/go/documentai/apiv1/documentaipb"
)

// ExtractCustomExtractorFields extracts entities from custom extractors into a map
// Recursively handles any level of nested properties and handles duplicate keys
func ExtractCustomExtractorFields(docProto *documentaipb.Document) map[string]interface{} {
	fields := make(map[string]interface{})

	if docProto == nil || len(docProto.Entities) == 0 {
		return fields
	}

	// Process all top-level entities
	for _, entity := range docProto.Entities {
		if entity.Type == "" {
			continue
		}

		// Process this entity and add to fields
		processEntity(entity, fields)
	}

	return fields
}

// processEntity handles a single entity and adds it to the provided fields map
// This function works recursively to handle any level of nesting
func processEntity(entity *documentaipb.Document_Entity, fields map[string]interface{}) {
	key := entity.Type
	value := entity.MentionText

	// If the entity has properties, create a nested map
	if len(entity.Properties) > 0 {
		// Create or get the existing properties map
		var propMap map[string]interface{}

		// Check if we already have an entry for this key
		if existing, exists := fields[key]; exists {
			// If it's already a map, use it
			if existingMap, ok := existing.(map[string]interface{}); ok {
				propMap = existingMap
			} else {
				// Otherwise create a new map and preserve the existing value under a special key
				propMap = make(map[string]interface{})
				propMap["_value"] = existing
			}
		} else {
			// Create a new map
			propMap = make(map[string]interface{})
			// Store the entity's own mention text under a special key if it's not empty
			if value != "" {
				propMap["_value"] = value
			}
		}

		// Process all properties recursively
		for _, prop := range entity.Properties {
			processEntity(prop, propMap)
		}

		// Add the properties map to the fields
		fields[key] = propMap
	} else {
		// No properties - just add the value, handling duplicates
		addValueToMap(fields, key, value)
	}
}

// addValueToMap adds a value to a map, handling duplicates by converting to arrays
func addValueToMap(fields map[string]interface{}, key string, value string) {
	if key == "" {
		return
	}

	if existing, exists := fields[key]; exists {
		switch v := existing.(type) {
		case string:
			if v != value && value != "" {
				fields[key] = []string{v, value}
			}
		case []string:
			// Check if value already exists in the array
			if value != "" {
				found := false
				for _, existingVal := range v {
					if existingVal == value {
						found = true
						break
					}
				}
				if !found {
					fields[key] = append(v, value)
				}
			}
		case map[string]interface{}:
			// It's a map, so add/update the _value key
			if value != "" {
				addValueToMap(v, "_value", value)
			}
		}
	} else {
		if value != "" {
			fields[key] = value
		} else {
			// Create an empty map for entities with no value but potential future properties
			fields[key] = make(map[string]interface{})
		}
	}
}
