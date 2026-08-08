package dh

import (
	"context"
	"fmt"
	"net/url"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
)

var validCertStatuses = map[string]bool{
	CertStatusMatched:   true,
	CertStatusAmbiguous: true,
	CertStatusNotFound:  true,
	"pending":           true,
}

// ResolveCert resolves a single PSA cert synchronously via the enterprise API.
// If PSA API keys are configured, the current key is sent via X-PSA-API-Key header.
func (c *Client) ResolveCert(ctx context.Context, req CertResolveRequest) (*CertResolution, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/certs/resolve", c.baseURL)
	body := CertResolveBody{Cert: req}

	var psaHeaders map[string]string
	if key := c.currentPSAKey(); key != "" {
		psaHeaders = map[string]string{"X-PSA-API-Key": key}
	}

	var resp CertResolution
	if err := c.doEnterprise(ctx, "POST", fullURL, body, &resp, psaHeaders); err != nil {
		return nil, err
	}
	if !validCertStatuses[resp.Status] {
		return nil, apperrors.ProviderInvalidResponse(providerName,
			fmt.Errorf("unexpected cert resolve status %q", resp.Status))
	}
	return &resp, nil
}

// ResolveCertsBatch submits up to 500 certs for asynchronous resolution.
// Returns a batch_id that can be polled via GetCertResolutionJob.
//
// The batch endpoints live under the /dev/ namespace; the singular resolve above
// does not. DH moved them on 2026-04-23 (their PR #906) without a deprecation
// window, which is why this call 404'd silently until 2026-08-08.
func (c *Client) ResolveCertsBatch(ctx context.Context, certs []CertResolveRequest) (*CertResolveBatchResponse, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/dev/certs/resolve_batch", c.baseURL)
	body := CertResolveBatchRequest{Certs: certs}

	var resp CertResolveBatchResponse
	if err := c.postEnterprise(ctx, fullURL, body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ConfirmMatch confirms the correct card match for a cert that was unmatched
// or incorrectly matched. This teaches the DH system for future lookups.
func (c *Client) ConfirmMatch(ctx context.Context, req ConfirmMatchRequest) (*ConfirmMatchResponse, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/certs/confirm_match", c.baseURL)

	var resp ConfirmMatchResponse
	if err := c.postEnterprise(ctx, fullURL, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ConfirmMatchBatch confirms correct card matches for up to 500 certs at once.
func (c *Client) ConfirmMatchBatch(ctx context.Context, confirmations []ConfirmMatchRequest) (*ConfirmMatchBatchResponse, error) {
	if len(confirmations) > 500 {
		return nil, fmt.Errorf("ConfirmMatchBatch: %d confirmations exceeds maximum of 500", len(confirmations))
	}
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/certs/confirm_match", c.baseURL)
	body := ConfirmMatchBatchRequest{Confirmations: confirmations}

	var resp ConfirmMatchBatchResponse
	if err := c.postEnterprise(ctx, fullURL, body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetCertResolutionJob polls for the status and results of a batch cert resolution job.
//
// The poll path is batch_status/{batch_id}, not resolve_batch/{id}. Note that the
// submit response carries a poll_url field that omits the /dev/ segment and 404s —
// build the path here rather than following it. Results live in Redis under a 1h
// TTL, so a 404 from this call may mean the batch expired, not that the path is wrong.
func (c *Client) GetCertResolutionJob(ctx context.Context, batchID string) (*CertResolutionJobStatus, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/dev/certs/batch_status/%s", c.baseURL, url.PathEscape(batchID))

	var resp CertResolutionJobStatus
	if err := c.doEnterprise(ctx, "GET", fullURL, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
