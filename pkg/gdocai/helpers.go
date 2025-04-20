package gdocai

import (
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// ToJSON converts various types to a pretty-printed JSON string
// It handles both protocol buffer messages and regular Go structs
func ToJSON(data interface{}) (string, error) {
	switch v := data.(type) {
	case proto.Message:
		// For protocol buffer messages, use protojson
		jsonData, err := protojson.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(jsonData), nil

	default:
		// For regular Go structs, use standard json package
		jsonData, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return "", err
		}
		return string(jsonData), nil
	}
}

// ExtractImageFromPage pulls out the image data from a Document AI page
func ExtractImageFromPage(page *Page) ([]byte, error) {
	if page == nil || page.DocumentaiObject == nil {
		return nil, fmt.Errorf("no documentai page provided")
	}

	// Access the image from the wrapped Document AI page.
	image := page.DocumentaiObject.GetImage()
	if image == nil {
		return nil, fmt.Errorf("no image found in documentai page")
	}

	content := image.GetContent()
	if len(content) == 0 {
		return nil, fmt.Errorf("image content is empty")
	}

	return content, nil
}
