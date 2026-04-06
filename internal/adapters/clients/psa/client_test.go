package psa

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
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

func TestGetCert_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cert/GetByCertNumber/12345678" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		resp := CertResponse{
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
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := newTestClient(t, server.URL)
	info, err := c.GetCert(context.Background(), "12345678")
	if err != nil {
		t.Fatalf("GetCert returned error: %v", err)
	}
	if info.CertNumber != "12345678" {
		t.Errorf("CertNumber = %q, want %q", info.CertNumber, "12345678")
	}
	if info.Subject != "CHARIZARD-HOLO" {
		t.Errorf("Subject = %q, want %q", info.Subject, "CHARIZARD-HOLO")
	}
	if info.TotalPopulation != 120 {
		t.Errorf("TotalPopulation = %d, want 120", info.TotalPopulation)
	}
}

func TestGetCert_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Return empty cert (CertNumber == "")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"PSACert":{}}`)
	}))
	defer server.Close()

	c := newTestClient(t, server.URL)
	_, err := c.GetCert(context.Background(), "00000000")
	if err == nil {
		t.Fatal("expected error for not-found cert")
	}
	if got := err.Error(); got != "cert 00000000 not found" {
		t.Errorf("error = %q, want 'cert 00000000 not found'", got)
	}
}

func TestGetCert_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{invalid json}`)
	}))
	defer server.Close()

	c := newTestClient(t, server.URL)
	_, err := c.GetCert(context.Background(), "12345678")
	if err == nil {
		t.Fatal("expected decode error")
	}
}

// --- GetImages ---

func TestGetImages_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cert/GetImagesByCertNumber/12345678" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		images := []ImageInfo{
			{IsFrontImage: true, ImageURL: "https://example.com/front.jpg"},
			{IsFrontImage: false, ImageURL: "https://example.com/back.jpg"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(images)
	}))
	defer server.Close()

	c := newTestClient(t, server.URL)
	images, err := c.GetImages(context.Background(), "12345678")
	if err != nil {
		t.Fatalf("GetImages returned error: %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("got %d images, want 2", len(images))
	}
	if !images[0].IsFrontImage {
		t.Error("first image should be front")
	}
	if images[1].IsFrontImage {
		t.Error("second image should be back")
	}
}

func TestGetImages_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `not json`)
	}))
	defer server.Close()

	c := newTestClient(t, server.URL)
	_, err := c.GetImages(context.Background(), "12345678")
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestGetImages_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	defer server.Close()

	c := newTestClient(t, server.URL)
	images, err := c.GetImages(context.Background(), "12345678")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(images) != 0 {
		t.Errorf("got %d images, want 0", len(images))
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
		resp := CertResponse{PSACert: CertInfo{CertNumber: "99999"}}
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
		resp := CertResponse{PSACert: CertInfo{CertNumber: "111"}}
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
		resp := CertResponse{PSACert: CertInfo{CertNumber: "333"}}
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
		resp := CertResponse{PSACert: CertInfo{CertNumber: "444"}}
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

	// Should have waited at least some portion of minRequestSpacing.
	// Use a generous lower bound to avoid flakiness.
	if elapsed < 200*time.Millisecond {
		t.Errorf("expected pacing delay, but second call completed in %v", elapsed)
	}
}

// --- doRequest: context cancellation during pacing ---

func TestDoRequest_ContextCancelledDuringPacing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := CertResponse{PSACert: CertInfo{CertNumber: "555"}}
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

// --- BackfillImages ---

type mockLister struct {
	rows []PurchaseImageRow
	err  error
}

func (m *mockLister) ListPurchasesMissingImages(_ context.Context, _ int) ([]PurchaseImageRow, error) {
	return m.rows, m.err
}

type mockUpdater struct {
	updated map[string][2]string // id -> [frontURL, backURL]
	err     error
}

func (m *mockUpdater) UpdatePurchaseImageURLs(_ context.Context, id, frontURL, backURL string) error {
	if m.err != nil {
		return m.err
	}
	if m.updated == nil {
		m.updated = make(map[string][2]string)
	}
	m.updated[id] = [2]string{frontURL, backURL}
	return nil
}

func TestBackfillImages_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		images := []ImageInfo{
			{IsFrontImage: true, ImageURL: "https://example.com/front.jpg"},
			{IsFrontImage: false, ImageURL: "https://example.com/back.jpg"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(images)
	}))
	defer server.Close()

	c := newTestClient(t, server.URL)
	lister := &mockLister{rows: []PurchaseImageRow{
		{ID: "p1", CertNumber: "111"},
	}}
	updater := &mockUpdater{}
	bf := NewImageBackfiller(c, lister, updater, observability.NewNoopLogger())

	updated, errors, err := bf.BackfillImages(context.Background())
	if err != nil {
		t.Fatalf("BackfillImages error: %v", err)
	}
	if updated != 1 {
		t.Errorf("updated = %d, want 1", updated)
	}
	if errors != 0 {
		t.Errorf("errors = %d, want 0", errors)
	}
	if urls, ok := updater.updated["p1"]; !ok {
		t.Error("p1 not updated")
	} else {
		if urls[0] != "https://example.com/front.jpg" {
			t.Errorf("front URL = %q", urls[0])
		}
		if urls[1] != "https://example.com/back.jpg" {
			t.Errorf("back URL = %q", urls[1])
		}
	}
}

func TestBackfillImages_NoPurchases(t *testing.T) {
	c := newTestClient(t, "http://unused")
	lister := &mockLister{rows: nil}
	updater := &mockUpdater{}
	bf := NewImageBackfiller(c, lister, updater, observability.NewNoopLogger())

	updated, errors, err := bf.BackfillImages(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated != 0 || errors != 0 {
		t.Errorf("updated=%d errors=%d, want 0,0", updated, errors)
	}
}

func TestBackfillImages_ListerError(t *testing.T) {
	c := newTestClient(t, "http://unused")
	lister := &mockLister{err: fmt.Errorf("db error")}
	bf := NewImageBackfiller(c, lister, &mockUpdater{}, observability.NewNoopLogger())

	_, _, err := bf.BackfillImages(context.Background())
	if err == nil {
		t.Fatal("expected lister error")
	}
}

func TestBackfillImages_PartialFailures(t *testing.T) {
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		call := callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if call == 1 {
			// First cert: return images with no front image
			json.NewEncoder(w).Encode([]ImageInfo{
				{IsFrontImage: false, ImageURL: "https://example.com/back-only.jpg"},
			})
			return
		}
		// Second cert: success
		json.NewEncoder(w).Encode([]ImageInfo{
			{IsFrontImage: true, ImageURL: "https://example.com/front2.jpg"},
		})
	}))
	defer server.Close()

	c := newTestClient(t, server.URL)
	lister := &mockLister{rows: []PurchaseImageRow{
		{ID: "p1", CertNumber: "aaa"},
		{ID: "p2", CertNumber: "bbb"},
	}}
	updater := &mockUpdater{}
	bf := NewImageBackfiller(c, lister, updater, observability.NewNoopLogger())

	updated, errors, err := bf.BackfillImages(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated != 1 {
		t.Errorf("updated = %d, want 1", updated)
	}
	if errors != 1 {
		t.Errorf("errors = %d, want 1 (no front image)", errors)
	}
}

func TestBackfillImages_UpdaterError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]ImageInfo{
			{IsFrontImage: true, ImageURL: "https://example.com/front.jpg"},
		})
	}))
	defer server.Close()

	c := newTestClient(t, server.URL)
	lister := &mockLister{rows: []PurchaseImageRow{{ID: "p1", CertNumber: "111"}}}
	updater := &mockUpdater{err: fmt.Errorf("update failed")}
	bf := NewImageBackfiller(c, lister, updater, observability.NewNoopLogger())

	updated, errors, err := bf.BackfillImages(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated != 0 {
		t.Errorf("updated = %d, want 0", updated)
	}
	if errors != 1 {
		t.Errorf("errors = %d, want 1", errors)
	}
}

func TestBackfillImages_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]ImageInfo{
			{IsFrontImage: true, ImageURL: "https://example.com/front.jpg"},
		})
	}))
	defer server.Close()

	c := newTestClient(t, server.URL)
	lister := &mockLister{rows: []PurchaseImageRow{
		{ID: "p1", CertNumber: "111"},
		{ID: "p2", CertNumber: "222"},
		{ID: "p3", CertNumber: "333"},
	}}
	updater := &mockUpdater{}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after the first iteration's throttle delay would start.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	bf := NewImageBackfiller(c, lister, updater, observability.NewNoopLogger())
	_, _, err := bf.BackfillImages(ctx)
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
