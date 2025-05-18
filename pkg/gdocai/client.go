package gdocai

import (
	"context"
	"fmt"
	"os"

	documentai "cloud.google.com/go/documentai/apiv1"
	"cloud.google.com/go/documentai/apiv1/documentaipb"
	"google.golang.org/api/option"
)

// ProcessDocument sends PDF bytes to Google Document AI for processing
// and returns the raw Document proto response
func ProcessDocument(ctx context.Context, pdfBytes []byte, cfg *Config) (*documentaipb.Document, error) {
	endpoint := fmt.Sprintf("%s-documentai.googleapis.com:443", cfg.Location)

        // Instantiate Document AI client using credentials from environment variable
        client, err := documentai.NewDocumentProcessorClient(
                ctx,
                option.WithEndpoint(endpoint),
                option.WithCredentialsFile(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")),
        )
	if err != nil {
		return nil, fmt.Errorf("failed to create Document AI client: %w", err)
	}
	defer client.Close()

	// Build the resource name of the processor
	name := fmt.Sprintf(
		"projects/%s/locations/%s/processors/%s",
		cfg.ProjectID, cfg.Location, cfg.ProcessorID,
	)

	// Create the request
	req := &documentaipb.ProcessRequest{
		Name: name,
		Source: &documentaipb.ProcessRequest_RawDocument{
			RawDocument: &documentaipb.RawDocument{
				Content:  pdfBytes,
				MimeType: "application/pdf",
			},
		},
		SkipHumanReview: true,
	}

	resp, err := client.ProcessDocument(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to process document: %w", err)
	}

	return resp.Document, nil
}
