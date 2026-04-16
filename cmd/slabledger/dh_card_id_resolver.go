package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// maxCertsPerBatch is DH's documented per-request limit for the batch cert
// resolution endpoint (see internal/adapters/clients/dh/certs.go). Callers
// pass arbitrary-size slices; the adapter chunks internally.
const maxCertsPerBatch = 500

// dhCardIDResolverAdapter satisfies inventory.CardIDResolver by submitting a
// batch cert resolution job to DH, polling until the job completes or a
// timeout elapses, and returning cert_number → stringified dh_card_id for
// every successfully matched cert.
//
// Large cert lists are automatically chunked at maxCertsPerBatch so the
// caller (batchResolveCardIDs) doesn't need to know about the DH limit.
//
// The interface returns map[string]string because it is shared with other
// external-ID sources (e.g. CardLadder); DH card IDs are numeric and are
// stringified here via strconv.Itoa.
//
// Ambiguous and not_found resolutions are omitted from the result map
// (silent success — missing keys mean "DH couldn't match this cert").
type dhCardIDResolverAdapter struct {
	client    *dh.Client
	logger    observability.Logger
	pollEvery time.Duration
	timeout   time.Duration
}

func newDHCardIDResolverAdapter(client *dh.Client, logger observability.Logger) *dhCardIDResolverAdapter {
	return &dhCardIDResolverAdapter{
		client:    client,
		logger:    logger,
		pollEvery: 2 * time.Second,
		timeout:   60 * time.Second,
	}
}

// ResolveCardIDsByCerts submits the given certs to DH's async batch endpoint
// (chunked at maxCertsPerBatch), polls each returned job until completion (or
// timeout), and returns the cert → card_id map for successfully matched
// certs. The grader parameter is accepted for interface compatibility but
// not passed to DH (DH infers the grader from the cert number).
//
// An error from any chunk aborts the whole operation; partial results from
// earlier chunks are discarded.
func (a *dhCardIDResolverAdapter) ResolveCardIDsByCerts(ctx context.Context, certs []string, grader string) (map[string]string, error) {
	if len(certs) == 0 {
		return map[string]string{}, nil
	}

	out := make(map[string]string, len(certs))
	for start := 0; start < len(certs); start += maxCertsPerBatch {
		end := min(start+maxCertsPerBatch, len(certs))
		resolved, err := a.resolveChunk(ctx, certs[start:end])
		if err != nil {
			return nil, err
		}
		for k, v := range resolved {
			out[k] = v
		}
	}
	return out, nil
}

// resolveChunk submits a single batch (≤ maxCertsPerBatch) and polls the job
// to completion.
func (a *dhCardIDResolverAdapter) resolveChunk(ctx context.Context, certs []string) (map[string]string, error) {
	reqs := make([]dh.CertResolveRequest, 0, len(certs))
	for _, c := range certs {
		reqs = append(reqs, dh.CertResolveRequest{CertNumber: c})
	}

	batch, err := a.client.ResolveCertsBatch(ctx, reqs)
	if err != nil {
		return nil, fmt.Errorf("submit batch cert resolve: %w", err)
	}
	if batch == nil || batch.JobID == "" {
		return nil, fmt.Errorf("batch cert resolve: empty job_id")
	}

	deadline := time.Now().Add(a.timeout)
	unknownStatusCount := 0
	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("batch cert resolve: job %s did not complete within %s", batch.JobID, a.timeout)
		}

		job, err := a.client.GetCertResolutionJob(ctx, batch.JobID)
		if err != nil {
			return nil, fmt.Errorf("poll job %s: %w", batch.JobID, err)
		}

		switch job.Status {
		case "completed":
			out := make(map[string]string, len(job.Results))
			for _, r := range job.Results {
				if r.Status == dh.CertStatusMatched && r.DHCardID > 0 && r.CertNumber != "" {
					out[r.CertNumber] = strconv.Itoa(r.DHCardID)
				}
			}
			return out, nil
		case "failed":
			return nil, fmt.Errorf("batch cert resolve: job %s failed", batch.JobID)
		case "queued", "processing":
			// fall through to sleep
		default:
			unknownStatusCount++
			a.logger.Warn(ctx, "batch cert resolve: unexpected job status",
				observability.String("job_id", batch.JobID),
				observability.String("status", job.Status))
			if unknownStatusCount >= 3 {
				return nil, fmt.Errorf("batch cert resolve: job %s returned unexpected status %q %d times", batch.JobID, job.Status, unknownStatusCount)
			}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(a.pollEvery):
		}
	}
}

// Compile-time assertion that the adapter satisfies the resolver interface.
var _ inventory.CardIDResolver = (*dhCardIDResolverAdapter)(nil)