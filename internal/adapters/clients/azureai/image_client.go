package azureai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// ImageConfig configures the Azure AI image generation client.
type ImageConfig struct {
	Endpoint       string // https://<resource>.openai.azure.com
	APIKey         string // Azure API key
	DeploymentName string // e.g. "gpt-image-1"
	APIVersion     string // e.g. "2024-12-01-preview"
}

// ImageOption configures the ImageClient.
type ImageOption func(*ImageClient)

// WithImageLogger sets the logger on the ImageClient.
func WithImageLogger(l observability.Logger) ImageOption {
	return func(c *ImageClient) { c.logger = l }
}

// ImageClient implements ai.ImageGenerator for Azure AI Foundry image generation.
type ImageClient struct {
	config     ImageConfig
	httpClient *http.Client
	logger     observability.Logger
}

// NewImageClient creates a new Azure AI image generation client.
// Required fields: Endpoint, APIKey, DeploymentName.
// APIVersion defaults to "2024-12-01-preview" if not set.
func NewImageClient(cfg ImageConfig, opts ...ImageOption) (*ImageClient, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("azureai: ImageConfig.Endpoint is required")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("azureai: ImageConfig.APIKey is required")
	}
	if cfg.DeploymentName == "" {
		return nil, fmt.Errorf("azureai: ImageConfig.DeploymentName is required")
	}
	if cfg.APIVersion == "" {
		cfg.APIVersion = "2024-12-01-preview"
	}

	c := &ImageClient{
		config: cfg,
		httpClient: &http.Client{
			Transport: httpx.DefaultTransport(),
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

var _ ai.ImageGenerator = (*ImageClient)(nil)

// imageGenerationRequest is the request body for Azure OpenAI image generation.
type imageGenerationRequest struct {
	Prompt       string `json:"prompt"`
	Size         string `json:"size"`
	Quality      string `json:"quality"`
	N            int    `json:"n"`
	OutputFormat string `json:"output_format"`
}

// imageGenerationResponse is the response from Azure OpenAI image generation.
type imageGenerationResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
	} `json:"data"`
}

// GenerateImage generates an image from the given prompt using Azure AI Foundry.
// It POSTs to {endpoint}/openai/deployments/{deployment}/images/generations?api-version={version}
// and returns the base64-decoded PNG image bytes.
func (c *ImageClient) GenerateImage(ctx context.Context, req ai.ImageRequest) (*ai.ImageResult, error) {
	size := req.Size
	if size == "" {
		size = "1024x1024"
	}
	quality := req.Quality
	if quality == "" {
		quality = "medium"
	}

	apiReq := imageGenerationRequest{
		Prompt:       req.Prompt,
		Size:         size,
		Quality:      quality,
		N:            1,
		OutputFormat: "png",
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("azureai image: marshal request: %w", err)
	}

	endpoint := strings.TrimRight(c.config.Endpoint, "/")
	url := fmt.Sprintf("%s/openai/deployments/%s/images/generations?api-version=%s",
		endpoint, c.config.DeploymentName, c.config.APIVersion)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("azureai image: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	if isAzureOpenAI(endpoint) {
		httpReq.Header.Set("api-key", c.config.APIKey)
	} else {
		httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("azureai image: http request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("azureai image: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if c.logger != nil {
			c.logger.Error(ctx, "azure ai image request failed",
				observability.Int("status", resp.StatusCode),
				observability.String("body", string(respBody)),
			)
		}
		return nil, fmt.Errorf("azureai image: returned %d: %s", resp.StatusCode, string(respBody))
	}

	var imageResp imageGenerationResponse
	if err := json.Unmarshal(respBody, &imageResp); err != nil {
		return nil, fmt.Errorf("azureai image: parse response: %w", err)
	}

	if len(imageResp.Data) == 0 || imageResp.Data[0].B64JSON == "" {
		return nil, fmt.Errorf("azureai image: no image data in response")
	}

	imageData, err := base64.StdEncoding.DecodeString(imageResp.Data[0].B64JSON)
	if err != nil {
		return nil, fmt.Errorf("azureai image: decode base64: %w", err)
	}

	return &ai.ImageResult{
		ImageData: imageData,
		Format:    "png",
	}, nil
}
