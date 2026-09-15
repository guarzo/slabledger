package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/stretchr/testify/require"
)

// Report independently of the HTTP response: browser retries/tolerated source
// failures must never turn a broken fixture into a passing browser regression.
func readinessHandler(report func(string, ...any), serve func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := serve(w, r); err != nil {
			report("%s %s: %v", r.Method, r.URL.Path, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// Shared with the real browser control server; HTTP regressions exercise these
// exact ledger checks and persisted-row reads rather than a simulated response.
func serveReadinessState(w http.ResponseWriter, r *http.Request, db *postgres.DB, baseline map[string]string, state map[string]any) error {
	ledger, err := readinessLedger(r.Context(), db)
	if err != nil {
		return err
	}
	for table, value := range ledger {
		if baseline[table] != value {
			return fmt.Errorf("financial/legacy ledger changed: %s", table)
		}
	}
	for _, field := range []struct{ key, table string }{
		{"evidence", "showprep_evidence"}, {"lists", "showprep_lists"},
		{"items", "showprep_items"}, {"holds", "showprep_price_holds"},
	} {
		rows, err := readinessRows(r.Context(), db, field.table)
		if err != nil {
			return err
		}
		state[field.key] = rows
	}
	state["ledgerUnchanged"] = true
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(state); err != nil {
		return fmt.Errorf("encode fixture state: %w", err)
	}
	return nil
}

func TestReadinessControlErrors(t *testing.T) {
	raw := os.Getenv("SHOW_READINESS_E2E_URL")
	if raw == "" {
		t.Skip("requires the explicitly owned disposable e2e database")
	}
	db := readinessDB(t, raw)
	defer func() { require.NoError(t, db.Close()) }()
	seedReadinessUpgrade(t, db, time.Now().UTC())
	baseline, err := readinessLedger(t.Context(), db)
	require.NoError(t, err)
	for _, tc := range []struct{ name, breakSQL, restoreSQL, want string }{
		{"ledger changed", "UPDATE campaigns SET name='unexpected mutation'", "UPDATE campaigns SET name='Readiness fixture'", "financial/legacy ledger changed: campaigns"},
		{"ledger read", "ALTER TABLE cl_sales_comps RENAME TO hidden_comps", "ALTER TABLE hidden_comps RENAME TO cl_sales_comps", "read fixture ledger cl_sales_comps"},
		{"row read", "ALTER TABLE showprep_evidence RENAME TO hidden_evidence", "ALTER TABLE hidden_evidence RENAME TO showprep_evidence", "read fixture rows showprep_evidence"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.RWMutex
			failures := make(chan string, 8)
			server := httptest.NewServer(readinessHandler(func(format string, args ...any) {
				failures <- fmt.Sprintf(format, args...)
			}, func(w http.ResponseWriter, r *http.Request) error {
				mu.RLock()
				defer mu.RUnlock()
				return serveReadinessState(w, r, db, baseline, map[string]any{})
			}))
			defer server.Close()
			_, err := db.ExecContext(t.Context(), tc.breakSQL)
			require.NoError(t, err)
			restored := false
			restore := func() {
				if !restored {
					_, err := db.ExecContext(t.Context(), tc.restoreSQL)
					require.NoError(t, err)
					restored = true
				}
			}
			defer restore()
			client := &http.Client{Timeout: time.Second}
			response, err := client.Get(server.URL + "/state")
			require.NoError(t, err, "broken control reads must return diagnostic HTTP responses")
			body, err := io.ReadAll(response.Body)
			require.NoError(t, response.Body.Close())
			require.NoError(t, err)
			require.Equal(t, http.StatusInternalServerError, response.StatusCode)
			require.Contains(t, string(body), tc.want)
			require.Len(t, failures, 1, "HTTP failure must also be recorded against the owning test")
			require.Contains(t, <-failures, "GET /state: "+tc.want)
			require.True(t, mu.TryLock(), "control failures must release the request lock")
			mu.Unlock()
			restore()
			response, err = client.Get(server.URL + "/state")
			require.NoError(t, err, "requests must continue after repairing the fixture")
			body, err = io.ReadAll(response.Body)
			require.NoError(t, response.Body.Close())
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, response.StatusCode)
			require.JSONEq(t, `{"evidence":[],"lists":[],"items":[],"holds":[],"ledgerUnchanged":true}`, string(body))
			require.Empty(t, failures)
			server.Close()
		})
	}
	finalLedger, err := readinessLedger(t.Context(), db)
	require.NoError(t, err)
	require.Equal(t, baseline, finalLedger)
}
