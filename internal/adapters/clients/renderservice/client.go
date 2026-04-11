package renderservice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/guarzo/slabledger/internal/domain/social"
)

const (
	defaultTimeout = 90 * time.Second
	healthPath     = "/health"
	renderPath     = "/render/"
)

// Client can render slide JPEGs via the sidecar and check its health.
type Client interface {
	Health(ctx context.Context) error
	Render(ctx context.Context, postID string, detail social.PostDetail) ([][]byte, error)
}

// HTTPClient is an HTTP implementation of Client.
type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new render service client targeting baseURL.
func NewClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

// Health pings the sidecar health endpoint. Returns an error if the sidecar is not healthy.
func (c *HTTPClient) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+healthPath, nil)
	if err != nil {
		return fmt.Errorf("build health request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned %d", resp.StatusCode)
	}
	return nil
}

// Render requests the sidecar to render all slides for a post.
// Returns JPEG bytes ordered by slide index (cover = 0, cards = 1..N).
func (c *HTTPClient) Render(ctx context.Context, postID string, detail social.PostDetail) ([][]byte, error) {
	body, err := json.Marshal(detail)
	if err != nil {
		return nil, fmt.Errorf("marshal post detail: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+renderPath+postID, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build render request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("render request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("render returned %d: %s", resp.StatusCode, string(body))
	}

	return parseMultipartResponse(resp)
}

// parseMultipartResponse reads the multipart/form-data response from the sidecar
// and returns the JPEG blobs ordered by slide index.
func parseMultipartResponse(resp *http.Response) ([][]byte, error) {
	ct := resp.Header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(ct)
	if err != nil {
		return nil, fmt.Errorf("parse content-type %q: %w", ct, err)
	}
	if mediaType != "multipart/form-data" {
		return nil, fmt.Errorf("expected multipart/form-data, got %s", mediaType)
	}

	boundary := params["boundary"]
	mr := multipart.NewReader(resp.Body, boundary)

	// The sidecar sends slide-0, slide-1, ... in order; collect all parts.
	var blobs [][]byte
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read multipart part: %w", err)
		}
		data, err := io.ReadAll(part)
		if err != nil {
			return nil, fmt.Errorf("read part body: %w", err)
		}
		blobs = append(blobs, data)
	}
	return blobs, nil
}
