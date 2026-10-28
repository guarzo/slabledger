package dh

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/stretchr/testify/require"
)

func TestPSARotation_LongClassifiedRateLimit(t *testing.T) {
	ctx := context.Background()
	operations := []struct {
		name string
		run  func(*Client) error
	}{
		{"update", func(c *Client) error {
			_, err := UpdateInventoryWithRotation(ctx, 1, InventoryUpdate{Status: InventoryStatusListed}, c.UpdateInventory, c.RotatePSAKey, nopLogger{}, "test")
			return err
		}},
		{"resolve", func(c *Client) error {
			_, err := ResolveCertWithRotation(ctx, CertResolveRequest{CertNumber: "12345678"}, c.ResolveCert, c.RotatePSAKey, nopLogger{}, "test")
			return err
		}},
	}
	message := strings.Repeat("x", 145) + " daily limit reached"
	bodies := []struct{ name, json string }{
		{"message", fmt.Sprintf(`{"error":%q}`, message)},
		{"body", fmt.Sprintf(`{"error":"rejected","detail":%q}`, message)},
	}
	for _, body := range bodies {
		for _, operation := range operations {
			t.Run(body.name+"/"+operation.name, func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					if r.Header.Get("X-PSA-API-Key") != "second" {
						w.WriteHeader(http.StatusUnprocessableEntity)
						_, _ = fmt.Fprint(w, body.json)
						return
					}
					_, _ = fmt.Fprint(w, `{"status":"matched"}`)
				}))
				defer server.Close()
				c := NewClient(server.URL, WithEnterpriseKey("test_key"), WithPSAKeys("first,second"), WithRateLimitRPS(1000))

				// Use the real HTTP classifier; the phrase is lost only in Error(),
				// not in the wrapped upstream message/body used for key rotation.
				_, err := c.httpClient.Get(ctx, server.URL, nil, 0)
				require.True(t, apperrors.HasErrorCode(err, apperrors.ErrCodeProviderInvalidReq))
				require.NotContains(t, err.Error(), "daily limit reached")
				require.True(t, IsPSARateLimitError(err))
				require.NoError(t, operation.run(c))
				require.Equal(t, "second", c.currentPSAKey())
				require.EqualValues(t, 3, calls.Load(), "probe, rejected first key, successful second key")
			})
		}
	}
}
