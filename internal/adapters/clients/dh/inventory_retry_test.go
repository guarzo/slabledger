package dh

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/stretchr/testify/require"
)

func retryTestClient(url string, timeout time.Duration) *Client {
	c := newTestClient(url)
	config := httpx.DefaultConfig("DH")
	config.RetryPolicy.MaxRetries = 2
	config.RetryPolicy.InitialBackoff = time.Millisecond
	config.RetryPolicy.MaxBackoff = 2 * time.Millisecond
	c.httpClient = httpx.NewClient(config)
	c.timeout = timeout
	return c
}

func TestUnsafeInventoryMutationsOneAttempt(t *testing.T) {
	tests := []struct {
		name, method, path string
		call               func(*Client) error
	}{
		{"patch", "PATCH", "/api/v1/enterprise/inventory/147840", func(c *Client) error {
			_, err := c.UpdateInventory(context.Background(), 147840, InventoryUpdate{Status: "listed"})
			return err
		}},
		{"delete", "DELETE", "/api/v1/enterprise/inventory/147840", func(c *Client) error {
			return c.DeleteInventory(context.Background(), 147840)
		}},
		{"push", "POST", "/api/v1/enterprise/inventory", func(c *Client) error {
			_, err := c.PushInventory(context.Background(), []InventoryItem{{CertNumber: "160944741"}})
			return err
		}},
		{"sync", "POST", "/api/v1/enterprise/inventory/147840/sync", func(c *Client) error {
			_, err := c.SyncChannels(context.Background(), 147840, []string{"ebay"})
			return err
		}},
		{"delist", "DELETE", "/api/v1/enterprise/inventory/147840/sync", func(c *Client) error {
			_, err := c.DelistChannels(context.Background(), 147840, nil)
			return err
		}},
		{"psa import", "POST", "/api/v1/enterprise/inventory/psa_import", func(c *Client) error {
			c.psaKeys = []string{"local-test-psa-key"}
			_, err := c.PSAImport(context.Background(), []PSAImportItem{{CertNumber: "160944741"}})
			return err
		}},
	}
	failures := []struct {
		name    string
		timeout bool
	}{{"5xx", false}, {"timeout", true}}
	for _, tt := range tests {
		for _, failure := range failures {
			t.Run(tt.name+"/"+failure.name, func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					require.Equal(t, tt.method, r.Method)
					require.Equal(t, tt.path, r.URL.Path)
					if failure.timeout {
						time.Sleep(50 * time.Millisecond)
					}
					w.WriteHeader(http.StatusServiceUnavailable)
				}))
				defer server.Close()
				timeout := time.Second
				if failure.timeout {
					timeout = 15 * time.Millisecond
				}
				err := tt.call(retryTestClient(server.URL, timeout))
				require.Error(t, err)
				require.Equal(t, int32(1), calls.Load(), "an unsafe mutation cannot hide an earlier attempt")
				var detail *httpx.RequestError
				require.ErrorAs(t, err, &detail)
				require.True(t, detail.Uncertain)
			})
		}
	}
}

func TestKeyedInventoryMutationsRetryIdenticalRequest(t *testing.T) {
	tests := []struct {
		name, path, response, wantBody string
		call                           func(*Client) error
	}{
		{"return", "/api/v1/enterprise/inventory/147840/return-to-stock",
			`{"dh_inventory_id":147840,"item_status":"in_stock","external_sale_id":443,"restored":false}`,
			`{"return_confirmed":true}`, func(c *Client) error {
				_, err := c.ReturnInventoryToStock(context.Background(), 147840, "persisted-key")
				return err
			}},
		{"sale", "/api/v1/enterprise/inventory/147840/sale",
			`{"sale_id":443,"dh_inventory_id":147840,"item_status":"sold","replayed":true}`,
			`{"sale_price_cents":12000,"quantity":1,"sold_at":"2026-09-27T00:00:00Z"}`, func(c *Client) error {
				_, err := c.RecordInventorySale(context.Background(), 147840, "persisted-key", InventorySaleRequest{SalePriceCents: 12000, SoldAt: "2026-09-27T00:00:00Z"})
				return err
			}},
	}
	failures := []string{"5xx", "timeout", "response read"}
	for _, tt := range tests {
		for _, failure := range failures {
			t.Run(tt.name+"/"+failure, func(t *testing.T) {
				var mu sync.Mutex
				var bodies, keys []string
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					require.Equal(t, tt.path, r.URL.Path)
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					mu.Lock()
					bodies = append(bodies, string(body))
					keys = append(keys, r.Header.Get("Idempotency-Key"))
					first := len(bodies) == 1
					mu.Unlock()
					if first {
						switch failure {
						case "5xx":
							w.WriteHeader(503)
							return
						case "timeout":
							time.Sleep(50 * time.Millisecond)
						case "response read":
							w.Header().Set("Content-Length", "100")
							_, _ = io.WriteString(w, "short")
							return
						}
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, tt.response)
				}))
				timeout := time.Second
				if failure == "timeout" {
					timeout = 15 * time.Millisecond
				}
				err := tt.call(retryTestClient(server.URL, timeout))
				server.Close() // includes the first request continuing after client timeout
				require.NoError(t, err)
				mu.Lock()
				require.Equal(t, []string{tt.wantBody, tt.wantBody}, bodies)
				require.Equal(t, []string{"persisted-key", "persisted-key"}, keys)
				mu.Unlock()
			})
		}
	}
}

func TestInventoryReadStillRetries(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(503)
			return
		}
		_, _ = io.WriteString(w, `{"results":[],"meta":{"page":1,"per_page":100,"total_count":0}}`)
	}))
	defer server.Close()
	_, err := retryTestClient(server.URL, time.Second).ListInventory(context.Background(), InventoryFilters{CertNumber: "160944741"})
	require.NoError(t, err)
	require.Equal(t, int32(3), calls.Load())
}
