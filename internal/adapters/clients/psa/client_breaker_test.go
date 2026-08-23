package psa

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/resilience"
)

// tightBreakerConfig trips after a couple of failures instead of the
// production threshold of 10 requests at an 80% failure ratio. The behaviour
// under test is which calls share a breaker, not where the threshold sits, and
// a production-sized threshold would cost 10 paced requests per assertion.
func tightBreakerConfig(name string) httpx.Config {
	cfg := httpx.DefaultConfig(name)
	cfg.DefaultTimeout = 5 * time.Second
	cfg.RetryPolicy = resilience.RetryPolicy{MaxRetries: 0}
	cfg.CircuitBreakerConfig = resilience.CircuitBreakerConfig{
		Name:         name,
		MaxRequests:  1,
		Interval:     60 * time.Second,
		Timeout:      60 * time.Second, // long enough that it cannot self-heal mid-test
		MinRequests:  2,
		FailureRatio: 0.5,
	}
	return cfg
}

// newBreakerTestClient builds a PSA client whose breakers trip quickly.
func newBreakerTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	c := NewClient("test-token", observability.NewNoopLogger())
	c.baseURL = serverURL
	c.httpClient = httpx.NewClient(tightBreakerConfig("PSA-cert-test"))
	c.imagesHTTP = httpx.NewClient(tightBreakerConfig("PSA-images-test"))
	return c
}

// TestImagesFailuresDoNotBlockCertLookup is the regression test for the
// 2026-08-23 production incident: PSA answers a deterministic HTTP 500 from
// GetImagesByCertNumber for certain older slabs while GetByCertNumber serves
// those same certs fine. Both endpoints shared one circuit breaker, so the
// background image backfill tripped it and every card scan came back
// "Internal server error" — a healthy endpoint taken down by a broken one.
func TestImagesFailuresDoNotBlockCertLookup(t *testing.T) {
	var certCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "GetImagesByCertNumber") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"Message":"An error occurred while processing your request."}`))
			return
		}
		certCalls.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"PSACert":{"CertNumber":"46224119","Subject":"CHARIZARD","CardGrade":"NM-MT 8"}}`))
	}))
	defer server.Close()

	c := newBreakerTestClient(t, server.URL)
	ctx := context.Background()

	// Drive the images endpoint until its breaker is open, mirroring the
	// backfill sweep walking a run of certs PSA cannot serve images for.
	var sawCircuitOpen bool
	for i := 0; i < 6; i++ {
		_, err := c.GetImages(ctx, "46224119")
		if err == nil {
			t.Fatal("GetImages: expected an error from the 500ing server, got nil")
		}
		if apperrors.HasErrorCode(err, apperrors.ErrCodeProviderCircuitOpen) {
			sawCircuitOpen = true
			break
		}
	}
	if !sawCircuitOpen {
		t.Fatal("images breaker never opened; test cannot prove the isolation it claims")
	}

	// The cert endpoint is healthy and is what card scanning depends on. It
	// must still answer while the images breaker is open.
	cert, err := c.GetCert(ctx, "46224119")
	if err != nil {
		t.Fatalf("GetCert failed while the images breaker was open: %v", err)
	}
	if cert.CertNumber != "46224119" {
		t.Errorf("GetCert returned cert %q, want 46224119", cert.CertNumber)
	}
	if got := certCalls.Load(); got != 1 {
		t.Errorf("cert endpoint received %d calls, want 1 (request never reached the server)", got)
	}
}

// TestCertBreakerStillTripsOnCertFailures guards the other direction: giving
// images its own breaker must not leave the cert endpoint unprotected. A real
// PSA outage on cert lookup should still open that breaker and stop the
// hammering.
func TestCertBreakerStillTripsOnCertFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"Message":"An error occurred while processing your request."}`))
	}))
	defer server.Close()

	c := newBreakerTestClient(t, server.URL)
	ctx := context.Background()

	for i := 0; i < 6; i++ {
		_, err := c.GetCert(ctx, "46224119")
		if err == nil {
			t.Fatal("GetCert: expected an error from the 500ing server, got nil")
		}
		if apperrors.HasErrorCode(err, apperrors.ErrCodeProviderCircuitOpen) {
			return // breaker opened as it should
		}
	}
	t.Fatal("cert breaker never opened despite a run of 500s")
}
