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

func TestHandleRetryMatch(t *testing.T) {
	unmatchedPurchase := func(id, cert string) *inventory.Purchase {
		return &inventory.Purchase{
			ID:           id,
			CertNumber:   cert,
			CardName:     "Charizard",
			SetName:      "Base Set",
			CardNumber:   "4",
			GradeValue:   9,
			BuyCostCents: 10000,
			DHPushStatus: inventory.DHPushStatusUnmatched,
		}
	}

	matchedRepo := func(p *inventory.Purchase) *mocks.PurchaseRepositoryMock {
		return &mocks.PurchaseRepositoryMock{
			GetPurchaseFn:                func(_ context.Context, _ string) (*inventory.Purchase, error) { return p, nil },
			UpdatePurchaseDHPushStatusFn: func(_ context.Context, _ string, _ string) error { return nil },
			UpdatePurchaseDHFieldsFn:     func(_ context.Context, _ string, _ inventory.DHFieldsUpdate) error { return nil },
			UpdatePurchaseDHCandidatesFn: func(_ context.Context, _ string, _ string) error { return nil },
		}
	}

	cases := []struct {
		name                 string
		purchaseID           string
		requestAuth          bool
		repo                 func() *mocks.PurchaseRepositoryMock
		certResolver         DHCertResolver
		psaImporter          DHPSAImporter
		expectedCode         int
		expectedBodyContains string
		checkResponse        func(t *testing.T, resp retryMatchResponse)
	}{
		{
			name:        "MissingAuth",
			purchaseID:  "p1",
			requestAuth: false,
			repo: func() *mocks.PurchaseRepositoryMock {
				return &mocks.PurchaseRepositoryMock{}
			},
			expectedCode: http.StatusUnauthorized,
		},
		{
			name:        "MissingPurchaseID",
			purchaseID:  "",
			requestAuth: true,
			repo: func() *mocks.PurchaseRepositoryMock {
				return &mocks.PurchaseRepositoryMock{}
			},
			expectedCode:         http.StatusBadRequest,
			expectedBodyContains: "purchaseId is required",
		},
		{
			name:        "PurchaseNotFound",
			purchaseID:  "missing-id",
			requestAuth: true,
			repo: func() *mocks.PurchaseRepositoryMock {
				return &mocks.PurchaseRepositoryMock{
					GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
						return nil, inventory.ErrPurchaseNotFound
					},
				}
			},
			expectedCode: http.StatusNotFound,
		},
		{
			name:        "PurchaseNotUnmatched",
			purchaseID:  "p1",
			requestAuth: true,
			repo: func() *mocks.PurchaseRepositoryMock {
				return &mocks.PurchaseRepositoryMock{
					GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
						return &inventory.Purchase{ID: "p1", DHPushStatus: inventory.DHPushStatusMatched}, nil
					},
				}
			},
			expectedCode:         http.StatusBadRequest,
			expectedBodyContains: "not in unmatched status",
		},
		{
			name:        "PSAImportMatchedDirectly",
			purchaseID:  "p1",
			requestAuth: true,
			repo: func() *mocks.PurchaseRepositoryMock {
				return matchedRepo(unmatchedPurchase("p1", "12345678"))
			},
			psaImporter: &mockDHPSAImporter{
				ImportFn: func(_ context.Context, items []dh.PSAImportItem) (*dh.PSAImportResponse, error) {
					return &dh.PSAImportResponse{
						Results: []dh.PSAImportResult{
							{Resolution: dh.PSAImportStatusMatched, DHCardID: 555, DHInventoryID: 444},
						},
					}, nil
				},
			},
			expectedCode: http.StatusOK,
			checkResponse: func(t *testing.T, resp retryMatchResponse) {
				t.Helper()
				assert.Equal(t, "ok", resp.Status)
				assert.Equal(t, 555, resp.DHCardID)
				assert.Equal(t, 444, resp.DHInventoryID)
			},
		},
		{
			name:        "PSAImportMatched",
			purchaseID:  "p2",
			requestAuth: true,
			repo: func() *mocks.PurchaseRepositoryMock {
				return matchedRepo(unmatchedPurchase("p2", "99887766"))
			},
			certResolver: &mockDHCertResolver{
				ResolveFn: func(_ context.Context, _ dh.CertResolveRequest) (*dh.CertResolution, error) {
					return &dh.CertResolution{Status: dh.CertStatusNotFound}, nil
				},
			},
			psaImporter: &mockDHPSAImporter{
				ImportFn: func(_ context.Context, _ []dh.PSAImportItem) (*dh.PSAImportResponse, error) {
					return &dh.PSAImportResponse{
						Results: []dh.PSAImportResult{
							{Resolution: dh.PSAImportStatusMatched, DHCardID: 777, DHInventoryID: 666},
						},
					}, nil
				},
			},
			expectedCode: http.StatusOK,
			checkResponse: func(t *testing.T, resp retryMatchResponse) {
				t.Helper()
				assert.Equal(t, "ok", resp.Status)
				assert.Equal(t, 777, resp.DHCardID)
				assert.Equal(t, 666, resp.DHInventoryID)
			},
		},
		{
			name:        "PSAImportUnmatchedCreated",
			purchaseID:  "p3",
			requestAuth: true,
			repo: func() *mocks.PurchaseRepositoryMock {
				return matchedRepo(unmatchedPurchase("p3", "11223344"))
			},
			certResolver: &mockDHCertResolver{},
			psaImporter: &mockDHPSAImporter{
				ImportFn: func(_ context.Context, _ []dh.PSAImportItem) (*dh.PSAImportResponse, error) {
					return &dh.PSAImportResponse{
						Results: []dh.PSAImportResult{
							{Resolution: dh.PSAImportStatusUnmatchedCreated, DHCardID: 888, DHInventoryID: 999},
						},
					}, nil
				},
			},
			expectedCode: http.StatusOK,
			checkResponse: func(t *testing.T, resp retryMatchResponse) {
				t.Helper()
				assert.Equal(t, "ok", resp.Status)
				assert.Equal(t, 888, resp.DHCardID)
			},
		},
		{
			name:        "PSAImportPartnerCardError",
			purchaseID:  "p4",
			requestAuth: true,
			repo: func() *mocks.PurchaseRepositoryMock {
				return &mocks.PurchaseRepositoryMock{
					GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
						return unmatchedPurchase("p4", "55667788"), nil
					},
				}
			},
			certResolver: &mockDHCertResolver{},
			psaImporter: &mockDHPSAImporter{
				ImportFn: func(_ context.Context, _ []dh.PSAImportItem) (*dh.PSAImportResponse, error) {
					return &dh.PSAImportResponse{
						Results: []dh.PSAImportResult{
							{Resolution: dh.PSAImportStatusPartnerCardError, Error: "invalid override"},
						},
					}, nil
				},
			},
			expectedCode:         http.StatusUnprocessableEntity,
			expectedBodyContains: "partner_card_error",
		},
		{
			name:        "PSAImportNil",
			purchaseID:  "p5",
			requestAuth: true,
			repo: func() *mocks.PurchaseRepositoryMock {
				return &mocks.PurchaseRepositoryMock{
					GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
						return unmatchedPurchase("p5", "12121212"), nil
					},
				}
			},
			certResolver:         &mockDHCertResolver{},
			psaImporter:          nil,
			expectedCode:         http.StatusUnprocessableEntity,
			expectedBodyContains: "PSA import not available",
		},
		{
			name:        "PSAImportAPIError",
			purchaseID:  "p6",
			requestAuth: true,
			repo: func() *mocks.PurchaseRepositoryMock {
				return &mocks.PurchaseRepositoryMock{
					GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
						return unmatchedPurchase("p6", "34343434"), nil
					},
				}
			},
			certResolver: &mockDHCertResolver{},
			psaImporter: &mockDHPSAImporter{
				ImportFn: func(_ context.Context, _ []dh.PSAImportItem) (*dh.PSAImportResponse, error) {
					return nil, errors.New("DH API timeout")
				},
			},
			expectedCode: http.StatusBadGateway,
		},
		{
			name:        "PSAImportEmptyResults",
			purchaseID:  "p7",
			requestAuth: true,
			repo: func() *mocks.PurchaseRepositoryMock {
				return &mocks.PurchaseRepositoryMock{
					GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
						return unmatchedPurchase("p7", "56565656"), nil
					},
				}
			},
			certResolver: &mockDHCertResolver{},
			psaImporter: &mockDHPSAImporter{
				ImportFn: func(_ context.Context, _ []dh.PSAImportItem) (*dh.PSAImportResponse, error) {
					return &dh.PSAImportResponse{Results: []dh.PSAImportResult{}}, nil
				},
			},
			expectedCode:         http.StatusUnprocessableEntity,
			expectedBodyContains: "no results from DH",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := tc.repo()
			h := retryMatchHandler(repo, tc.psaImporter, tc.certResolver)

			var rr *httptest.ResponseRecorder
			if !tc.requestAuth {
				body, _ := json.Marshal(retryMatchRequest{PurchaseID: tc.purchaseID})
				req := httptest.NewRequest(http.MethodPost, "/api/dh/retry-match", bytes.NewReader(body))
				rr = httptest.NewRecorder()
				h.HandleRetryMatch(rr, req)
			} else {
				rr = postRetryMatch(h, tc.purchaseID)
			}

			assert.Equal(t, tc.expectedCode, rr.Code)
			if tc.expectedBodyContains != "" {
				assert.Contains(t, rr.Body.String(), tc.expectedBodyContains)
			}
			if tc.checkResponse != nil {
				require.Equal(t, tc.expectedCode, rr.Code, "body: %s", rr.Body.String())
				var resp retryMatchResponse
				require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
				tc.checkResponse(t, resp)
			}
		})
	}
}
