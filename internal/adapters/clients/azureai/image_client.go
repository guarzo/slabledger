package azureai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// ImageOption configures the ImageClient.
type ImageOption func(*ImageClient)

// WithImageLogger sets the logger on the ImageClient.
func WithImageLogger(l observability.Logger) ImageOption {
	return func(c *ImageClient) { c.logger = l }
}

// ImageClient implements ai.ImageGenerator for Azure AI Foundry image generation.
// It reuses the same Config type as the LLM client.
type ImageClient struct {
	config     Config
	httpClient *httpx.Client
	logger     observability.Logger
}

// NewImageClient creates a new Azure AI image generation client.
// Required fields: Endpoint, APIKey, DeploymentName.
// APIVersion defaults to "2024-12-01-preview" if not set.
func NewImageClient(cfg Config, opts ...ImageOption) (*ImageClient, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("azureai: Endpoint is required")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("azureai: APIKey is required")
	}
	if cfg.DeploymentName == "" {
		return nil, fmt.Errorf("azureai: DeploymentName is required")
	}
	if cfg.APIVersion == "" {
		cfg.APIVersion = "2024-12-01-preview"
	}

	httpCfg := httpx.DefaultConfig("AzureAIImage")
	httpCfg.DefaultTimeout = 2 * time.Minute // image generation is slow
	c := &ImageClient{
		config:     cfg,
		httpClient: httpx.NewClient(httpCfg),
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

	headers := map[string]string{"Content-Type": "application/json"}
	if isAzureOpenAI(endpoint) {
		headers["api-key"] = c.config.APIKey
	} else {
		headers["Authorization"] = "Bearer " + c.config.APIKey
	}

	resp, err := c.httpClient.Post(ctx, url, headers, body, 2*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("azureai image: http request: %w", err)
	}
	respBody := resp.Body

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
