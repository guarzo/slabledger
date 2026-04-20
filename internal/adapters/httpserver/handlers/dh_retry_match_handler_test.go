package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- Local test doubles ----

type mockDHCertResolver struct {
	ResolveFn func(ctx context.Context, req dh.CertResolveRequest) (*dh.CertResolution, error)
}

func (m *mockDHCertResolver) ResolveCert(ctx context.Context, req dh.CertResolveRequest) (*dh.CertResolution, error) {
	if m.ResolveFn != nil {
		return m.ResolveFn(ctx, req)
	}
	return &dh.CertResolution{Status: dh.CertStatusNotFound}, nil
}

type mockDHPSAImporter struct {
	ImportFn func(ctx context.Context, items []dh.PSAImportItem) (*dh.PSAImportResponse, error)
}

func (m *mockDHPSAImporter) PSAImport(ctx context.Context, items []dh.PSAImportItem) (*dh.PSAImportResponse, error) {
	if m.ImportFn != nil {
		return m.ImportFn(ctx, items)
	}
	return &dh.PSAImportResponse{
		Results: []dh.PSAImportResult{
			{Resolution: dh.PSAImportStatusMatched, DHCardID: 999, DHInventoryID: 888},
		},
	}, nil
}

func retryMatchHandler(repo *mocks.PurchaseRepositoryMock, psaImporter DHPSAImporter, certResolver DHCertResolver) *DHHandler {
	return NewDHHandler(DHHandlerDeps{
		PurchaseLister:    repo,
		PushStatusUpdater: repo,
		DHFieldsUpdater:   repo,
		CandidatesSaver:   repo,
		CardIDSaver:       &mockDHCardIDSaver{},
		InventoryPusher:   &mockDHInventoryPusher{},
		PSAImporter:       psaImporter,
		CertResolver:      certResolver,
		Logger:            mocks.NewMockLogger(),
		BaseCtx:           context.Background(),
	})
}

func postRetryMatch(h *DHHandler, purchaseID string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(retryMatchRequest{PurchaseID: purchaseID})
	req := httptest.NewRequest(http.MethodPost, "/api/dh/retry-match", bytes.NewReader(body))
	req = authenticatedRequest(req)
	rr := httptest.NewRecorder()
	h.HandleRetryMatch(rr, req)
	return rr
}

// ---- Tests ----

func TestHandleRetryMatch_MissingAuth(t *testing.T) {
	h := NewDHHandler(DHHandlerDeps{Logger: mocks.NewMockLogger(), BaseCtx: context.Background()})
	body, _ := json.Marshal(retryMatchRequest{PurchaseID: "p1"})
	req := httptest.NewRequest(http.MethodPost, "/api/dh/retry-match", bytes.NewReader(body))
	// no auth
	rr := httptest.NewRecorder()
	h.HandleRetryMatch(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHandleRetryMatch_MissingPurchaseID(t *testing.T) {
	h := NewDHHandler(DHHandlerDeps{Logger: mocks.NewMockLogger(), BaseCtx: context.Background()})
	body, _ := json.Marshal(retryMatchRequest{PurchaseID: ""})
	req := httptest.NewRequest(http.MethodPost, "/api/dh/retry-match", bytes.NewReader(body))
	req = authenticatedRequest(req)
	rr := httptest.NewRecorder()
	h.HandleRetryMatch(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "purchaseId is required")
}

func TestHandleRetryMatch_PurchaseNotFound(t *testing.T) {
	repo := &mocks.PurchaseRepositoryMock{
		GetPurchaseFn: func(_ context.Context, id string) (*inventory.Purchase, error) {
			return nil, inventory.ErrPurchaseNotFound
		},
	}
	h := retryMatchHandler(repo, nil, nil)
	rr := postRetryMatch(h, "missing-id")
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandleRetryMatch_PurchaseNotUnmatched(t *testing.T) {
	purchase := &inventory.Purchase{
		ID:           "p1",
		DHPushStatus: inventory.DHPushStatusMatched, // already matched
	}
	repo := &mocks.PurchaseRepositoryMock{
		GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
			return purchase, nil
		},
	}
	h := retryMatchHandler(repo, nil, nil)
	rr := postRetryMatch(h, "p1")
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "not in unmatched status")
}

func TestHandleRetryMatch_ResolveCertMatched(t *testing.T) {
	purchase := &inventory.Purchase{
		ID:           "p1",
		CertNumber:   "12345678",
		CardName:     "Charizard",
		SetName:      "Base Set",
		CardNumber:   "4",
		GradeValue:   9,
		BuyCostCents: 10000,
		DHPushStatus: inventory.DHPushStatusUnmatched,
	}
	repo := &mocks.PurchaseRepositoryMock{
		GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
			return purchase, nil
		},
		UpdatePurchaseDHPushStatusFn: func(_ context.Context, _ string, _ string) error { return nil },
		UpdatePurchaseDHFieldsFn:     func(_ context.Context, _ string, _ inventory.DHFieldsUpdate) error { return nil },
		UpdatePurchaseDHCandidatesFn: func(_ context.Context, _ string, _ string) error { return nil },
	}
	certResolver := &mockDHCertResolver{
		ResolveFn: func(_ context.Context, req dh.CertResolveRequest) (*dh.CertResolution, error) {
			return &dh.CertResolution{Status: dh.CertStatusMatched, DHCardID: 555}, nil
		},
	}
	h := retryMatchHandler(repo, nil, certResolver)
	rr := postRetryMatch(h, "p1")
	require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())
	var resp retryMatchResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, "ok", resp.Status)
	assert.Equal(t, 555, resp.DHCardID)
}

func TestHandleRetryMatch_PSAImportMatched(t *testing.T) {
	purchase := &inventory.Purchase{
		ID:           "p2",
		CertNumber:   "99887766",
		CardName:     "Blastoise",
		SetName:      "Base Set",
		CardNumber:   "2",
		GradeValue:   8,
		BuyCostCents: 5000,
		DHPushStatus: inventory.DHPushStatusUnmatched,
	}
	repo := &mocks.PurchaseRepositoryMock{
		GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
			return purchase, nil
		},
		UpdatePurchaseDHPushStatusFn: func(_ context.Context, _ string, _ string) error { return nil },
		UpdatePurchaseDHFieldsFn:     func(_ context.Context, _ string, _ inventory.DHFieldsUpdate) error { return nil },
		UpdatePurchaseDHCandidatesFn: func(_ context.Context, _ string, _ string) error { return nil },
	}
	certResolver := &mockDHCertResolver{
		ResolveFn: func(_ context.Context, _ dh.CertResolveRequest) (*dh.CertResolution, error) {
			return &dh.CertResolution{Status: dh.CertStatusNotFound}, nil
		},
	}
	psaImporter := &mockDHPSAImporter{
		ImportFn: func(_ context.Context, _ []dh.PSAImportItem) (*dh.PSAImportResponse, error) {
			return &dh.PSAImportResponse{
				Results: []dh.PSAImportResult{
					{Resolution: dh.PSAImportStatusMatched, DHCardID: 777, DHInventoryID: 666},
				},
			}, nil
		},
	}
	h := retryMatchHandler(repo, psaImporter, certResolver)
	rr := postRetryMatch(h, "p2")
	require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())
	var resp retryMatchResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, "ok", resp.Status)
	assert.Equal(t, 777, resp.DHCardID)
	assert.Equal(t, 666, resp.DHInventoryID)
}

func TestHandleRetryMatch_PSAImportUnmatchedCreated(t *testing.T) {
	purchase := &inventory.Purchase{
		ID:           "p3",
		CertNumber:   "11223344",
		CardName:     "Venusaur",
		SetName:      "Base Set",
		CardNumber:   "15",
		GradeValue:   7,
		BuyCostCents: 3000,
		DHPushStatus: inventory.DHPushStatusUnmatched,
	}
	repo := &mocks.PurchaseRepositoryMock{
		GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
			return purchase, nil
		},
		UpdatePurchaseDHPushStatusFn: func(_ context.Context, _ string, _ string) error { return nil },
		UpdatePurchaseDHFieldsFn:     func(_ context.Context, _ string, _ inventory.DHFieldsUpdate) error { return nil },
		UpdatePurchaseDHCandidatesFn: func(_ context.Context, _ string, _ string) error { return nil },
	}
	certResolver := &mockDHCertResolver{}
	psaImporter := &mockDHPSAImporter{
		ImportFn: func(_ context.Context, _ []dh.PSAImportItem) (*dh.PSAImportResponse, error) {
			return &dh.PSAImportResponse{
				Results: []dh.PSAImportResult{
					{Resolution: dh.PSAImportStatusUnmatchedCreated, DHCardID: 888, DHInventoryID: 999},
				},
			}, nil
		},
	}
	h := retryMatchHandler(repo, psaImporter, certResolver)
	rr := postRetryMatch(h, "p3")
	require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())
	var resp retryMatchResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, "ok", resp.Status)
	assert.Equal(t, 888, resp.DHCardID)
}

func TestHandleRetryMatch_PSAImportPartnerCardError(t *testing.T) {
	purchase := &inventory.Purchase{
		ID:           "p4",
		CertNumber:   "55667788",
		DHPushStatus: inventory.DHPushStatusUnmatched,
	}
	repo := &mocks.PurchaseRepositoryMock{
		GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
			return purchase, nil
		},
	}
	certResolver := &mockDHCertResolver{}
	psaImporter := &mockDHPSAImporter{
		ImportFn: func(_ context.Context, _ []dh.PSAImportItem) (*dh.PSAImportResponse, error) {
			return &dh.PSAImportResponse{
				Results: []dh.PSAImportResult{
					{Resolution: dh.PSAImportStatusPartnerCardError, Error: "invalid override"},
				},
			}, nil
		},
	}
	h := retryMatchHandler(repo, psaImporter, certResolver)
	rr := postRetryMatch(h, "p4")
	assert.Equal(t, http.StatusUnprocessableEntity, rr.Code)
	assert.Contains(t, rr.Body.String(), "partner_card_error")
}

func TestHandleRetryMatch_PSAImportNil(t *testing.T) {
	purchase := &inventory.Purchase{
		ID:           "p5",
		CertNumber:   "12121212",
		DHPushStatus: inventory.DHPushStatusUnmatched,
	}
	repo := &mocks.PurchaseRepositoryMock{
		GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
			return purchase, nil
		},
	}
	certResolver := &mockDHCertResolver{}
	h := retryMatchHandler(repo, nil, certResolver) // psaImporter = nil
	rr := postRetryMatch(h, "p5")
	assert.Equal(t, http.StatusUnprocessableEntity, rr.Code)
	assert.Contains(t, rr.Body.String(), "PSA import not available")
}

func TestHandleRetryMatch_PSAImportAPIError(t *testing.T) {
	purchase := &inventory.Purchase{
		ID:           "p6",
		CertNumber:   "34343434",
		DHPushStatus: inventory.DHPushStatusUnmatched,
	}
	repo := &mocks.PurchaseRepositoryMock{
		GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
			return purchase, nil
		},
	}
	certResolver := &mockDHCertResolver{}
	psaImporter := &mockDHPSAImporter{
		ImportFn: func(_ context.Context, _ []dh.PSAImportItem) (*dh.PSAImportResponse, error) {
			return nil, errors.New("DH API timeout")
		},
	}
	h := retryMatchHandler(repo, psaImporter, certResolver)
	rr := postRetryMatch(h, "p6")
	assert.Equal(t, http.StatusBadGateway, rr.Code)
}

func TestHandleRetryMatch_PSAImportEmptyResults(t *testing.T) {
	purchase := &inventory.Purchase{
		ID:           "p7",
		CertNumber:   "56565656",
		DHPushStatus: inventory.DHPushStatusUnmatched,
	}
	repo := &mocks.PurchaseRepositoryMock{
		GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
			return purchase, nil
		},
	}
	certResolver := &mockDHCertResolver{}
	psaImporter := &mockDHPSAImporter{
		ImportFn: func(_ context.Context, _ []dh.PSAImportItem) (*dh.PSAImportResponse, error) {
			return &dh.PSAImportResponse{Results: []dh.PSAImportResult{}}, nil
		},
	}
	h := retryMatchHandler(repo, psaImporter, certResolver)
	rr := postRetryMatch(h, "p7")
	assert.Equal(t, http.StatusUnprocessableEntity, rr.Code)
	assert.Contains(t, rr.Body.String(), "no results from DH")
}

func TestHandleRetryMatch_ResolveCertAmbiguousWithCandidates(t *testing.T) {
	purchase := &inventory.Purchase{
		ID:           "p8",
		CertNumber:   "78787878",
		DHPushStatus: inventory.DHPushStatusUnmatched,
	}
	var savedCandidates string
	repo := &mocks.PurchaseRepositoryMock{
		GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
			return purchase, nil
		},
		UpdatePurchaseDHCandidatesFn: func(_ context.Context, _ string, candidates string) error {
			savedCandidates = candidates
			return nil
		},
	}
	certResolver := &mockDHCertResolver{
		ResolveFn: func(_ context.Context, _ dh.CertResolveRequest) (*dh.CertResolution, error) {
			return &dh.CertResolution{
				Status:     dh.CertStatusAmbiguous,
				Candidates: []dh.CertResolutionCandidate{{DHCardID: 111}, {DHCardID: 222}},
			}, nil
		},
	}
	h := retryMatchHandler(repo, nil, certResolver)
	rr := postRetryMatch(h, "p8")
	assert.Equal(t, http.StatusUnprocessableEntity, rr.Code)
	assert.Contains(t, rr.Body.String(), "ambiguous")
	assert.NotEmpty(t, savedCandidates, "candidates should be saved")
}
