package psaportal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

// CreateCampaign creates a new portal campaign from fd and returns the new
// campaign's request ID. The ref-packed wire body is the bare formData object
// (unlike updateCampaign, which wraps {id, formData} in a single-element
// array). Errors after a 200 response mention that the campaign may already
// exist on the portal, so callers never blind-retry a decode failure.
func (c *Client) CreateCampaign(ctx context.Context, fd psacampaign.CampaignFormData) (string, error) {
	buildHash, err := c.fetchBuildHash(ctx)
	if err != nil {
		return "", err
	}

	// Round-trip through JSON so EncodeRefPacked sees plain JSON types; the
	// struct's json tags define the wire field names, and its typed fields
	// keep grades as strings and the rest as numbers.
	fdJSON, err := json.Marshal(fd)
	if err != nil {
		return "", fmt.Errorf("psaportal: marshal create formData: %w", err)
	}
	var fdMap map[string]any
	if err := json.Unmarshal(fdJSON, &fdMap); err != nil {
		return "", fmt.Errorf("psaportal: remarshal create formData: %w", err)
	}

	packed, err := EncodeRefPacked(fdMap)
	if err != nil {
		return "", fmt.Errorf("psaportal: encode create payload: %w", err)
	}
	arrJSON, err := json.Marshal(packed)
	if err != nil {
		return "", fmt.Errorf("psaportal: marshal create payload: %w", err)
	}
	body, err := json.Marshal(map[string]any{
		"payload":   base64.StdEncoding.EncodeToString(arrJSON),
		"refreshes": []any{},
	})
	if err != nil {
		return "", fmt.Errorf("psaportal: marshal create request: %w", err)
	}

	createURL := fmt.Sprintf("%s/buyercampaignmanager/_app/remote/%s/createCampaign", c.baseURL(), buildHash)
	resp, err := c.fetch.Do(ctx, FetchRequest{URL: createURL, Method: "POST", Body: string(body)})
	if err != nil {
		return "", fmt.Errorf("psaportal: create campaign: %w", err)
	}
	if resp.Status != 200 {
		return "", fmt.Errorf("psaportal: create campaign status %d", resp.Status)
	}

	var envelope struct {
		Type   string `json:"type"`
		Result string `json:"result"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &envelope); err != nil {
		return "", fmt.Errorf("psaportal: decode create campaign response: %w", err)
	}
	if envelope.Type != "result" {
		return "", fmt.Errorf("psaportal: create campaign response type %q, want \"result\"", envelope.Type)
	}

	// From here on the portal HAS accepted the create; decode failures must
	// not read as retryable.
	var resultPacked []json.RawMessage
	if err := json.Unmarshal([]byte(envelope.Result), &resultPacked); err != nil {
		return "", fmt.Errorf("psaportal: create succeeded but result undecodable (campaign may exist on portal — verify before retrying): %w", err)
	}
	root, err := DecodeRefPacked(resultPacked)
	if err != nil {
		return "", fmt.Errorf("psaportal: create succeeded but result undecodable (campaign may exist on portal — verify before retrying): %w", err)
	}
	rootMap, ok := root.(map[string]any)
	if !ok {
		return "", fmt.Errorf("psaportal: create result not an object (campaign may exist on portal — verify before retrying)")
	}
	id, ok := rootMap["campaignRequestId"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("psaportal: create result missing campaignRequestId (campaign may exist on portal — verify before retrying)")
	}
	return id, nil
}
