package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixMatchValidationHandler builds a DHHandler with no production deps wired,
// suitable for testing the request-validation paths that short-circuit before
// any service call.
func fixMatchValidationHandler() *DHHandler {
	return NewDHHandler(DHHandlerDeps{
		Logger:  mocks.NewMockLogger(),
		BaseCtx: context.Background(),
	})
}

// TestHandleFixMatch_AuthAndBody covers the early request-validation guards
// that don't touch any service dependency: missing user → 401 and malformed
// JSON body → 400.
func TestHandleFixMatch_AuthAndBody(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		withAuth bool
		wantCode int
	}{
		{
			name:     "no user → 401",
			body:     `{"purchaseId":"p1","dhUrl":"https://doubleholo.com/card/123/foo"}`,
			withAuth: false,
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "malformed JSON body → 400",
			body:     `{not json`,
			withAuth: true,
			wantCode: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := fixMatchValidationHandler()

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/dh/fix-match", strings.NewReader(tt.body))
			if tt.withAuth {
				req = withUser(req)
			}
			h.HandleFixMatch(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("status: got %d, want %d (body=%s)", rec.Code, tt.wantCode, rec.Body.String())
			}
		})
	}
}

func TestHandleFixMatch_RequiredFields(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"missing purchaseId", `{"dhUrl":"https://doubleholo.com/card/123/x"}`, "purchaseId is required"},
		{"empty purchaseId", `{"purchaseId":"","dhUrl":"https://doubleholo.com/card/123/x"}`, "purchaseId is required"},
		{"missing dhUrl", `{"purchaseId":"p1"}`, "dhUrl is required"},
		{"empty dhUrl", `{"purchaseId":"p1","dhUrl":""}`, "dhUrl is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := fixMatchValidationHandler()

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/dh/fix-match", strings.NewReader(tt.body))
			req = withUser(req)
			h.HandleFixMatch(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status: got %d, want 400 (body=%s)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.want) {
				t.Errorf("body %q does not contain %q", rec.Body.String(), tt.want)
			}
		})
	}
}

// TestHandleFixMatch_InvalidURLOrCardID covers all 400-rejection paths driven
// by the dhUrl shape: wrong domain / wrong path / unparseable / a parseable
// URL whose card-id segment is zero. Each case asserts both the status code
// and the specific error-message snippet so a regression that swaps the two
// validation branches still fails the test.
// TestHandleFixMatch_DelistOnCardChange covers the happy path for re-matching
// an already-matched purchase to a different DH card. The prior listing under
// the wrong card must have its channels delisted so eBay/Shopify don't keep
// advertising the wrong card after the swap.
//
// The handler guards delist on three conditions AND'd together:
//
//	oldInventoryID != 0             — there's a prior listing to consider
//	oldInventoryID != newInventoryID — DH created a new row (not re-pointed)
//	oldCardID != newCardID          — the card actually changed
//
// Each case below exercises a different branch of that guard chain.
func TestHandleFixMatch_DelistOnCardChange(t *testing.T) {
	tests := []struct {
		name            string
		oldCardID       int
		oldInventoryID  int
		newCardID       int
		newInventoryID  int
		wantDelistCalls int
	}{
		{
			// All three guards true → delist.
			name:            "card swap creates new inv row → delist old",
			oldCardID:       598,
			oldInventoryID:  880,
			newCardID:       999,
			newInventoryID:  881,
			wantDelistCalls: 1,
		},
		{
			// oldCardID == newCardID guard fails → skip.
			name:            "same card reconfirm → no delist",
			oldCardID:       598,
			oldInventoryID:  880,
			newCardID:       598,
			newInventoryID:  880,
			wantDelistCalls: 0,
		},
		{
			// oldInventoryID == 0 guard fails → skip. First-time match.
			name:            "first-time match (no prior listing) → no delist",
			oldCardID:       0,
			oldInventoryID:  0,
			newCardID:       999,
			newInventoryID:  881,
			wantDelistCalls: 0,
		},
		{
			// oldInventoryID == newInventoryID guard fails even though the card
			// changed. DH re-pointed the existing row at the new card, so the
			// listing that's live is now correctly associated — don't take it down.
			name:            "DH reused inv row across card change → no delist",
			oldCardID:       598,
			oldInventoryID:  880,
			newCardID:       599,
			newInventoryID:  880,
			wantDelistCalls: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			purchase := &inventory.Purchase{
				ID:            "p1",
				CertNumber:    "49396327",
				CardName:      "PIKACHU 1ST EDITION",
				SetName:       "SPANISH",
				CardNumber:    "58",
				GradeValue:    10,
				BuyCostCents:  61420,
				DHCardID:      tt.oldCardID,
				DHInventoryID: tt.oldInventoryID,
				DHPushStatus:  inventory.DHPushStatusMatched,
			}

			repo := &mocks.PurchaseRepositoryMock{
				GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
					return purchase, nil
				},
			}

			pusher := &mocks.DHInventoryPusherMock{
				PushInventoryFn: func(_ context.Context, items []dh.InventoryItem) (*dh.InventoryPushResponse, error) {
					require.Len(t, items, 1)
					assert.Equal(t, tt.newCardID, items[0].DHCardID)
					return &dh.InventoryPushResponse{
						Results: []dh.InventoryResult{
							{DHInventoryID: tt.newInventoryID, Status: "in_stock", AssignedPriceCents: 77700},
						},
					}, nil
				},
			}

			delister := &mocks.DHChannelDelisterMock{}

			h := NewDHHandler(DHHandlerDeps{
				PurchaseLister:    repo,
				PushStatusUpdater: repo,
				DHFieldsUpdater:   repo,
				CandidatesSaver:   repo,
				InventoryPusher:   pusher,
				CardIDSaver:       &mocks.DHCardIDSaverMock{},
				ChannelDelister:   delister,
				Logger:            mocks.NewMockLogger(),
				BaseCtx:           context.Background(),
			})

			body, _ := json.Marshal(fixMatchRequest{
				PurchaseID: "p1",
				DHURL:      "https://doubleholo.com/card/" + strconv.Itoa(tt.newCardID) + "/pikachu",
			})
			req := httptest.NewRequest(http.MethodPost, "/api/dh/fix-match", bytes.NewReader(body))
			req = withUser(req)
			rr := httptest.NewRecorder()
			h.HandleFixMatch(rr, req)

			require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())

			// Delist now runs in a background goroutine dispatched before the
			// response is written. Drain the dispatcher so the assertion sees
			// the completed call state.
			h.Wait()

			if tt.wantDelistCalls == 0 {
				assert.False(t, delister.Called, "DelistChannels must not run when there's no stranded listing")
			} else {
				assert.True(t, delister.Called, "DelistChannels must run when swapping to a different card")
				assert.Equal(t, tt.oldInventoryID, delister.InventoryID, "delist targets the OLD inventory ID")
				assert.Nil(t, delister.Channels, "empty channels = delist from all")
			}
		})
	}
}

func TestHandleFixMatch_InvalidURLOrCardID(t *testing.T) {
	tests := []struct {
		name             string
		url              string
		wantBodyContains string
	}{
		{"wrong domain", "https://example.com/card/123/foo", "invalid DH URL"},
		{"missing card path", "https://doubleholo.com/products/123", "invalid DH URL"},
		{"missing id segment", "https://doubleholo.com/card/foo", "invalid DH URL"},
		{"empty path", "https://doubleholo.com/", "invalid DH URL"},
		{"plain text", "not a url at all", "invalid DH URL"},
		{"zero card id", "https://doubleholo.com/card/0/zero", "invalid card ID"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := fixMatchValidationHandler()

			body := `{"purchaseId":"p1","dhUrl":"` + tt.url + `"}`
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/dh/fix-match", strings.NewReader(body))
			req = withUser(req)
			h.HandleFixMatch(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("url %q: got %d, want 400 (body=%s)", tt.url, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.wantBodyContains) {
				t.Errorf("url %q: body should contain %q, got %s", tt.url, tt.wantBodyContains, rec.Body.String())
			}
		})
	}
}
