package dh

import (
	"context"
	"fmt"
	"net/url"
)

// ResolveCert resolves a single PSA cert synchronously via the enterprise API.
// Wraps in the batch format (DH API requires the certs array wrapper).
func (c *Client) ResolveCert(ctx context.Context, req CertResolveRequest) (*CertResolution, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/certs/resolve", c.baseURL)
	body := CertResolveBatchRequest{Certs: []CertResolveRequest{req}}

	var resp []CertResolution
	if err := c.postEnterprise(ctx, fullURL, body, &resp); err != nil {
		return nil, err
	}
	if len(resp) == 0 {
		return nil, fmt.Errorf("dh: resolve returned empty results for cert %s", req.CertNumber)
	}
	return &resp[0], nil
}

// ResolveCertsBatch submits up to 500 certs for resolution. Returns results synchronously
// (DH resolves small batches inline via the same /certs/resolve endpoint).
func (c *Client) ResolveCertsBatch(ctx context.Context, certs []CertResolveRequest) ([]CertResolution, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/certs/resolve", c.baseURL)
	body := CertResolveBatchRequest{Certs: certs}

	var resp []CertResolution
	if err := c.postEnterprise(ctx, fullURL, body, &resp); err != nil {
		return nil, err
	}
	return resp, nil
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
