package dh

import (
	"context"
	"fmt"
	"net/url"
)

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
	return &resp, nil
}

// ResolveCertsBatch submits up to 500 certs for asynchronous resolution.
// Returns a job status with a job_id that can be polled via GetCertResolutionJob.
func (c *Client) ResolveCertsBatch(ctx context.Context, certs []CertResolveRequest) (*CertResolveBatchResponse, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/certs/resolve_batch", c.baseURL)
	body := CertResolveBatchRequest{Certs: certs}

	var resp CertResolveBatchResponse
	if err := c.postEnterprise(ctx, fullURL, body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetCertResolutionJob polls for the status and results of a batch cert resolution job.
func (c *Client) GetCertResolutionJob(ctx context.Context, jobID string) (*CertResolutionJobStatus, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/certs/resolve_batch/%s", c.baseURL, url.PathEscape(jobID))

	var resp CertResolutionJobStatus
	if err := c.getEnterprise(ctx, fullURL, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
