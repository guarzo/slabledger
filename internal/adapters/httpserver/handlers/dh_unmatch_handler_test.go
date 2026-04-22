package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time check that the shared mocks satisfy the handler interfaces.
var (
	_ DHMappingDeleter   = (*mocks.DHMappingDeleterMock)(nil)
	_ DHInventoryDeleter = (*mocks.DHInventoryDeleterMock)(nil)
)

func postUnmatchDH(h *DHHandler, purchaseID string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"purchaseId": purchaseID})
	req := httptest.NewRequest(http.MethodPost, "/api/dh/unmatch", bytes.NewReader(body))
	req = authenticatedRequest(req)
	rr := httptest.NewRecorder()
	h.HandleUnmatchDH(rr, req)
	return rr
}

func TestHandleUnmatchDH(t *testing.T) {
	receivedAt := "2026-04-22T10:00:00Z"

	type assertions struct {
		updatedFields     *inventory.DHFieldsUpdate
		updatedStatus     *string
		clearedCandidates *string
		mappingDeleter    *mocks.DHMappingDeleterMock
		inventoryDeleter  *mocks.DHInventoryDeleterMock
	}

	cases := []struct {
		name         string
		purchaseID   string
		requestAuth  bool
		repo         func(a *assertions) *mocks.PurchaseRepositoryMock
		deleteErr    error // injected into DHInventoryDeleterMock.DeleteInventoryFn
		mappingErr   error // injected into DHMappingDeleterMock.DeleteAutoMappingFn
		expectedCode int
		expectedBody string
		check        func(t *testing.T, a assertions)
	}{
		{
			name:        "MissingAuth",
			purchaseID:  "p1",
			requestAuth: false,
			repo: func(_ *assertions) *mocks.PurchaseRepositoryMock {
				return &mocks.PurchaseRepositoryMock{}
			},
			expectedCode: http.StatusUnauthorized,
		},
		{
			name:        "MissingPurchaseID",
			purchaseID:  "",
			requestAuth: true,
			repo: func(_ *assertions) *mocks.PurchaseRepositoryMock {
				return &mocks.PurchaseRepositoryMock{}
			},
			expectedCode: http.StatusBadRequest,
			expectedBody: "purchaseId is required",
		},
		{
			name:        "PurchaseNotFound",
			purchaseID:  "missing-id",
			requestAuth: true,
			repo: func(_ *assertions) *mocks.PurchaseRepositoryMock {
				return &mocks.PurchaseRepositoryMock{
					GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
						return nil, inventory.ErrPurchaseNotFound
					},
				}
			},
			expectedCode: http.StatusNotFound,
		},
		{
			name:        "InvalidState_NotMatched",
			purchaseID:  "p1",
			requestAuth: true,
			repo: func(_ *assertions) *mocks.PurchaseRepositoryMock {
				return &mocks.PurchaseRepositoryMock{
					GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
						return &inventory.Purchase{
							ID:           "p1",
							DHPushStatus: inventory.DHPushStatusUnmatched,
						}, nil
					},
				}
			},
			expectedCode: http.StatusConflict,
			expectedBody: "invalid purchase state for unmatch",
		},
		{
			name:        "UpdateDHFieldsFails_NoExternalMutation",
			purchaseID:  "p1",
			requestAuth: true,
			repo: func(a *assertions) *mocks.PurchaseRepositoryMock {
				return &mocks.PurchaseRepositoryMock{
					GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
						return &inventory.Purchase{
							ID:            "p1",
							DHPushStatus:  inventory.DHPushStatusMatched,
							DHInventoryID: 666,
						}, nil
					},
					UpdatePurchaseDHFieldsFn: func(_ context.Context, _ string, u inventory.DHFieldsUpdate) error {
						a.updatedFields = &u
						return errors.New("db error")
					},
					UpdatePurchaseDHPushStatusFn: func(_ context.Context, _ string, status string) error {
						a.updatedStatus = &status
						return nil
					},
				}
			},
			expectedCode: http.StatusInternalServerError,
			check: func(t *testing.T, a assertions) {
				t.Helper()
				require.NotNil(t, a.updatedFields)
				assert.Nil(t, a.updatedStatus, "UpdatePurchaseDHPushStatus should not be called when UpdatePurchaseDHFields fails")
				assert.False(t, a.inventoryDeleter.Called, "delete inventory must not run when local DB update failed")
				assert.False(t, a.mappingDeleter.Called, "mapping delete must not run when local DB update failed")
			},
		},
		{
			name:        "UpdatePushStatusFails_NoExternalMutation",
			purchaseID:  "p1",
			requestAuth: true,
			repo: func(a *assertions) *mocks.PurchaseRepositoryMock {
				return &mocks.PurchaseRepositoryMock{
					GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
						return &inventory.Purchase{
							ID:            "p1",
							DHPushStatus:  inventory.DHPushStatusMatched,
							DHInventoryID: 666,
						}, nil
					},
					UpdatePurchaseDHFieldsFn: func(_ context.Context, _ string, u inventory.DHFieldsUpdate) error {
						a.updatedFields = &u
						return nil
					},
					UpdatePurchaseDHPushStatusFn: func(_ context.Context, _ string, status string) error {
						a.updatedStatus = &status
						return errors.New("db error")
					},
					UpdatePurchaseDHCandidatesFn: func(_ context.Context, _ string, c string) error {
						a.clearedCandidates = &c
						return nil
					},
				}
			},
			expectedCode: http.StatusInternalServerError,
			check: func(t *testing.T, a assertions) {
				t.Helper()
				require.NotNil(t, a.updatedFields)
				require.NotNil(t, a.updatedStatus)
				assert.Nil(t, a.clearedCandidates, "UpdatePurchaseDHCandidates should not be called when UpdatePurchaseDHPushStatus fails")
				assert.False(t, a.inventoryDeleter.Called, "delete inventory must not run when local DB update failed")
				assert.False(t, a.mappingDeleter.Called, "mapping delete must not run when local DB update failed")
			},
		},
		{
			// ReceivedAt is nil → not yet received → status stays "unmatched"
			name:        "Success_NotReceived_SetsUnmatched",
			purchaseID:  "p1",
			requestAuth: true,
			repo: func(a *assertions) *mocks.PurchaseRepositoryMock {
				return &mocks.PurchaseRepositoryMock{
					GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
						return &inventory.Purchase{
							ID:            "p1",
							CardName:      "PIKACHU 1ST EDITION",
							SetName:       "SPANISH",
							CardNumber:    "58",
							DHPushStatus:  inventory.DHPushStatusMatched,
							DHCardID:      555,
							DHInventoryID: 666,
							ReceivedAt:    nil,
						}, nil
					},
					UpdatePurchaseDHFieldsFn: func(_ context.Context, _ string, u inventory.DHFieldsUpdate) error {
						a.updatedFields = &u
						return nil
					},
					UpdatePurchaseDHPushStatusFn: func(_ context.Context, _ string, status string) error {
						a.updatedStatus = &status
						return nil
					},
					UpdatePurchaseDHCandidatesFn: func(_ context.Context, _ string, c string) error {
						a.clearedCandidates = &c
						return nil
					},
				}
			},
			expectedCode: http.StatusOK,
			check: func(t *testing.T, a assertions) {
				t.Helper()
				require.NotNil(t, a.updatedFields)
				assert.Equal(t, 0, a.updatedFields.CardID)
				assert.Equal(t, 0, a.updatedFields.InventoryID)
				require.NotNil(t, a.updatedStatus)
				assert.Equal(t, inventory.DHPushStatusUnmatched, *a.updatedStatus)
				require.NotNil(t, a.clearedCandidates)
				assert.Equal(t, "", *a.clearedCandidates)
				assert.True(t, a.inventoryDeleter.Called, "DeleteInventory should be called when inventory ID is set")
				assert.Equal(t, 666, a.inventoryDeleter.InventoryID)
				assert.True(t, a.mappingDeleter.Called, "DeleteAutoMapping should be called")
				assert.Equal(t, "PIKACHU 1ST EDITION", a.mappingDeleter.CardName)
				assert.Equal(t, "SPANISH", a.mappingDeleter.SetName)
				assert.Equal(t, "58", a.mappingDeleter.CollectorNumber)
				assert.Equal(t, pricing.SourceDH, a.mappingDeleter.Provider)
			},
		},
		{
			// ReceivedAt is set → card in hand → status becomes "pending" so scheduler retries immediately
			name:        "Success_Received_SetsPending",
			purchaseID:  "p1",
			requestAuth: true,
			repo: func(a *assertions) *mocks.PurchaseRepositoryMock {
				return &mocks.PurchaseRepositoryMock{
					GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
						return &inventory.Purchase{
							ID:            "p1",
							CardName:      "PIKACHU 1ST EDITION",
							SetName:       "SPANISH",
							CardNumber:    "58",
							DHPushStatus:  inventory.DHPushStatusMatched,
							DHCardID:      555,
							DHInventoryID: 666,
							ReceivedAt:    &receivedAt,
						}, nil
					},
					UpdatePurchaseDHFieldsFn: func(_ context.Context, _ string, u inventory.DHFieldsUpdate) error {
						a.updatedFields = &u
						return nil
					},
					UpdatePurchaseDHPushStatusFn: func(_ context.Context, _ string, status string) error {
						a.updatedStatus = &status
						return nil
					},
					UpdatePurchaseDHCandidatesFn: func(_ context.Context, _ string, c string) error {
						a.clearedCandidates = &c
						return nil
					},
				}
			},
			expectedCode: http.StatusOK,
			check: func(t *testing.T, a assertions) {
				t.Helper()
				require.NotNil(t, a.updatedStatus)
				assert.Equal(t, inventory.DHPushStatusPending, *a.updatedStatus, "received items should go to pending for immediate retry")
				assert.True(t, a.inventoryDeleter.Called)
				assert.Equal(t, 666, a.inventoryDeleter.InventoryID)
			},
		},
		{
			name:        "Success_NoInventoryID_SkipsDelete",
			purchaseID:  "p1",
			requestAuth: true,
			repo: func(a *assertions) *mocks.PurchaseRepositoryMock {
				return &mocks.PurchaseRepositoryMock{
					GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
						return &inventory.Purchase{
							ID:            "p1",
							CardName:      "FOO",
							SetName:       "BAR",
							CardNumber:    "1",
							DHPushStatus:  inventory.DHPushStatusMatched,
							DHCardID:      555,
							DHInventoryID: 0,
						}, nil
					},
					UpdatePurchaseDHFieldsFn: func(_ context.Context, _ string, u inventory.DHFieldsUpdate) error {
						a.updatedFields = &u
						return nil
					},
					UpdatePurchaseDHPushStatusFn: func(_ context.Context, _ string, status string) error {
						a.updatedStatus = &status
						return nil
					},
					UpdatePurchaseDHCandidatesFn: func(_ context.Context, _ string, c string) error {
						a.clearedCandidates = &c
						return nil
					},
				}
			},
			expectedCode: http.StatusOK,
			check: func(t *testing.T, a assertions) {
				t.Helper()
				assert.False(t, a.inventoryDeleter.Called, "DeleteInventory should be skipped when inventory ID is 0")
				assert.True(t, a.mappingDeleter.Called, "DeleteAutoMapping should still be called")
			},
		},
		{
			name:        "Success_DeleteInventoryFails_ContinuesUnmatch",
			purchaseID:  "p1",
			requestAuth: true,
			repo: func(a *assertions) *mocks.PurchaseRepositoryMock {
				return &mocks.PurchaseRepositoryMock{
					GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
						return &inventory.Purchase{
							ID:            "p1",
							CardName:      "FOO",
							SetName:       "BAR",
							CardNumber:    "1",
							DHPushStatus:  inventory.DHPushStatusMatched,
							DHCardID:      555,
							DHInventoryID: 666,
						}, nil
					},
					UpdatePurchaseDHFieldsFn: func(_ context.Context, _ string, u inventory.DHFieldsUpdate) error {
						a.updatedFields = &u
						return nil
					},
					UpdatePurchaseDHPushStatusFn: func(_ context.Context, _ string, status string) error {
						a.updatedStatus = &status
						return nil
					},
					UpdatePurchaseDHCandidatesFn: func(_ context.Context, _ string, c string) error {
						a.clearedCandidates = &c
						return nil
					},
				}
			},
			deleteErr:    errors.New("delete boom"),
			expectedCode: http.StatusOK,
			check: func(t *testing.T, a assertions) {
				t.Helper()
				assert.True(t, a.inventoryDeleter.Called)
				require.NotNil(t, a.updatedStatus, "unmatch must still complete when delete fails")
				assert.Equal(t, inventory.DHPushStatusUnmatched, *a.updatedStatus)
				assert.True(t, a.mappingDeleter.Called, "DeleteAutoMapping should still run when delete fails")
			},
		},
		{
			name:        "Success_MappingDeleteFails_ContinuesUnmatch",
			purchaseID:  "p1",
			requestAuth: true,
			repo: func(a *assertions) *mocks.PurchaseRepositoryMock {
				return &mocks.PurchaseRepositoryMock{
					GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
						return &inventory.Purchase{
							ID:            "p1",
							CardName:      "FOO",
							SetName:       "BAR",
							CardNumber:    "1",
							DHPushStatus:  inventory.DHPushStatusMatched,
							DHCardID:      555,
							DHInventoryID: 666,
						}, nil
					},
					UpdatePurchaseDHFieldsFn: func(_ context.Context, _ string, u inventory.DHFieldsUpdate) error {
						a.updatedFields = &u
						return nil
					},
					UpdatePurchaseDHPushStatusFn: func(_ context.Context, _ string, status string) error {
						a.updatedStatus = &status
						return nil
					},
					UpdatePurchaseDHCandidatesFn: func(_ context.Context, _ string, c string) error {
						a.clearedCandidates = &c
						return nil
					},
				}
			},
			mappingErr:   errors.New("mapping delete boom"),
			expectedCode: http.StatusOK,
			check: func(t *testing.T, a assertions) {
				t.Helper()
				assert.True(t, a.inventoryDeleter.Called)
				assert.True(t, a.mappingDeleter.Called)
				require.NotNil(t, a.updatedStatus, "unmatch must still complete when mapping delete fails")
				assert.Equal(t, inventory.DHPushStatusUnmatched, *a.updatedStatus)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var a assertions
			a.mappingDeleter = &mocks.DHMappingDeleterMock{}
			a.inventoryDeleter = &mocks.DHInventoryDeleterMock{}
			if tc.deleteErr != nil {
				deleteErr := tc.deleteErr
				a.inventoryDeleter.DeleteInventoryFn = func(_ context.Context, _ int) error {
					return deleteErr
				}
			}
			if tc.mappingErr != nil {
				mappingErr := tc.mappingErr
				a.mappingDeleter.DeleteAutoMappingFn = func(_ context.Context, _, _, _, _ string) (int64, error) {
					return 0, mappingErr
				}
			}
			repo := tc.repo(&a)
			h := NewDHHandler(DHHandlerDeps{
				PurchaseLister:    repo,
				DHFieldsUpdater:   repo,
				PushStatusUpdater: repo,
				CandidatesSaver:   repo,
				MappingDeleter:    a.mappingDeleter,
				InventoryDeleter:  a.inventoryDeleter,
				Logger:            mocks.NewMockLogger(),
				BaseCtx:           context.Background(),
			})

			var rr *httptest.ResponseRecorder
			if !tc.requestAuth {
				body, _ := json.Marshal(map[string]string{"purchaseId": tc.purchaseID})
				req := httptest.NewRequest(http.MethodPost, "/api/dh/unmatch", bytes.NewReader(body))
				rr = httptest.NewRecorder()
				h.HandleUnmatchDH(rr, req)
			} else {
				rr = postUnmatchDH(h, tc.purchaseID)
			}

			assert.Equal(t, tc.expectedCode, rr.Code)
			if tc.expectedBody != "" {
				assert.Contains(t, rr.Body.String(), tc.expectedBody)
			}
			if tc.check != nil {
				tc.check(t, a)
			}
		})
	}
}
