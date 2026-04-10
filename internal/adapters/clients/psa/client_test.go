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

// --- doRequest: daily call limit ---

func TestDoRequest_DailyCallLimitEnforced(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		resp := CertResponse{PSACert: CertInfo{CertNumber: "111", CardGrade: "MINT 9"}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(t, server.URL)
	today := time.Now().UTC().Format("2006-01-02")
	c.dailyCounts.mu.Lock()
	c.dailyCounts.counts[tokenDayKey{token: "test-token", day: today}] = dailyCallLimit - 1
	c.dailyCounts.mu.Unlock()

	_, err := c.GetCert(context.Background(), "111")
	if err != nil {
		t.Fatalf("expected first call to succeed: %v", err)
	}

	_, err = c.GetCert(context.Background(), "222")
	if err == nil {
		t.Fatal("expected daily limit error")
	}
}

func TestDoRequest_DailyLimitRotatesToNextToken(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		auth := r.Header.Get("Authorization")
		if auth != "Bearer token-b" {
			t.Errorf("expected token-b after rotation, got %s", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		resp := CertResponse{PSACert: CertInfo{CertNumber: "333", CardGrade: "NM-MT 8"}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(t, server.URL, "token-a", "token-b")
	today := time.Now().UTC().Format("2006-01-02")
	c.dailyCounts.mu.Lock()
	c.dailyCounts.counts[tokenDayKey{token: "token-a", day: today}] = dailyCallLimit
	c.dailyCounts.mu.Unlock()

	info, err := c.GetCert(context.Background(), "333")
	if err != nil {
		t.Fatalf("expected success with token-b: %v", err)
	}
	if info.CertNumber != "333" {
		t.Errorf("CertNumber = %q, want %q", info.CertNumber, "333")
	}
}

// --- doRequest: no tokens configured ---

func TestDoRequest_NoTokensConfigured(t *testing.T) {
	c := &Client{
		httpClient:  httpx.NewClient(httpx.DefaultConfig("test")),
		baseURL:     "http://localhost",
		tokens:      nil,
		logger:      observability.NewNoopLogger(),
		dailyCounts: newTokenDayCounter(),
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

// --- tokenDayCounter ---

func TestTokenDayCounter_Increments(t *testing.T) {
	c := newTokenDayCounter()
	if got := c.add("tok1"); got != 1 {
		t.Errorf("first add = %d, want 1", got)
	}
	if got := c.add("tok1"); got != 2 {
		t.Errorf("second add = %d, want 2", got)
	}
	// Different token starts at 1.
	if got := c.add("tok2"); got != 1 {
		t.Errorf("tok2 first add = %d, want 1", got)
	}
}

// --- currentToken / rotateToken ---

func TestCurrentToken_EmptyTokens(t *testing.T) {
	c := &Client{tokens: nil}
	if got := c.currentToken(); got != "" {
		t.Errorf("currentToken with no tokens = %q, want empty", got)
	}
}

func TestRotateToken_SingleToken(t *testing.T) {
	c := &Client{
		tokens: []string{"only-one"},
		logger: observability.NewNoopLogger(),
	}
	if c.rotateToken() {
		t.Error("rotateToken should return false with single token")
	}
}

func TestRotateToken_MultipleTokens(t *testing.T) {
	c := &Client{
		tokens: []string{"a", "b", "c"},
		logger: observability.NewNoopLogger(),
	}
	if !c.rotateToken() {
		t.Error("rotateToken should return true with multiple tokens")
	}
	if got := c.currentToken(); got != "b" {
		t.Errorf("after rotation, currentToken = %q, want %q", got, "b")
	}
}

// --- Error type assertions ---

func TestClient_GetCert_ErrorTypes(t *testing.T) {
	t.Run("no tokens returns ConfigMissing", func(t *testing.T) {
		c := &Client{
			httpClient:  httpx.NewClient(httpx.DefaultConfig("test")),
			baseURL:     "http://localhost",
			tokens:      nil,
			logger:      observability.NewNoopLogger(),
			dailyCounts: newTokenDayCounter(),
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

	t.Run("daily limit returns ProviderRateLimited", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			resp := CertResponse{PSACert: CertInfo{CertNumber: "111", CardGrade: "MINT 9"}}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		c := newTestClient(t, server.URL) // single token
		today := time.Now().UTC().Format("2006-01-02")
		c.dailyCounts.mu.Lock()
		c.dailyCounts.counts[tokenDayKey{token: "test-token", day: today}] = dailyCallLimit + 1
		c.dailyCounts.mu.Unlock()

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
