package dh

import (
	"context"
	"fmt"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
)

// PSAImport submits PSA-graded certs for import, including those whose underlying
// card isn't in DH's public catalog. DH will either match against the catalog,
// create a private "partner-submitted" card, or return a per-cert error.
//
// The PSA API key is sent in the request body (not the X-PSA-API-Key header)
// per DH's endpoint contract. The current key from the rotator is used so key
// rotation on rate limits still works.
//
// Caller is responsible for batching — DH enforces PSAImportMaxItems per request.
func (c *Client) PSAImport(ctx context.Context, items []PSAImportItem) (*PSAImportResponse, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("PSAImport: items is empty")
	}
	if len(items) > PSAImportMaxItems {
		return nil, fmt.Errorf("PSAImport: %d items exceeds maximum of %d", len(items), PSAImportMaxItems)
	}

	psaKey := c.currentPSAKey()
	if psaKey == "" {
		return nil, apperrors.ConfigMissing("PSA API key", "PSA_API_TOKENS")
	}

	fullURL := fmt.Sprintf("%s/api/v1/enterprise/inventory/psa_import", c.baseURL)
	body := PSAImportRequest{
		PSAAPIKey: psaKey,
		Items:     items,
	}

	var resp PSAImportResponse
	if err := c.postEnterprise(ctx, fullURL, body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
