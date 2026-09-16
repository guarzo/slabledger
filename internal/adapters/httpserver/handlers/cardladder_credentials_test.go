package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cl "github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	domain "github.com/guarzo/slabledger/internal/domain/cardladder"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestCardLadderSaveActivatesSharedClientThenNotifies(t *testing.T) {
	firebase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/accounts:signInWithPassword", r.URL.Path)
		_ = json.NewEncoder(w).Encode(cl.FirebaseAuthResponse{IDToken: "id", RefreshToken: "saved-refresh", LocalID: "uid"})
	}))
	defer firebase.Close()
	for _, tc := range []struct {
		name               string
		saveErr, notifyErr error
		want               int
		notified           bool
	}{
		{"first save", nil, nil, 200, true}, {"save failure", errors.New("private db detail"), nil, 500, false},
		{"notification failure", nil, errors.New("private worker detail"), 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var saved *domain.Config
			store := &mocks.CardLadderStoreMock{GetConfigFn: func(context.Context) (*domain.Config, error) { return saved, nil },
				SaveConfigFn: func(_ context.Context, email, token, collection, key, uid string) error {
					if tc.saveErr != nil {
						return tc.saveErr
					}
					saved = &domain.Config{Email: email, RefreshToken: token, CollectionID: collection, FirebaseAPIKey: key, FirebaseUID: uid}
					return nil
				},
			}
			logger := mocks.NewMockLogger()
			shared := cl.NewConfiguredClient(store, nil, logger, cl.WithAuthBaseURL(firebase.URL), cl.WithTokenBaseURL(firebase.URL))
			refresher := scheduler.NewCardLadderRefreshScheduler(nil, nil, nil, nil, nil, nil, logger, config.CardLadderConfig{})
			shared.SetConsumer(refresher.SetClient)
			notified := false
			h := NewCardLadderHandler(store, nil, logger)
			h.SetConfiguredClient(shared, func(context.Context) error {
				notified = true
				require.NotNil(t, shared.Current(), "client updated before notification")
				require.True(t, shared.Current().Available())
				return tc.notifyErr
			})
			r := httptest.NewRequest("POST", "/api/admin/cardladder/config", strings.NewReader(`{"email":"fixture@example.test","password":"fixture","collectionId":"collection","firebaseApiKey":"key"}`))
			w := httptest.NewRecorder()
			h.HandleSaveConfig(w, r)
			require.Equal(t, tc.want, w.Code)
			require.Equal(t, tc.notified, notified)
			require.NotContains(t, w.Body.String(), "private")
			if tc.notifyErr != nil {
				require.Contains(t, w.Body.String(), `"status":"saved"`)
				require.Contains(t, w.Body.String(), `"workerNotification":"failed"`)
			}
		})
	}
}
