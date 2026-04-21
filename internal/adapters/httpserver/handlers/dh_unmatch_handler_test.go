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
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type unmatchMappingDeleter struct {
	called              bool
	cardName, setName   string
	collectorNumber     string
	provider            string
	rows                int64
	err                 error
}

func (m *unmatchMappingDeleter) DeleteAutoMapping(_ context.Context, cardName, setName, collectorNumber, provider string) (int64, error) {
	m.called = true
	m.cardName = cardName
	m.setName = setName
	m.collectorNumber = collectorNumber
	m.provider = provider
	return m.rows, m.err
}

type unmatchChannelDelister struct {
	called      bool
	inventoryID int
	channels    []string
	err         error
}

func (d *unmatchChannelDelister) DelistChannels(_ context.Context, inventoryID int, channels []string) (*dh.ChannelSyncResponse, error) {
	d.called = true
	d.inventoryID = inventoryID
	d.channels = channels
	if d.err != nil {
		return nil, d.err
	}
	return &dh.ChannelSyncResponse{}, nil
}

func postUnmatchDH(h *DHHandler, purchaseID string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"purchaseId": purchaseID})
	req := httptest.NewRequest(http.MethodPost, "/api/dh/unmatch", bytes.NewReader(body))
	req = authenticatedRequest(req)
	rr := httptest.NewRecorder()
	h.HandleUnmatchDH(rr, req)
	return rr
}

func TestHandleUnmatchDH(t *testing.T) {
	type assertions struct {
		updatedFields     *inventory.DHFieldsUpdate
		updatedStatus     *string
		clearedCandidates *string
		mappingDeleter    *unmatchMappingDeleter
		channelDelister   *unmatchChannelDelister
	}

	cases := []struct {
		name         string
		purchaseID   string
		requestAuth  bool
		repo         func(a *assertions) *mocks.PurchaseRepositoryMock
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
			name:        "UpdateDHFieldsFails",
			purchaseID:  "p1",
			requestAuth: true,
			repo: func(a *assertions) *mocks.PurchaseRepositoryMock {
				return &mocks.PurchaseRepositoryMock{
					GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
						return &inventory.Purchase{
							ID:           "p1",
							DHPushStatus: inventory.DHPushStatusMatched,
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
			},
		},
		{
			name:        "UpdatePushStatusFails",
			purchaseID:  "p1",
			requestAuth: true,
			repo: func(a *assertions) *mocks.PurchaseRepositoryMock {
				return &mocks.PurchaseRepositoryMock{
					GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
						return &inventory.Purchase{
							ID:           "p1",
							DHPushStatus: inventory.DHPushStatusMatched,
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
			},
		},
		{
			name:        "Success",
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
				require.NotNil(t, a.channelDelister)
				assert.True(t, a.channelDelister.called, "DelistChannels should be called when inventory ID is set")
				assert.Equal(t, 666, a.channelDelister.inventoryID)
				assert.Nil(t, a.channelDelister.channels, "empty channels = delist from all")
				require.NotNil(t, a.mappingDeleter)
				assert.True(t, a.mappingDeleter.called, "DeleteAutoMapping should be called")
				assert.Equal(t, "PIKACHU 1ST EDITION", a.mappingDeleter.cardName)
				assert.Equal(t, "SPANISH", a.mappingDeleter.setName)
				assert.Equal(t, "58", a.mappingDeleter.collectorNumber)
				assert.Equal(t, pricing.SourceDH, a.mappingDeleter.provider)
			},
		},
		{
			name:        "Success_NoInventoryID_SkipsDelist",
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
				require.NotNil(t, a.channelDelister)
				assert.False(t, a.channelDelister.called, "DelistChannels should be skipped when inventory ID is 0")
				require.NotNil(t, a.mappingDeleter)
				assert.True(t, a.mappingDeleter.called, "DeleteAutoMapping should still be called")
			},
		},
		{
			name:        "Success_DelistFails_ContinuesUnmatch",
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
			expectedCode: http.StatusOK,
			check: func(t *testing.T, a assertions) {
				t.Helper()
				require.NotNil(t, a.channelDelister)
				assert.True(t, a.channelDelister.called)
				require.NotNil(t, a.updatedStatus, "unmatch must still complete when delist fails")
				assert.Equal(t, inventory.DHPushStatusUnmatched, *a.updatedStatus)
				require.NotNil(t, a.mappingDeleter)
				assert.True(t, a.mappingDeleter.called, "DeleteAutoMapping should still run when delist fails")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var a assertions
			a.mappingDeleter = &unmatchMappingDeleter{}
			a.channelDelister = &unmatchChannelDelister{}
			if tc.name == "Success_DelistFails_ContinuesUnmatch" {
				a.channelDelister.err = errors.New("delist boom")
			}
			repo := tc.repo(&a)
			h := NewDHHandler(DHHandlerDeps{
				PurchaseLister:    repo,
				DHFieldsUpdater:   repo,
				PushStatusUpdater: repo,
				CandidatesSaver:   repo,
				MappingDeleter:    a.mappingDeleter,
				ChannelDelister:   a.channelDelister,
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
