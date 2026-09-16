package cardladder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domain "github.com/guarzo/slabledger/internal/domain/cardladder"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestConfiguredClientLateSaveCrossInstanceAndReadOnly(t *testing.T) {
	var cfg *domain.Config
	store := &mocks.CardLadderStoreMock{GetConfigFn: func(context.Context) (*domain.Config, error) { return cfg, nil }}
	owner := NewConfiguredClient(store, nil, mocks.NewMockLogger())
	configured, err := owner.Configured(context.Background())
	require.NoError(t, err)
	require.False(t, configured)
	require.Nil(t, owner.Current())
	require.NoError(t, owner.Refresh(context.Background()))
	require.Nil(t, owner.Current())
	cfg = &domain.Config{FirebaseAPIKey: "key", RefreshToken: "first"}
	configured, err = owner.Configured(context.Background())
	require.NoError(t, err)
	require.True(t, configured)
	require.Nil(t, owner.Current(), "read status must not activate client")
	require.NoError(t, owner.Refresh(context.Background()))
	client := owner.Current()
	require.NotNil(t, client)
	limiter := client.rateLimiter
	client.SetToken("cached-id-token", time.Now().Add(time.Hour))
	require.NoError(t, owner.Refresh(context.Background()))
	token, err := client.getToken(context.Background())
	require.NoError(t, err)
	require.Equal(t, "cached-id-token", token)
	cfg = &domain.Config{FirebaseAPIKey: "other-instance-key", RefreshToken: "second"}
	require.NoError(t, owner.Refresh(context.Background()))
	require.Same(t, client, owner.Current())
	require.Same(t, limiter, client.rateLimiter)
	require.Equal(t, "second", client.refreshToken)
	require.Empty(t, client.token.IDToken)
	a, err := owner.Source(context.Background())
	require.NoError(t, err)
	b, err := owner.Source(context.Background())
	require.NoError(t, err)
	require.NotSame(t, a, b, "each sweep gets a fresh singleflight wrapper")
	cfg = nil
	require.NoError(t, owner.Refresh(context.Background()))
	require.Nil(t, owner.Current())
	require.False(t, client.Available())
}

func TestConfiguredClientCanceledSweepDoesNotWaitForConfigRead(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	store := &mocks.CardLadderStoreMock{GetConfigFn: func(context.Context) (*domain.Config, error) {
		select {
		case <-entered:
		default:
			close(entered)
		}
		<-release
		return nil, nil
	}}
	owner := NewConfiguredClient(store, nil, mocks.NewMockLogger())
	first := make(chan error, 1)
	go func() { first <- owner.Refresh(context.Background()) }()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	second := make(chan error, 1)
	go func() { second <- owner.Refresh(ctx) }()
	var stopped bool
	select {
	case err := <-second:
		stopped = true
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
	}
	close(release)
	<-first
	if !stopped {
		<-second
	}
	require.True(t, stopped, "shutdown cannot wait behind another config read")
}

func TestCredentialsRepairDoesNotJoinOrPublishOldTokenRefresh(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		token := r.Form.Get("refresh_token")
		if token == "old" {
			close(entered)
			<-release
		}
		_ = json.NewEncoder(w).Encode(FirebaseRefreshResponse{IDToken: token + "-id", RefreshToken: token + "-rotated", ExpiresIn: "3600"})
	}))
	defer server.Close()
	auth := NewFirebaseAuth("fixture", WithTokenBaseURL(server.URL))
	c := NewClient(WithTokenManager(auth, "old", time.Time{}))
	oldDone := make(chan error, 1)
	go func() { _, err := c.getToken(context.Background()); oldDone <- err }()
	<-entered
	c.UpdateCredentials(auth, "new")
	result := make(chan string, 1)
	go func() { token, _ := c.getToken(context.Background()); result <- token }()
	var token string
	select {
	case token = <-result:
	case <-time.After(time.Second):
	}
	close(release)
	<-oldDone
	require.Equal(t, "new-id", token, "new generation must not join old refresh")
	token, err := c.getToken(context.Background())
	require.NoError(t, err)
	require.Equal(t, "new-id", token, "old completion cannot overwrite current token")
}
