# Cert Import UX Redesign — Streaming Scanner

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the batch textarea cert import with a streaming scanner that gives instant per-cert feedback.

**Architecture:** Two new thin backend endpoints (`scan-cert`, `resolve-cert`) delegate to service methods that reuse existing repo queries. The frontend `CertEntryTab.tsx` is rewritten to a scan-loop pattern with per-row state management. No database schema changes.

**Tech Stack:** Go 1.26, React/TypeScript, SQLite, existing hexagonal architecture patterns.

**Spec:** `docs/superpowers/specs/2026-04-05-cert-import-ux-design.md`

---

### Task 1: Add domain types for scan-cert and resolve-cert

**Files:**
- Modify: `internal/domain/campaigns/ebay_types.go`

- [ ] **Step 1: Add ScanCertRequest and ScanCertResult types**

Append to `internal/domain/campaigns/ebay_types.go`:

```go
// ScanCertRequest is the input for POST /api/purchases/scan-cert.
type ScanCertRequest struct {
	CertNumber string `json:"certNumber"`
}

// ScanCertResult is the response from POST /api/purchases/scan-cert.
type ScanCertResult struct {
	Status     string `json:"status"`               // "existing", "sold", "new"
	CardName   string `json:"cardName,omitempty"`
	PurchaseID string `json:"purchaseId,omitempty"`
	CampaignID string `json:"campaignId,omitempty"`
}

// ResolveCertRequest is the input for POST /api/purchases/resolve-cert.
type ResolveCertRequest struct {
	CertNumber string `json:"certNumber"`
}

// ResolveCertResult is the response from POST /api/purchases/resolve-cert.
type ResolveCertResult struct {
	CertNumber string  `json:"certNumber"`
	CardName   string  `json:"cardName"`
	Grade      float64 `json:"grade"`
	Year       string  `json:"year"`
	Category   string  `json:"category"`
	Subject    string  `json:"subject"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /workspace && go build ./internal/domain/campaigns/...`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/domain/campaigns/ebay_types.go
git commit -m "feat(cert-import): add domain types for scan-cert and resolve-cert"
```

---

### Task 2: Add ScanCert service method with tests

**Files:**
- Modify: `internal/domain/campaigns/service.go` (Service interface)
- Modify: `internal/domain/campaigns/service_cert_entry.go` (implementation)
- Modify: `internal/domain/campaigns/service_cert_entry_test.go` (tests)
- Modify: `internal/testutil/mocks/campaign_service.go` (mock)

- [ ] **Step 1: Add ScanCert to the Service interface**

In `internal/domain/campaigns/service.go`, after line 239 (`GetPurchasesByCertNumbers`), add:

```go
	ScanCert(ctx context.Context, certNumber string) (*ScanCertResult, error)
	ResolveCert(ctx context.Context, certNumber string) (*CertInfo, error)
```

- [ ] **Step 2: Add mock implementations**

In `internal/testutil/mocks/campaign_service.go`, add the Fn fields after `GetPurchasesByCertNumbersFn` (around line 132):

```go
	ScanCertFn    func(ctx context.Context, certNumber string) (*campaigns.ScanCertResult, error)
	ResolveCertFn func(ctx context.Context, certNumber string) (*campaigns.CertInfo, error)
```

Add the method implementations before the `Close()` method (around line 684):

```go
func (m *MockCampaignService) ScanCert(ctx context.Context, certNumber string) (*campaigns.ScanCertResult, error) {
	if m.ScanCertFn != nil {
		return m.ScanCertFn(ctx, certNumber)
	}
	return &campaigns.ScanCertResult{Status: "new"}, nil
}

func (m *MockCampaignService) ResolveCert(ctx context.Context, certNumber string) (*campaigns.CertInfo, error) {
	if m.ResolveCertFn != nil {
		return m.ResolveCertFn(ctx, certNumber)
	}
	return nil, nil
}
```

- [ ] **Step 3: Write failing tests for ScanCert**

Append to `internal/domain/campaigns/service_cert_entry_test.go`:

```go
func TestScanCert(t *testing.T) {
	tests := []struct {
		name       string
		seed       func(*mockRepo)
		certNumber string
		wantStatus string
		wantCard   string
	}{
		{
			name: "existing cert not sold",
			seed: func(r *mockRepo) {
				r.purchases["p1"] = &Purchase{
					ID: "p1", CertNumber: "11111111", Grader: "PSA",
					CardName: "Charizard", CampaignID: "camp-1",
				}
			},
			certNumber: "11111111",
			wantStatus: "existing",
			wantCard:   "Charizard",
		},
		{
			name: "sold cert",
			seed: func(r *mockRepo) {
				r.purchases["p1"] = &Purchase{
					ID: "p1", CertNumber: "22222222", Grader: "PSA",
					CardName: "Pikachu", CampaignID: "camp-1",
				}
				r.sales["s1"] = &Sale{ID: "s1", PurchaseID: "p1"}
				r.purchaseSales["p1"] = true
			},
			certNumber: "22222222",
			wantStatus: "sold",
			wantCard:   "Pikachu",
		},
		{
			name:       "new cert not in DB",
			seed:       func(_ *mockRepo) {},
			certNumber: "33333333",
			wantStatus: "new",
			wantCard:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepo()
			tc.seed(repo)
			svc := &service{repo: repo, idGen: func() string { return "test-id" }}

			result, err := svc.ScanCert(context.Background(), tc.certNumber)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", result.Status, tc.wantStatus)
			}
			if result.CardName != tc.wantCard {
				t.Errorf("cardName = %q, want %q", result.CardName, tc.wantCard)
			}
		})
	}
}

func TestScanCert_ExistingSetsExportFlag(t *testing.T) {
	repo := newMockRepo()
	repo.purchases["p1"] = &Purchase{
		ID: "p1", CertNumber: "11111111", Grader: "PSA",
		CardName: "Charizard", CampaignID: "camp-1",
	}

	svc := &service{repo: repo, idGen: func() string { return "test-id" }}

	_, err := svc.ScanCert(context.Background(), "11111111")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.purchases["p1"].EbayExportFlaggedAt == nil {
		t.Error("expected ebay export flag to be set for existing cert")
	}
}
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `cd /workspace && go test ./internal/domain/campaigns/ -run TestScanCert -v`
Expected: FAIL — `ScanCert` method not defined yet.

- [ ] **Step 5: Implement ScanCert**

Add to `internal/domain/campaigns/service_cert_entry.go`, after the `GetPurchasesByCertNumbers` method:

```go
// ScanCert checks a single cert against the database and returns its status.
// For existing (unsold) certs, it also sets the eBay export flag.
func (s *service) ScanCert(ctx context.Context, certNumber string) (*ScanCertResult, error) {
	certNumber = strings.TrimSpace(certNumber)
	if certNumber == "" {
		return nil, fmt.Errorf("cert number is required")
	}

	existingMap, err := s.repo.GetPurchasesByGraderAndCertNumbers(ctx, "PSA", []string{certNumber})
	if err != nil {
		return nil, fmt.Errorf("scan cert lookup: %w", err)
	}

	existing, found := existingMap[certNumber]
	if !found {
		return &ScanCertResult{Status: "new"}, nil
	}

	// Check if sold
	salesMap, err := s.repo.GetSalesByPurchaseIDs(ctx, []string{existing.ID})
	if err != nil {
		return nil, fmt.Errorf("scan cert sale check: %w", err)
	}

	if _, hasSale := salesMap[existing.ID]; hasSale {
		return &ScanCertResult{
			Status:     "sold",
			CardName:   existing.CardName,
			PurchaseID: existing.ID,
			CampaignID: existing.CampaignID,
		}, nil
	}

	// Existing and not sold — flag for eBay export
	now := time.Now()
	if flagErr := s.repo.SetEbayExportFlag(ctx, existing.ID, now); flagErr != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "scan cert: failed to set ebay export flag",
				observability.String("cert", certNumber),
				observability.Err(flagErr))
		}
	}

	return &ScanCertResult{
		Status:     "existing",
		CardName:   existing.CardName,
		PurchaseID: existing.ID,
		CampaignID: existing.CampaignID,
	}, nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /workspace && go test ./internal/domain/campaigns/ -run TestScanCert -v`
Expected: PASS — all 4 test cases pass.

- [ ] **Step 7: Commit**

```bash
git add internal/domain/campaigns/service.go internal/domain/campaigns/service_cert_entry.go internal/domain/campaigns/service_cert_entry_test.go internal/testutil/mocks/campaign_service.go
git commit -m "feat(cert-import): add ScanCert service method with tests"
```

---

### Task 3: Add ResolveCert service method with tests

**Files:**
- Modify: `internal/domain/campaigns/service_cert_entry.go` (implementation)
- Modify: `internal/domain/campaigns/service_cert_entry_test.go` (tests)

- [ ] **Step 1: Write failing tests for ResolveCert**

Append to `internal/domain/campaigns/service_cert_entry_test.go`:

```go
func TestResolveCert(t *testing.T) {
	tests := []struct {
		name       string
		certNumber string
		lookupFn   func(ctx context.Context, certNumber string) (*CertInfo, error)
		wantErr    bool
		wantName   string
	}{
		{
			name:       "successful lookup",
			certNumber: "44444444",
			lookupFn: func(_ context.Context, cert string) (*CertInfo, error) {
				return &CertInfo{
					CertNumber: cert, CardName: "Umbreon VMAX", Grade: 10,
					Year: "2022", Category: "EVOLVING SKIES", Subject: "2022 Pokemon Evolving Skies Umbreon VMAX",
				}, nil
			},
			wantErr:  false,
			wantName: "Umbreon VMAX",
		},
		{
			name:       "cert not found",
			certNumber: "00000000",
			lookupFn: func(_ context.Context, _ string) (*CertInfo, error) {
				return nil, fmt.Errorf("cert 00000000 not found")
			},
			wantErr: true,
		},
		{
			name:       "no cert lookup configured",
			certNumber: "55555555",
			lookupFn:   nil,
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepo()
			var certLookup CertLookup
			if tc.lookupFn != nil {
				certLookup = &mockCertLookup{lookupFn: tc.lookupFn}
			}
			svc := &service{repo: repo, certLookup: certLookup, idGen: func() string { return "test-id" }}

			info, err := svc.ResolveCert(context.Background(), tc.certNumber)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if info.CardName != tc.wantName {
				t.Errorf("cardName = %q, want %q", info.CardName, tc.wantName)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /workspace && go test ./internal/domain/campaigns/ -run TestResolveCert -v`
Expected: FAIL — `ResolveCert` method not defined yet.

- [ ] **Step 3: Implement ResolveCert**

Add to `internal/domain/campaigns/service_cert_entry.go`, after `ScanCert`:

```go
// ResolveCert looks up a PSA cert number via the external PSA API.
// Returns card info for preview; does NOT create a purchase.
func (s *service) ResolveCert(ctx context.Context, certNumber string) (*CertInfo, error) {
	certNumber = strings.TrimSpace(certNumber)
	if certNumber == "" {
		return nil, fmt.Errorf("cert number is required")
	}

	if s.certLookup == nil {
		return nil, ErrCertLookupNotConfigured
	}

	info, err := s.certLookup.LookupCert(ctx, certNumber)
	if err != nil {
		return nil, fmt.Errorf("resolve cert %s: %w", certNumber, err)
	}
	if info == nil {
		return nil, fmt.Errorf("cert %s not found", certNumber)
	}

	return info, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /workspace && go test ./internal/domain/campaigns/ -run TestResolveCert -v`
Expected: PASS — all 3 test cases pass.

- [ ] **Step 5: Run all cert entry tests together**

Run: `cd /workspace && go test ./internal/domain/campaigns/ -run "TestScanCert|TestResolveCert|TestImportCerts" -v`
Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/campaigns/service_cert_entry.go internal/domain/campaigns/service_cert_entry_test.go
git commit -m "feat(cert-import): add ResolveCert service method with tests"
```

---

### Task 4: Add HTTP handlers for scan-cert and resolve-cert

**Files:**
- Modify: `internal/adapters/httpserver/handlers/campaigns_imports.go`
- Modify: `internal/adapters/httpserver/routes.go`

- [ ] **Step 1: Add HandleScanCert handler**

Add to `internal/adapters/httpserver/handlers/campaigns_imports.go`, after the `HandleImportCerts` method (around line 393):

```go
// HandleScanCert handles POST /api/purchases/scan-cert.
func (h *CampaignsHandler) HandleScanCert(w http.ResponseWriter, r *http.Request) {
	const maxBytes = 1 << 10 // 1KB — single cert number
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	var req campaigns.ScanCertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if req.CertNumber == "" {
		writeError(w, http.StatusBadRequest, "certNumber is required")
		return
	}

	result, err := h.service.ScanCert(r.Context(), req.CertNumber)
	if err != nil {
		h.logger.Error(r.Context(), "scan cert failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	writeJSON(w, http.StatusOK, result)
}
```

- [ ] **Step 2: Add HandleResolveCert handler**

Add directly after `HandleScanCert`:

```go
// HandleResolveCert handles POST /api/purchases/resolve-cert.
func (h *CampaignsHandler) HandleResolveCert(w http.ResponseWriter, r *http.Request) {
	const maxBytes = 1 << 10 // 1KB — single cert number
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	var req campaigns.ResolveCertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if req.CertNumber == "" {
		writeError(w, http.StatusBadRequest, "certNumber is required")
		return
	}

	info, err := h.service.ResolveCert(r.Context(), req.CertNumber)
	if err != nil {
		h.logger.Warn(r.Context(), "resolve cert failed",
			observability.String("cert", req.CertNumber),
			observability.Err(err))
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, campaigns.ResolveCertResult{
		CertNumber: info.CertNumber,
		CardName:   info.CardName,
		Grade:      info.Grade,
		Year:       info.Year,
		Category:   info.Category,
		Subject:    info.Subject,
	})
}
```

- [ ] **Step 3: Register routes**

In `internal/adapters/httpserver/routes.go`, after the `import-certs` route (line 157), add:

```go
	mux.Handle("POST /api/purchases/scan-cert", authRoute(rt.campaignsHandler.HandleScanCert))
	mux.Handle("POST /api/purchases/resolve-cert", authRoute(rt.campaignsHandler.HandleResolveCert))
```

- [ ] **Step 4: Verify it compiles**

Run: `cd /workspace && go build ./...`
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/httpserver/handlers/campaigns_imports.go internal/adapters/httpserver/routes.go
git commit -m "feat(cert-import): add scan-cert and resolve-cert HTTP handlers and routes"
```

---

### Task 5: Add handler tests for scan-cert and resolve-cert

**Files:**
- Modify: `internal/adapters/httpserver/handlers/campaigns_imports_test.go`

- [ ] **Step 1: Write handler tests**

Append to `internal/adapters/httpserver/handlers/campaigns_imports_test.go`:

```go
// --- HandleScanCert ---

func TestHandleScanCert_Existing(t *testing.T) {
	svc := &mocks.MockCampaignService{
		ScanCertFn: func(_ context.Context, cert string) (*campaigns.ScanCertResult, error) {
			return &campaigns.ScanCertResult{
				Status:     "existing",
				CardName:   "Charizard PSA 10",
				PurchaseID: "p1",
				CampaignID: "c1",
			}, nil
		},
	}
	h := newTestHandler(svc)

	body := strings.NewReader(`{"certNumber":"12345678"}`)
	req := httptest.NewRequest("POST", "/api/purchases/scan-cert", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleScanCert(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var result campaigns.ScanCertResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "existing" {
		t.Errorf("status = %q, want existing", result.Status)
	}
	if result.CardName != "Charizard PSA 10" {
		t.Errorf("cardName = %q, want Charizard PSA 10", result.CardName)
	}
}

func TestHandleScanCert_EmptyCert(t *testing.T) {
	h := newTestHandler(&mocks.MockCampaignService{})

	body := strings.NewReader(`{"certNumber":""}`)
	req := httptest.NewRequest("POST", "/api/purchases/scan-cert", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleScanCert(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleScanCert_InvalidJSON(t *testing.T) {
	h := newTestHandler(&mocks.MockCampaignService{})

	body := strings.NewReader(`not json`)
	req := httptest.NewRequest("POST", "/api/purchases/scan-cert", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleScanCert(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleScanCert_ServiceError(t *testing.T) {
	svc := &mocks.MockCampaignService{
		ScanCertFn: func(_ context.Context, _ string) (*campaigns.ScanCertResult, error) {
			return nil, fmt.Errorf("database failure")
		},
	}
	h := newTestHandler(svc)

	body := strings.NewReader(`{"certNumber":"111"}`)
	req := httptest.NewRequest("POST", "/api/purchases/scan-cert", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleScanCert(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", rec.Code, rec.Body.String())
	}
}

// --- HandleResolveCert ---

func TestHandleResolveCert_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		ResolveCertFn: func(_ context.Context, cert string) (*campaigns.CertInfo, error) {
			return &campaigns.CertInfo{
				CertNumber: cert, CardName: "Umbreon VMAX", Grade: 10,
				Year: "2022", Category: "EVOLVING SKIES",
				Subject: "2022 Pokemon Evolving Skies Umbreon VMAX",
			}, nil
		},
	}
	h := newTestHandler(svc)

	body := strings.NewReader(`{"certNumber":"91234567"}`)
	req := httptest.NewRequest("POST", "/api/purchases/resolve-cert", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleResolveCert(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var result campaigns.ResolveCertResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.CardName != "Umbreon VMAX" {
		t.Errorf("cardName = %q, want Umbreon VMAX", result.CardName)
	}
	if result.Grade != 10 {
		t.Errorf("grade = %v, want 10", result.Grade)
	}
}

func TestHandleResolveCert_NotFound(t *testing.T) {
	svc := &mocks.MockCampaignService{
		ResolveCertFn: func(_ context.Context, _ string) (*campaigns.CertInfo, error) {
			return nil, fmt.Errorf("cert 00000000 not found")
		},
	}
	h := newTestHandler(svc)

	body := strings.NewReader(`{"certNumber":"00000000"}`)
	req := httptest.NewRequest("POST", "/api/purchases/resolve-cert", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleResolveCert(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleResolveCert_EmptyCert(t *testing.T) {
	h := newTestHandler(&mocks.MockCampaignService{})

	body := strings.NewReader(`{"certNumber":""}`)
	req := httptest.NewRequest("POST", "/api/purchases/resolve-cert", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleResolveCert(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run handler tests**

Run: `cd /workspace && go test ./internal/adapters/httpserver/handlers/ -run "TestHandleScanCert|TestHandleResolveCert" -v`
Expected: PASS — all 7 tests pass.

- [ ] **Step 3: Run full test suite to check for regressions**

Run: `cd /workspace && go test ./...`
Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/httpserver/handlers/campaigns_imports_test.go
git commit -m "test(cert-import): add handler tests for scan-cert and resolve-cert"
```

---

### Task 6: Add frontend types and API client methods

**Files:**
- Modify: `web/src/types/campaigns/core.ts`
- Modify: `web/src/js/api/campaigns.ts`

- [ ] **Step 1: Add TypeScript types**

In `web/src/types/campaigns/core.ts`, after the `CertImportSoldItem` interface (around line 481), add:

```typescript
export interface ScanCertResponse {
  status: 'existing' | 'sold' | 'new';
  cardName?: string;
  purchaseId?: string;
  campaignId?: string;
}

export interface ResolveCertResponse {
  certNumber: string;
  cardName: string;
  grade: number;
  year: string;
  category: string;
  subject: string;
}
```

- [ ] **Step 2: Add API client methods — declaration merging**

In `web/src/js/api/campaigns.ts`, inside the `declare module './client'` block, after the `importCerts` declaration (around line 135), add:

```typescript
    scanCert(certNumber: string): Promise<ScanCertResponse>;
    resolveCert(certNumber: string): Promise<ResolveCertResponse>;
```

Also add `ScanCertResponse, ResolveCertResponse` to the import from `../../types/campaigns` (line 14-16 area).

- [ ] **Step 3: Add API client method implementations**

In `web/src/js/api/campaigns.ts`, after the `importCerts` implementation (around line 474), add:

```typescript
proto.scanCert = async function (
  this: APIClient, certNumber: string,
): Promise<ScanCertResponse> {
  return this.post<ScanCertResponse>('/purchases/scan-cert', { certNumber });
};

proto.resolveCert = async function (
  this: APIClient, certNumber: string,
): Promise<ResolveCertResponse> {
  return this.post<ResolveCertResponse>('/purchases/resolve-cert', { certNumber });
};
```

- [ ] **Step 4: Verify TypeScript compiles**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: no type errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/types/campaigns/core.ts web/src/js/api/campaigns.ts
git commit -m "feat(cert-import): add frontend types and API client methods for scan/resolve"
```

---

### Task 7: Rewrite CertEntryTab.tsx — streaming scanner component

**Files:**
- Rewrite: `web/src/react/pages/tools/CertEntryTab.tsx`

- [ ] **Step 1: Rewrite the component**

Replace the entire contents of `web/src/react/pages/tools/CertEntryTab.tsx` with:

```tsx
import { useState, useRef, useCallback, useEffect } from 'react';
import { api } from '@/js/api';
import type { ScanCertResponse, ResolveCertResponse, CertImportResult, CertImportSoldItem } from '@/types/campaigns/core';

type CertStatus = 'scanning' | 'existing' | 'sold' | 'returned' | 'resolving' | 'resolved' | 'failed' | 'importing' | 'imported';

interface CertRow {
  certNumber: string;
  status: CertStatus;
  cardName?: string;
  purchaseId?: string;
  campaignId?: string;
  error?: string;
}

export default function CertEntryTab() {
  const [input, setInput] = useState('');
  const [certs, setCerts] = useState<Map<string, CertRow>>(new Map());
  const [importLoading, setImportLoading] = useState(false);
  const [importError, setImportError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const highlightRef = useRef<string | null>(null);

  // Keep input focused
  useEffect(() => { inputRef.current?.focus(); }, []);

  const updateCert = useCallback((certNumber: string, updates: Partial<CertRow>) => {
    setCerts(prev => {
      const next = new Map(prev);
      const existing = next.get(certNumber);
      if (existing) {
        next.set(certNumber, { ...existing, ...updates });
      }
      return next;
    });
  }, []);

  const handleScan = useCallback(async (certNumber: string) => {
    certNumber = certNumber.trim();
    if (!certNumber) return;

    // Duplicate check — scroll to existing row
    if (certs.has(certNumber)) {
      highlightRef.current = certNumber;
      const el = document.getElementById(`cert-row-${certNumber}`);
      el?.scrollIntoView({ behavior: 'smooth', block: 'center' });
      el?.classList.add('ring-2', 'ring-yellow-400');
      setTimeout(() => el?.classList.remove('ring-2', 'ring-yellow-400'), 1500);
      return;
    }

    // Add row in scanning state
    setCerts(prev => {
      const next = new Map(prev);
      next.set(certNumber, { certNumber, status: 'scanning' });
      return next;
    });

    try {
      const result: ScanCertResponse = await api.scanCert(certNumber);

      if (result.status === 'existing') {
        updateCert(certNumber, {
          status: 'existing',
          cardName: result.cardName,
          purchaseId: result.purchaseId,
          campaignId: result.campaignId,
        });
      } else if (result.status === 'sold') {
        updateCert(certNumber, {
          status: 'sold',
          cardName: result.cardName,
          purchaseId: result.purchaseId,
          campaignId: result.campaignId,
        });
      } else {
        // New cert — trigger background resolve
        updateCert(certNumber, { status: 'resolving' });
        resolveInBackground(certNumber);
      }
    } catch (err) {
      updateCert(certNumber, {
        status: 'failed',
        error: err instanceof Error ? err.message : 'Scan failed',
      });
    }
  }, [certs, updateCert]);

  const resolveInBackground = useCallback(async (certNumber: string) => {
    try {
      const info: ResolveCertResponse = await api.resolveCert(certNumber);
      updateCert(certNumber, {
        status: 'resolved',
        cardName: info.cardName,
      });
    } catch (err) {
      updateCert(certNumber, {
        status: 'failed',
        error: err instanceof Error ? err.message : 'Cert not found',
      });
    }
  }, [updateCert]);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      handleScan(input);
      setInput('');
    }
  };

  const handleReturnToInventory = async (certNumber: string) => {
    const row = certs.get(certNumber);
    if (!row?.purchaseId || !row?.campaignId) return;

    updateCert(certNumber, { status: 'scanning' }); // reuse scanning as loading indicator
    try {
      await api.deleteSale(row.campaignId, row.purchaseId);
      updateCert(certNumber, { status: 'returned', cardName: row.cardName });
    } catch (err) {
      updateCert(certNumber, {
        status: 'sold', // revert to sold
        error: err instanceof Error ? err.message : 'Failed to return',
      });
    }
  };

  const handleDismiss = (certNumber: string) => {
    setCerts(prev => {
      const next = new Map(prev);
      next.delete(certNumber);
      return next;
    });
  };

  const handleImportNew = async () => {
    const resolvedCerts = Array.from(certs.values())
      .filter(c => c.status === 'resolved')
      .map(c => c.certNumber);

    if (resolvedCerts.length === 0) return;

    setImportLoading(true);
    setImportError(null);

    // Mark all resolved as importing
    for (const cn of resolvedCerts) {
      updateCert(cn, { status: 'importing' });
    }

    try {
      const result: CertImportResult = await api.importCerts(resolvedCerts);

      // Mark successful imports
      const failedSet = new Set(result.errors.map(e => e.certNumber));
      for (const cn of resolvedCerts) {
        if (failedSet.has(cn)) {
          const errMsg = result.errors.find(e => e.certNumber === cn)?.error ?? 'Import failed';
          updateCert(cn, { status: 'failed', error: errMsg });
        } else {
          updateCert(cn, { status: 'imported' });
        }
      }
    } catch (err) {
      setImportError(err instanceof Error ? err.message : 'Import failed');
      // Revert to resolved
      for (const cn of resolvedCerts) {
        updateCert(cn, { status: 'resolved' });
      }
    } finally {
      setImportLoading(false);
      inputRef.current?.focus();
    }
  };

  // Compute stats
  const rows = Array.from(certs.values());
  const stats = {
    existing: rows.filter(r => r.status === 'existing' || r.status === 'returned' || r.status === 'imported').length,
    sold: rows.filter(r => r.status === 'sold').length,
    newCerts: rows.filter(r => r.status === 'resolving' || r.status === 'resolved' || r.status === 'importing').length,
    failed: rows.filter(r => r.status === 'failed').length,
    total: rows.length,
  };

  const resolvedCount = rows.filter(r => r.status === 'resolved').length;

  // Display rows newest-first
  const displayRows = [...rows].reverse();

  return (
    <div className="space-y-3">
      {/* Input */}
      <div className="flex items-center gap-2">
        <input
          ref={inputRef}
          type="text"
          value={input}
          onChange={e => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Scan or type cert number..."
          className="flex-1 rounded border-2 border-blue-500 bg-gray-900 px-3 py-2.5 font-mono text-base text-gray-100 placeholder-gray-500 focus:border-blue-400 focus:outline-none"
          autoFocus
        />
        <span className="text-xs text-gray-500 whitespace-nowrap">↵ Enter</span>
      </div>

      {/* Stats bar */}
      {stats.total > 0 && (
        <div className="flex flex-wrap gap-4 rounded bg-gray-800 px-3 py-2 text-xs">
          <span><span className="text-green-400">●</span> <span className="text-gray-400">{stats.existing} in inventory</span></span>
          <span><span className="text-amber-400">●</span> <span className="text-gray-400">{stats.sold} sold</span></span>
          <span><span className="text-blue-400">●</span> <span className="text-gray-400">{stats.newCerts} new</span></span>
          <span><span className="text-red-400">●</span> <span className="text-gray-400">{stats.failed} failed</span></span>
          <span className="ml-auto text-gray-500">{stats.total} scanned</span>
        </div>
      )}

      {/* Cert rows */}
      {displayRows.length > 0 && (
        <div className="flex flex-col gap-1">
          {displayRows.map(row => (
            <CertRowItem
              key={row.certNumber}
              row={row}
              onReturn={handleReturnToInventory}
              onDismiss={handleDismiss}
            />
          ))}
        </div>
      )}

      {/* Import error */}
      {importError && (
        <div className="rounded border border-red-700 bg-red-900/30 p-3 text-sm text-red-300">
          {importError}
        </div>
      )}

      {/* Staging area for new certs */}
      {resolvedCount > 0 && (
        <div className="rounded border border-dashed border-blue-500 bg-blue-900/10 p-3">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-blue-300">
              {resolvedCount} NEW CERT{resolvedCount > 1 ? 'S' : ''} STAGED — Review before importing
            </span>
            <button
              onClick={handleImportNew}
              disabled={importLoading}
              className="rounded bg-blue-600 px-4 py-1.5 text-xs font-medium text-white hover:bg-blue-500 disabled:opacity-50"
            >
              {importLoading ? 'Importing...' : `Import ${resolvedCount} New Cert${resolvedCount > 1 ? 's' : ''}`}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

function CertRowItem({ row, onReturn, onDismiss }: {
  row: CertRow;
  onReturn: (certNumber: string) => void;
  onDismiss: (certNumber: string) => void;
}) {
  const base = 'flex items-center justify-between rounded border px-3 py-2 text-sm transition-all';

  const statusConfig: Record<CertStatus, { bg: string; border: string; certColor: string; label: string; icon: string }> = {
    scanning:  { bg: 'bg-gray-800',     border: 'border-gray-600', certColor: 'text-gray-400', label: 'Checking...',  icon: '⟳' },
    existing:  { bg: 'bg-emerald-950/30', border: 'border-emerald-800', certColor: 'text-emerald-300', label: '✓ In inventory', icon: '' },
    sold:      { bg: 'bg-amber-950/30', border: 'border-amber-700', certColor: 'text-amber-300', label: '⚠ Sold',     icon: '' },
    returned:  { bg: 'bg-emerald-950/30', border: 'border-emerald-800', certColor: 'text-emerald-300', label: '✓ Returned',   icon: '' },
    resolving: { bg: 'bg-blue-950/30',  border: 'border-blue-800', certColor: 'text-blue-300', label: '⟳ Looking up...', icon: '' },
    resolved:  { bg: 'bg-blue-950/30',  border: 'border-blue-800', certColor: 'text-blue-300', label: '★ New',       icon: '' },
    failed:    { bg: 'bg-red-950/30',   border: 'border-red-800',  certColor: 'text-red-300',  label: '✗ Failed',    icon: '' },
    importing: { bg: 'bg-blue-950/30',  border: 'border-blue-800', certColor: 'text-blue-300', label: '⟳ Importing...', icon: '' },
    imported:  { bg: 'bg-emerald-950/30', border: 'border-emerald-800', certColor: 'text-emerald-300', label: '✓ Imported',   icon: '' },
  };

  const cfg = statusConfig[row.status];

  return (
    <div id={`cert-row-${row.certNumber}`} className={`${base} ${cfg.bg} ${cfg.border}`}>
      <div className="flex items-center gap-2 min-w-0">
        <span className={`font-mono ${cfg.certColor} min-w-[80px]`}>{row.certNumber}</span>
        <span className={`${cfg.certColor} min-w-[110px] text-xs`}>{cfg.label}</span>
        {row.cardName && <span className="text-gray-400 text-xs truncate">{row.cardName}</span>}
        {row.error && row.status === 'failed' && (
          <span className="text-red-400 text-xs truncate">{row.error}</span>
        )}
      </div>
      <div className="flex items-center gap-2 shrink-0">
        {row.status === 'sold' && (
          <button
            onClick={() => onReturn(row.certNumber)}
            className="rounded bg-amber-600 px-3 py-1 text-xs font-medium text-white hover:bg-amber-500"
          >
            Return to Inventory
          </button>
        )}
        {row.status === 'failed' && (
          <button
            onClick={() => onDismiss(row.certNumber)}
            className="rounded border border-gray-600 px-2 py-1 text-xs text-gray-400 hover:bg-gray-700"
          >
            ✕
          </button>
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: no type errors.

- [ ] **Step 3: Verify ESLint passes**

Run: `cd /workspace/web && npx eslint src/react/pages/tools/CertEntryTab.tsx`
Expected: no lint errors (or only pre-existing warnings).

- [ ] **Step 4: Commit**

```bash
git add web/src/react/pages/tools/CertEntryTab.tsx
git commit -m "feat(cert-import): rewrite CertEntryTab as streaming scanner"
```

---

### Task 8: Run full quality checks

**Files:** none (verification only)

- [ ] **Step 1: Run Go tests with race detection**

Run: `cd /workspace && go test -race ./...`
Expected: all tests pass.

- [ ] **Step 2: Run frontend type check**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 3: Run frontend lint**

Run: `cd /workspace/web && npx eslint src/`
Expected: no new errors.

- [ ] **Step 4: Run make check**

Run: `cd /workspace && make check`
Expected: passes (lint + architecture import check + file size check).

- [ ] **Step 5: Commit any fixes if needed**

If any quality checks surface issues, fix them and commit:

```bash
git add -A
git commit -m "fix(cert-import): address quality check issues"
```
