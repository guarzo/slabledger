package psa

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// newTestClient creates a PSA client pointing at a test server.
func newTestClient(t *testing.T, serverURL string, tokens ...string) *Client {
	t.Helper()
	tokenStr := "test-token"
	if len(tokens) > 0 {
		tokenStr = strings.Join(tokens, ",")
	}
	c := NewClient(tokenStr, observability.NewNoopLogger())
	c.baseURL = serverURL
	// Use a fast httpx client for tests (low timeout, no retries).
	cfg := httpx.DefaultConfig("PSA-test")
	cfg.DefaultTimeout = 5 * time.Second
	c.httpClient = httpx.NewClient(cfg)
	return c
}

// --- ParseGrade ---

func TestParseGrade(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{"GEM MT 10", 10},
		{"MINT 9", 9},
		{"NM-MT 8", 8},
		{"NM-MT 8.5", 8.5},
		{"NM 7", 7},
		{"EX-MT 6", 6},
		{"VG-EX 4", 4},
		{"PR 1", 1},
		{"", 0},
		{"AUTHENTIC", 0},
		// Edge cases
		{"VG 3", 3},
		{"FAIR 1.5", 1.5},
		{"NM-MT+ 8.5", 8.5},
		{"PSA 10", 10},
		{"grade 0", 0},  // 0 is out of range (1-10)
		{"grade 11", 0}, // 11 is out of range
		{"grade -1", 1}, // regex matches "1" from "-1"
		{"POOR 1", 1},   // minimum valid
		{"GEM-MT 10", 10},
	}

	for _, tt := range tests {
		got := ParseGrade(tt.input)
		if got != tt.expected {
			t.Errorf("ParseGrade(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

// --- BuildCardName ---

func TestBuildCardName(t *testing.T) {
	tests := []struct {
		name     string
		info     *CertInfo
		expected string
	}{
		{
			name:     "subject with variety appended",
			info:     &CertInfo{Year: "2022", Brand: "POKEMON", Category: "CELEBRATIONS", Subject: "CHARIZARD", Variety: "V"},
			expected: "CHARIZARD V",
		},
		{
			name:     "1st edition variety",
			info:     &CertInfo{Subject: "DARK GYARADOS-HOLO", Variety: "1ST EDITION"},
			expected: "DARK GYARADOS-HOLO 1ST EDITION",
		},
		{
			name:     "shadowless variety",
			info:     &CertInfo{Subject: "BLASTOISE-HOLO", Variety: "SHADOWLESS"},
			expected: "BLASTOISE-HOLO SHADOWLESS",
		},
		{
			name:     "empty subject falls back to category",
			info:     &CertInfo{Category: "CELEBRATIONS"},
			expected: "CELEBRATIONS",
		},
		{
			name:     "only subject no variety",
			info:     &CertInfo{Subject: "PIKACHU"},
			expected: "PIKACHU",
		},
		{
			name:     "empty variety not appended",
			info:     &CertInfo{Subject: "MEWTWO-REV.FOIL", Variety: ""},
			expected: "MEWTWO-REV.FOIL",
		},
		{
			name:     "all empty fields",
			info:     &CertInfo{},
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildCardName(tt.info)
			if got != tt.expected {
				t.Errorf("BuildCardName = %q, want %q", got, tt.expected)
			}
		})
	}
}

// --- GetCert ---

func TestGetCert(t *testing.T) {
	tests := []struct {
		name           string
		certNumber     string
		handler        http.HandlerFunc
		wantErr        bool
		wantErrContain string
		verify         func(t *testing.T, info *CertInfo)
	}{
		{
			name:       "success",
			certNumber: "12345678",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/cert/GetByCertNumber/12345678" {
					// Can't use t.Errorf here; write a bad response instead.
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if r.Header.Get("Authorization") != "Bearer test-token" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(CertResponse{
					PSACert: CertInfo{
						CertNumber:       "12345678",
						Subject:          "CHARIZARD-HOLO",
						CardGrade:        "GEM MT 10",
						GradeDescription: "Gem Mint",
						Year:             "1999",
						Brand:            "POKEMON",
						Category:         "BASE SET",
						TotalPopulation:  120,
						PopulationHigher: 0,
					},
				})
			},
			verify: func(t *testing.T, info *CertInfo) {
				if info.CertNumber != "12345678" {
					t.Errorf("CertNumber = %q, want %q", info.CertNumber, "12345678")
				}
				if info.Subject != "CHARIZARD-HOLO" {
					t.Errorf("Subject = %q, want %q", info.Subject, "CHARIZARD-HOLO")
				}
				if info.TotalPopulation != 120 {
					t.Errorf("TotalPopulation = %d, want 120", info.TotalPopulation)
				}
			},
		},
		{
			name:       "not found",
			certNumber: "00000000",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"PSACert":{}}`)
			},
			wantErr:        true,
			wantErrContain: "not found",
		},
		{
			name:       "decode error",
			certNumber: "12345678",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{invalid json}`)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			c := newTestClient(t, server.URL)
			info, err := c.GetCert(context.Background(), tt.certNumber)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if tt.wantErrContain != "" && !strings.Contains(err.Error(), tt.wantErrContain) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErrContain)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.verify != nil {
				tt.verify(t, info)
			}
		})
	}
}

// --- GetImages ---

func TestGetImages(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantErr    bool
		wantLen    int
		verifyPath string
	}{
		{
			name: "success",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/cert/GetImagesByCertNumber/12345678" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]ImageInfo{
					{IsFrontImage: true, ImageURL: "https://example.com/front.jpg"},
					{IsFrontImage: false, ImageURL: "https://example.com/back.jpg"},
				})
			},
			wantLen: 2,
		},
		{
			name: "decode error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(w, `not json`)
			},
			wantErr: true,
		},
		{
			name: "empty",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `[]`)
			},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			c := newTestClient(t, server.URL)
			images, err := c.GetImages(context.Background(), "12345678")

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(images) != tt.wantLen {
				t.Fatalf("got %d images, want %d", len(images), tt.wantLen)
			}
			if tt.wantLen == 2 {
				if !images[0].IsFrontImage {
					t.Error("first image should be front")
				}
				if images[1].IsFrontImage {
					t.Error("second image should be back")
				}
			}
		})
	}
}

// --- doRequest: token rotation on 429 ---

func TestDoRequest_TokenRotationOn429(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		auth := r.Header.Get("Authorization")
		if call == 1 {
			if auth != "Bearer token-a" {
				t.Errorf("call 1: expected token-a, got %s", auth)
			}
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, "rate limited")
			return
		}
		if auth != "Bearer token-b" {
			t.Errorf("call 2: expected token-b, got %s", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		resp := CertResponse{PSACert: CertInfo{CertNumber: "99999", CardGrade: "GEM MT 10"}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(t, server.URL, "token-a", "token-b")
	info, err := c.GetCert(context.Background(), "99999")
	if err != nil {
		t.Fatalf("expected success after rotation, got: %v", err)
	}
	if info.CertNumber != "99999" {
		t.Errorf("CertNumber = %q, want %q", info.CertNumber, "99999")
	}
	if calls.Load() != 2 {
		t.Errorf("expected 2 calls, got %d", calls.Load())
	}
}

func TestDoRequest_AllTokens429(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, "rate limited")
	}))
	defer server.Close()

	c := newTestClient(t, server.URL, "token-a", "token-b")
	_, err := c.GetCert(context.Background(), "12345678")
	if err == nil {
		t.Fatal("expected error when all tokens are 429'd")
	}
}

func TestDoRequest_SingleToken429NoRotation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	c := newTestClient(t, server.URL) // single token
	_, err := c.GetCert(context.Background(), "12345678")
	if err == nil {
		t.Fatal("expected error for single token 429")
	}
}

// TestDoRequest_PreservesRetryAfter proves a 429's Retry-After header survives
// from the httpx layer through doRequest into the returned AppError's
// reset_time context, rather than being flattened into an empty
// ProviderRateLimited. Covers both real-429 return paths: single-token (no
// rotation) and multi-token (loop exhausted after all tokens 429).
func TestDoRequest_PreservesRetryAfter(t *testing.T) {
	tests := []struct {
		name   string
		tokens []string
	}{
		{name: "single token, no rotation", tokens: nil},
		{name: "multi token, all 429", tokens: []string{"token-a", "token-b"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", "86115")
				w.WriteHeader(http.StatusTooManyRequests)
			}))
			defer server.Close()

			c := newTestClient(t, server.URL, tc.tokens...)
			_, err := c.GetCert(context.Background(), "12345678")
			if err == nil {
				t.Fatal("expected rate-limit error")
			}
			if !apperrors.HasErrorCode(err, apperrors.ErrCodeProviderRateLimit) {
				t.Fatalf("expected ErrCodeProviderRateLimit in chain, got %v", err)
			}
			var appErr *apperrors.AppError
			if !errors.As(err, &appErr) {
				t.Fatalf("expected AppError in chain, got %T: %v", err, err)
			}
			got, _ := appErr.Context["reset_time"].(string)
			if got != "86115" {
				t.Errorf("reset_time = %q, want %q", got, "86115")
			}
		})
	}
}

// --- doRequest: 403 handling ---

// TestDoRequest_RotatesOn403 is the regression test for SLA-108: a 403 from an
// unapproved key used to fall through to the blanket ProviderUnavailable wrap,
// so the client neither rotated nor reported the real cause. Every subsequent
// call reused the same rejected key until the breaker tripped.
func TestDoRequest_RotatesOn403(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") == "Bearer token-a" {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"Message":"Access to this API is limited to approved customers."}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CertResponse{PSACert: CertInfo{CertNumber: "777", CardGrade: "MINT 9"}})
	}))
	defer server.Close()

	c := newTestClient(t, server.URL, "token-a", "token-b")
	info, err := c.GetCert(context.Background(), "777")
	if err != nil {
		t.Fatalf("expected rotation past the 403 key, got: %v", err)
	}
	if info.CertNumber != "777" {
		t.Errorf("CertNumber = %q, want %q", info.CertNumber, "777")
	}
	if calls.Load() != 2 {
		t.Errorf("expected 2 calls (403 then success), got %d", calls.Load())
	}
}

// TestDoRequest_DeadKeyNotRetried proves a 403'd key is retired rather than
// re-tried on every subsequent request. Without this the client burns a call
// per request on a key that can never succeed.
func TestDoRequest_DeadKeyNotRetried(t *testing.T) {
	var tokenACalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer token-a" {
			tokenACalls.Add(1)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CertResponse{PSACert: CertInfo{CertNumber: "888", CardGrade: "MINT 9"}})
	}))
	defer server.Close()

	c := newTestClient(t, server.URL, "token-a", "token-b")
	for i := 0; i < 3; i++ {
		if _, err := c.GetCert(context.Background(), "888"); err != nil {
			t.Fatalf("call %d failed: %v", i+1, err)
		}
	}
	if got := tokenACalls.Load(); got != 1 {
		t.Errorf("dead key was tried %d times, want 1", got)
	}
}

// TestDoRequest_All403ReturnsAuthError proves an all-unapproved pool surfaces
// ERR_PROV_AUTH rather than ERR_PROV_UNAVAILABLE. The distinction drives
// whether the import layer queues the cert for retry or tells the operator the
// keys need fixing.
func TestDoRequest_All403ReturnsAuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"Message":"Access to this API is limited to approved customers."}`)
	}))
	defer server.Close()

	c := newTestClient(t, server.URL, "token-a", "token-b")
	_, err := c.GetCert(context.Background(), "12345678")
	if err == nil {
		t.Fatal("expected error when every key is rejected")
	}
	if !apperrors.HasErrorCode(err, apperrors.ErrCodeProviderAuth) {
		t.Fatalf("expected ErrCodeProviderAuth, got %v", err)
	}

	// A second call must not re-probe the dead keys.
	_, err = c.GetCert(context.Background(), "12345678")
	if !apperrors.HasErrorCode(err, apperrors.ErrCodeProviderAuth) {
		t.Fatalf("second call: expected ErrCodeProviderAuth, got %v", err)
	}
}

// TestDoRequest_TransientUnauthorizedKeyKept proves a one-off 401 does not
// retire a key, and that a later success clears its strike count. A 401 makes
// no claim about the next call, and with a two-key pool retiring on the first
// one halves capacity until the process restarts.
//
// The pool is inspected directly rather than by counting calls: rotation is
// sticky, so after advancing past token-a the client has no reason to come back
// to it while token-b keeps working — "not called again" would prove nothing.
func TestDoRequest_TransientUnauthorizedKeyKept(t *testing.T) {
	var tokenACalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer token-b" {
			// token-b is spent, which forces the client back onto token-a.
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `"API calls quota exceeded! maximum admitted 100 per Day."`)
			return
		}
		// Reject token-a exactly once, then accept it like PSA would after a
		// transient blip.
		if tokenACalls.Add(1) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CertResponse{PSACert: CertInfo{CertNumber: "901", CardGrade: "MINT 9"}})
	}))
	defer server.Close()

	c := newTestClient(t, server.URL, "token-a", "token-b")

	// First call: token-a 401s, token-b is spent, so the call fails.
	if _, err := c.GetCert(context.Background(), "901"); err == nil {
		t.Fatal("expected the first call to fail: one key 401'd and the other is spent")
	}
	if c.pool.dead["token-a"] {
		t.Fatal("token-a was retired by a single 401")
	}
	if got := c.pool.authFails["token-a"]; got != 1 {
		t.Errorf("authFails[token-a] = %d, want 1", got)
	}

	// Second call: token-a is still in the pool and now answers, which must
	// clear the strike.
	if _, err := c.GetCert(context.Background(), "901"); err != nil {
		t.Fatalf("expected the kept key to serve the second call, got: %v", err)
	}
	if got := c.pool.authFails["token-a"]; got != 0 {
		t.Errorf("authFails[token-a] = %d after a success, want 0", got)
	}
}

// TestDoRequest_PersistentUnauthorizedKeyRetired proves the other half: keys
// that keep answering 401 are eventually retired, so genuinely revoked
// credentials do not cost a wasted call on every request forever.
func TestDoRequest_PersistentUnauthorizedKeyRetired(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	c := newTestClient(t, server.URL, "token-a", "token-b")

	// Each call visits both usable keys, so the strike budget is spent after
	// maxConsecutiveAuthFailures calls.
	for i := 0; i < maxConsecutiveAuthFailures; i++ {
		if _, err := c.GetCert(context.Background(), "902"); err == nil {
			t.Fatalf("call %d: expected failure from an all-401 pool", i+1)
		}
	}
	for _, tok := range []string{"token-a", "token-b"} {
		if !c.pool.dead[tok] {
			t.Errorf("%s still usable after %d rejections", tok, maxConsecutiveAuthFailures)
		}
	}

	// With the pool retired, the next call must not touch the network at all.
	before := calls.Load()
	_, err := c.GetCert(context.Background(), "902")
	if !apperrors.HasErrorCode(err, apperrors.ErrCodeProviderAuth) {
		t.Fatalf("expected ErrCodeProviderAuth once every key is dead, got %v", err)
	}
	if got := calls.Load(); got != before {
		t.Errorf("made %d further request(s) with an entirely dead pool", got-before)
	}
}

// TestTokenPool_AuthFailureStrikes covers the counter directly, including the
// reset paths that the HTTP-level tests cannot isolate.
func TestTokenPool_AuthFailureStrikes(t *testing.T) {
	tests := []struct {
		name string
		// run applies some sequence of outcomes to token "a".
		run      func(p *tokenPool)
		wantDead bool
	}{
		{
			name:     "one strike keeps the token",
			run:      func(p *tokenPool) { p.markAuthFailure("a") },
			wantDead: false,
		},
		{
			name: "one short of the budget keeps the token",
			run: func(p *tokenPool) {
				for i := 0; i < maxConsecutiveAuthFailures-1; i++ {
					p.markAuthFailure("a")
				}
			},
			wantDead: false,
		},
		{
			name: "the budget retires the token",
			run: func(p *tokenPool) {
				for i := 0; i < maxConsecutiveAuthFailures; i++ {
					p.markAuthFailure("a")
				}
			},
			wantDead: true,
		},
		{
			name: "a success resets the run",
			run: func(p *tokenPool) {
				for i := 0; i < maxConsecutiveAuthFailures-1; i++ {
					p.markAuthFailure("a")
				}
				p.markHealthy("a")
				p.markAuthFailure("a")
			},
			wantDead: false,
		},
		{
			name: "a 429 counts as a success",
			run: func(p *tokenPool) {
				for i := 0; i < maxConsecutiveAuthFailures-1; i++ {
					p.markAuthFailure("a")
				}
				// Only an authenticated key is told its quota is spent.
				p.markSpent("a")
				p.markAuthFailure("a")
			},
			wantDead: false,
		},
		{
			name: "strikes are per token",
			run: func(p *tokenPool) {
				for i := 0; i < maxConsecutiveAuthFailures; i++ {
					p.markAuthFailure("b")
				}
			},
			wantDead: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newTokenPool([]string{"a", "b"})
			tc.run(p)
			if got := p.dead["a"]; got != tc.wantDead {
				t.Errorf("dead[a] = %v, want %v", got, tc.wantDead)
			}
		})
	}
}

// TestTokenPool_AuthFailureReportsRetirement pins the return value, which the
// client uses to choose between two different log lines.
func TestTokenPool_AuthFailureReportsRetirement(t *testing.T) {
	p := newTokenPool([]string{"a"})
	for i := 1; i <= maxConsecutiveAuthFailures; i++ {
		retired := p.markAuthFailure("a")
		want := i == maxConsecutiveAuthFailures
		if retired != want {
			t.Errorf("strike %d: retired = %v, want %v", i, retired, want)
		}
	}
}

// --- doRequest: quota exhaustion ---

// TestDoRequest_SpentKeySkipped proves a 429'd key is retired for the rest of
// the UTC day instead of being re-probed on every call. Unlike a 403 the key is
// healthy, so it is only shelved until rollover.
func TestDoRequest_SpentKeySkipped(t *testing.T) {
	var tokenACalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer token-a" {
			tokenACalls.Add(1)
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `"API calls quota exceeded! maximum admitted 100 per Day."`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CertResponse{PSACert: CertInfo{CertNumber: "333", CardGrade: "NM-MT 8"}})
	}))
	defer server.Close()

	c := newTestClient(t, server.URL, "token-a", "token-b")
	for i := 0; i < 3; i++ {
		info, err := c.GetCert(context.Background(), "333")
		if err != nil {
			t.Fatalf("call %d failed: %v", i+1, err)
		}
		if info.CertNumber != "333" {
			t.Errorf("call %d: CertNumber = %q, want %q", i+1, info.CertNumber, "333")
		}
	}
	if got := tokenACalls.Load(); got != 1 {
		t.Errorf("spent key was tried %d times, want 1", got)
	}
}

// --- doRequest: no tokens configured ---

func TestDoRequest_NoTokensConfigured(t *testing.T) {
	c := &Client{
		httpClient: httpx.NewClient(httpx.DefaultConfig("test")),
		baseURL:    "http://localhost",
		pool:       newTokenPool(nil),
		logger:     observability.NewNoopLogger(),
	}
	_, err := c.GetCert(context.Background(), "12345678")
	if err == nil {
		t.Fatal("expected error for no tokens")
	}
}

// --- doRequest: request pacing ---

func TestDoRequest_RequestPacing(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		resp := CertResponse{PSACert: CertInfo{CertNumber: "444", CardGrade: "GEM MT 10"}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(t, server.URL)

	_, err := c.GetCert(context.Background(), "444")
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	start := time.Now()
	_, err = c.GetCert(context.Background(), "444")
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed < minRequestSpacing/2 {
		t.Errorf("expected pacing delay, but second call completed in %v", elapsed)
	}
}

// --- doRequest: context cancellation during pacing ---

func TestDoRequest_ContextCancelledDuringPacing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := CertResponse{PSACert: CertInfo{CertNumber: "555", CardGrade: "MINT 9"}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(t, server.URL)

	_, _ = c.GetCert(context.Background(), "555")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.GetCert(ctx, "555")
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

// --- CertAdapter ---

func TestCertAdapter_LookupCert(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := CertResponse{
			PSACert: CertInfo{
				CertNumber:       "12345678",
				Subject:          "CHARIZARD-HOLO",
				Variety:          "1ST EDITION",
				CardGrade:        "GEM MT 10",
				GradeDescription: "Gem Mint",
				Year:             "1999",
				Brand:            "POKEMON",
				Category:         "BASE SET",
				CardNumber:       "004",
				TotalPopulation:  120,
				PopulationHigher: 0,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(t, server.URL)
	adapter := NewCertAdapter(c)

	certInfo, err := adapter.LookupCert(context.Background(), "12345678")
	if err != nil {
		t.Fatalf("LookupCert error: %v", err)
	}
	if certInfo.CertNumber != "12345678" {
		t.Errorf("CertNumber = %q, want %q", certInfo.CertNumber, "12345678")
	}
	if certInfo.CardName != "CHARIZARD-HOLO 1ST EDITION" {
		t.Errorf("CardName = %q, want %q", certInfo.CardName, "CHARIZARD-HOLO 1ST EDITION")
	}
	if certInfo.Grade != 10 {
		t.Errorf("Grade = %v, want 10", certInfo.Grade)
	}
	if certInfo.Year != "1999" {
		t.Errorf("Year = %q, want %q", certInfo.Year, "1999")
	}
	if certInfo.CardNumber != "004" {
		t.Errorf("CardNumber = %q, want %q", certInfo.CardNumber, "004")
	}
	if certInfo.Population != 120 {
		t.Errorf("Population = %d, want 120", certInfo.Population)
	}
}

func TestCertAdapter_LookupImages(t *testing.T) {
	cases := []struct {
		name      string
		handler   http.HandlerFunc
		wantFront string
		wantBack  string
	}{
		{
			name: "front and back returned",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/cert/GetImagesByCertNumber/12345678" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]ImageInfo{
					{IsFrontImage: true, ImageURL: "https://example.com/front.jpg"},
					{IsFrontImage: false, ImageURL: "https://example.com/back.jpg"},
				})
			},
			wantFront: "https://example.com/front.jpg",
			wantBack:  "https://example.com/back.jpg",
		},
		{
			name: "empty response",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `[]`)
			},
			wantFront: "",
			wantBack:  "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(tc.handler)
			defer server.Close()

			adapter := NewCertAdapter(newTestClient(t, server.URL))
			front, back, err := adapter.LookupImages(context.Background(), "12345678")
			if err != nil {
				t.Fatalf("LookupImages error: %v", err)
			}
			if front != tc.wantFront {
				t.Errorf("front = %q, want %q", front, tc.wantFront)
			}
			if back != tc.wantBack {
				t.Errorf("back = %q, want %q", back, tc.wantBack)
			}
		})
	}
}

func TestCertAdapter_LookupCert_FallbackGradeDescription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := CertResponse{
			PSACert: CertInfo{
				CertNumber:       "99999",
				Subject:          "PIKACHU",
				CardGrade:        "AUTHENTIC", // no numeric grade
				GradeDescription: "NM-MT 8",   // fallback
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(t, server.URL)
	adapter := NewCertAdapter(c)

	certInfo, err := adapter.LookupCert(context.Background(), "99999")
	if err != nil {
		t.Fatalf("LookupCert error: %v", err)
	}
	if certInfo.Grade != 8 {
		t.Errorf("Grade = %v, want 8 (from GradeDescription fallback)", certInfo.Grade)
	}
}

// --- tokenPool ---

func TestTokenPool_NextSkipsDeadAndSpent(t *testing.T) {
	tests := []struct {
		name    string
		tokens  []string
		dead    []string
		spent   []string
		want    string
		wantOK  bool
		wantCnt int
	}{
		{
			name:    "all usable returns first",
			tokens:  []string{"a", "b", "c"},
			want:    "a",
			wantOK:  true,
			wantCnt: 3,
		},
		{
			name:    "skips dead key",
			tokens:  []string{"a", "b"},
			dead:    []string{"a"},
			want:    "b",
			wantOK:  true,
			wantCnt: 1,
		},
		{
			name:    "skips spent key",
			tokens:  []string{"a", "b"},
			spent:   []string{"a"},
			want:    "b",
			wantOK:  true,
			wantCnt: 1,
		},
		{
			name:    "dead and spent leaves one",
			tokens:  []string{"a", "b", "c"},
			dead:    []string{"a"},
			spent:   []string{"b"},
			want:    "c",
			wantOK:  true,
			wantCnt: 1,
		},
		{
			name:    "everything unusable",
			tokens:  []string{"a", "b"},
			dead:    []string{"a"},
			spent:   []string{"b"},
			wantOK:  false,
			wantCnt: 0,
		},
		{
			name:    "empty pool",
			tokens:  nil,
			wantOK:  false,
			wantCnt: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newTokenPool(tc.tokens)
			for _, d := range tc.dead {
				p.markDead(d)
			}
			for _, s := range tc.spent {
				p.markSpent(s)
			}
			got, ok := p.next()
			if ok != tc.wantOK {
				t.Fatalf("next() ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Errorf("next() = %q, want %q", got, tc.want)
			}
			if n := p.usableCount(); n != tc.wantCnt {
				t.Errorf("usableCount() = %d, want %d", n, tc.wantCnt)
			}
		})
	}
}

func TestTokenPool_AdvanceMovesToNextUsable(t *testing.T) {
	p := newTokenPool([]string{"a", "b"})
	if got, _ := p.next(); got != "a" {
		t.Fatalf("initial next() = %q, want %q", got, "a")
	}
	if !p.advance() {
		t.Fatal("advance() = false, want true with a second usable key")
	}
	if got, _ := p.next(); got != "b" {
		t.Errorf("after advance, next() = %q, want %q", got, "b")
	}

	// With the only remaining key retired, advance reports exhaustion.
	p.markDead("b")
	p.markDead("a")
	if p.advance() {
		t.Error("advance() = true, want false when no key is usable")
	}
}

// TestTokenPool_SpentClearsOnDayRollover proves the spent marker is keyed on
// the UTC day, so a key exhausted yesterday is usable again today. Dead keys
// have no such reprieve.
func TestTokenPool_SpentClearsOnDayRollover(t *testing.T) {
	p := newTokenPool([]string{"a"})
	p.mu.Lock()
	p.spent[tokenDayKey{token: "a", day: "2000-01-01"}] = true
	p.mu.Unlock()

	got, ok := p.next()
	if !ok || got != "a" {
		t.Fatalf("next() = (%q, %v), want (\"a\", true) — stale spent marker not pruned", got, ok)
	}
	p.mu.Lock()
	stale := len(p.spent)
	p.mu.Unlock()
	if stale != 0 {
		t.Errorf("stale spent entries = %d, want 0", stale)
	}
}

func TestTokenPool_Stats(t *testing.T) {
	p := newTokenPool([]string{"a", "b", "c"})
	p.markDead("a")
	p.markSpent("b")
	total, dead, spent := p.stats()
	if total != 3 || dead != 1 || spent != 1 {
		t.Errorf("stats() = (%d, %d, %d), want (3, 1, 1)", total, dead, spent)
	}
}

// --- Error type assertions ---

func TestClient_GetCert_ErrorTypes(t *testing.T) {
	t.Run("no tokens returns ConfigMissing", func(t *testing.T) {
		c := &Client{
			httpClient: httpx.NewClient(httpx.DefaultConfig("test")),
			baseURL:    "http://localhost",
			pool:       newTokenPool(nil),
			logger:     observability.NewNoopLogger(),
		}
		_, err := c.GetCert(context.Background(), "12345678")
		var appErr *apperrors.AppError
		if !errors.As(err, &appErr) {
			t.Fatalf("expected AppError, got %T: %v", err, err)
		}
		if appErr.Code != apperrors.ErrCodeConfigMissing {
			t.Errorf("expected ConfigMissing, got %s", appErr.Code)
		}
	})

	t.Run("cert not found returns ProviderNotFound", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(CertResponse{PSACert: CertInfo{CertNumber: ""}})
		}))
		defer server.Close()

		c := newTestClient(t, server.URL)
		_, err := c.GetCert(context.Background(), "12345678")
		var appErr *apperrors.AppError
		if !errors.As(err, &appErr) {
			t.Fatalf("expected AppError, got %T: %v", err, err)
		}
		if appErr.Code != apperrors.ErrCodeProviderNotFound {
			t.Errorf("expected ProviderNotFound, got %s", appErr.Code)
		}
	})

	t.Run("rate limited returns ProviderRateLimited", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer server.Close()

		c := newTestClient(t, server.URL) // single token, cannot rotate
		_, err := c.GetCert(context.Background(), "12345678")
		var appErr *apperrors.AppError
		if !errors.As(err, &appErr) {
			t.Fatalf("expected AppError, got %T: %v", err, err)
		}
		if appErr.Code != apperrors.ErrCodeProviderRateLimit {
			t.Errorf("expected ProviderRateLimit, got %s", appErr.Code)
		}
	})

	t.Run("decode error returns ProviderInvalidResponse", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{invalid json}`)
		}))
		defer server.Close()

		c := newTestClient(t, server.URL)
		_, err := c.GetCert(context.Background(), "12345678")
		var appErr *apperrors.AppError
		if !errors.As(err, &appErr) {
			t.Fatalf("expected AppError, got %T: %v", err, err)
		}
		if appErr.Code != apperrors.ErrCodeProviderInvalidResp {
			t.Errorf("expected ProviderInvalidResp, got %s", appErr.Code)
		}
	})

	t.Run("exhausted pool returns ProviderRateLimited", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer server.Close()

		c := newTestClient(t, server.URL) // single token
		if _, err := c.GetCert(context.Background(), "111"); err == nil {
			t.Fatal("expected first call to 429")
		}

		// The key is now marked spent, so the second call short-circuits
		// without another HTTP request and still reports a rate limit.
		_, err := c.GetCert(context.Background(), "111")
		var appErr *apperrors.AppError
		if !errors.As(err, &appErr) {
			t.Fatalf("expected AppError, got %T: %v", err, err)
		}
		if appErr.Code != apperrors.ErrCodeProviderRateLimit {
			t.Errorf("expected ProviderRateLimit, got %s", appErr.Code)
		}
	})
}
