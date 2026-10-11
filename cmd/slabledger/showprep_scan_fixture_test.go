package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/stretchr/testify/require"
)

// Test-only owner: runtime restarts drain requests and rebuild all collaborators.
// Controlled price/availability changes have individually attributed baselines;
// ordinary scans and reads cannot change even one financial field.
type scanBrowserFixture struct {
	mu       sync.RWMutex
	db       *postgres.DB
	raw      string
	f        *readinessSourceFixture
	source   string
	router   http.Handler
	stop     func()
	baseline map[string]string
	requests []map[string]string
	lock     *sql.Tx
	restarts int
}

func (s *scanBrowserFixture) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requests = append(s.requests, map[string]string{"method": r.Method, "path": r.URL.Path})
	s.mu.Unlock()
	s.mu.RLock()
	defer s.mu.RUnlock()
	s.router.ServeHTTP(w, r)
}
func (s *scanBrowserFixture) control(t *testing.T, w http.ResponseWriter, r *http.Request) error {
	if r.Header.Get("Authorization") != "Bearer "+readinessToken {
		http.Error(w, "fixture token required", http.StatusUnauthorized)
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Method == "POST" {
		switch r.URL.Path {
		case "/restart":
			if s.lock != nil {
				return fmt.Errorf("release purchase lock before restart")
			}
			s.stop()
			if err := s.db.Close(); err != nil {
				return err
			}
			next, err := openReadinessDB(r.Context(), s.raw)
			if err != nil {
				return err
			}
			s.db = next
			result, stop := readinessRuntime(t, next, s.source, false)
			s.stop = stop
			s.router = readinessRouter(next, s.f, result)
			s.restarts++
		case "/hold-purchase":
			if s.lock != nil {
				return fmt.Errorf("purchase lock already held")
			}
			// The lock outlives this request; fixture cleanup owns its rollback.
			tx, err := s.db.BeginTx(t.Context(), nil)
			if err != nil {
				return err
			}
			if _, err = tx.ExecContext(r.Context(), `SELECT id FROM campaign_purchases WHERE id=$1 FOR UPDATE`, readinessPurchase(3)); err != nil {
				_ = tx.Rollback()
				return err
			}
			s.lock = tx
		case "/release-purchase":
			if s.lock == nil {
				return fmt.Errorf("purchase lock not held")
			}
			err := s.lock.Rollback()
			s.lock = nil
			if err != nil {
				return err
			}
		case "/asking-drift":
			if err := s.changeLedger(r.Context(), `UPDATE campaign_purchases SET reviewed_price_cents=32500 WHERE id=$1`, readinessPurchase(1), "reviewed_price_cents", float64(32500)); err != nil {
				return err
			}
		case "/unreceive":
			if err := s.changeLedger(r.Context(), `UPDATE campaign_purchases SET received_at=NULL WHERE id=$1`, readinessPurchase(2), "received_at", nil); err != nil {
				return err
			}
		case "/legacy-member":
			var body struct {
				ListID string `json:"listId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				return err
			}
			_, err := s.db.ExecContext(r.Context(), `INSERT INTO showprep_items(id,list_id,purchase_id,card_name,cert_number,grader,grade,added_at,version,acknowledged_price_cents,acknowledged_status) VALUES('99999999-9999-4999-8999-999999999999',$1,$2,'Readiness slab 04','91000004','PSA',10,now(),1,30000,'needs_review')`, body.ListID, readinessPurchase(4))
			if err != nil {
				return err
			}
		default:
			http.NotFound(w, r)
			return nil
		}
	} else if r.Method != "GET" || r.URL.Path != "/state" {
		http.NotFound(w, r)
		return nil
	}
	s.f.mu.Lock()
	calls, providers := len(s.f.calls), len(s.f.providerRequests)
	s.f.mu.Unlock()
	if calls != 0 || providers != 0 {
		return fmt.Errorf("scan/read accessed provider: %d/%d", calls, providers)
	}
	receipts, err := readinessRows(r.Context(), s.db, "showprep_command_receipts")
	if err != nil {
		return err
	}
	retired, err := readinessRows(r.Context(), s.db, "showprep_retired_lists")
	if err != nil {
		return err
	}
	return serveReadinessState(w, r, s.db, s.baseline, map[string]any{"ledger": s.baseline, "restarted": s.restarts > 0, "restarts": s.restarts, "receipts": receipts, "retired": retired, "requests": s.requests, "sourceCalls": calls, "providerRequests": providers})
}
func (s *scanBrowserFixture) changeLedger(ctx context.Context, query, id, field string, value any) error {
	before, err := readinessLedger(ctx, s.db)
	if err != nil {
		return err
	}
	for table, rows := range before {
		if rows != s.baseline[table] {
			return fmt.Errorf("ledger changed before controlled mutation: %s", table)
		}
	}
	var purchases []map[string]any
	if err := json.Unmarshal([]byte(before["campaign_purchases"]), &purchases); err != nil {
		return err
	}
	for _, p := range purchases {
		if p["id"] == id {
			p[field] = value
		}
	}
	if _, err = s.db.ExecContext(ctx, query, id); err != nil {
		return err
	}
	after, err := readinessLedger(ctx, s.db)
	if err != nil {
		return err
	}
	var actual []map[string]any
	if err := json.Unmarshal([]byte(after["campaign_purchases"]), &actual); err != nil {
		return err
	}
	if !reflect.DeepEqual(purchases, actual) {
		return fmt.Errorf("controlled mutation changed more than %s on %s", field, id)
	}
	for table, rows := range before {
		if table != "campaign_purchases" && rows != after[table] {
			return fmt.Errorf("controlled mutation changed %s", table)
		}
	}
	s.baseline = after
	return nil
}
func (s *scanBrowserFixture) close(t *testing.T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock != nil {
		require.NoError(t, s.lock.Rollback())
	}
	s.stop()
	final, err := readinessLedger(context.Background(), s.db)
	require.NoError(t, err)
	require.Equal(t, s.baseline, final)
	require.NoError(t, s.db.Close())
}

// Seed pre-rollout history without reintroducing a public planning mutation.
func seedReadinessLegacyMember(ctx context.Context, db *postgres.DB, listID string, purchase int) error {
	_, err := db.ExecContext(ctx, `INSERT INTO showprep_items(id,list_id,purchase_id,card_name,cert_number,grader,grade,added_at,version,acknowledged_price_cents,acknowledged_status)
 SELECT gen_random_uuid(),$1,id,card_name,cert_number,grader,grade_value,now(),1,reviewed_price_cents,'needs_review' FROM campaign_purchases WHERE id=$2`, listID, readinessPurchase(purchase))
	return err
}

func newScanBrowserFixture(t *testing.T, raw string, f *readinessSourceFixture, source string) *scanBrowserFixture {
	db := readinessDB(t, raw)
	seedReadinessUpgrade(t, db, time.Now().UTC())
	baseline, err := readinessLedger(t.Context(), db)
	require.NoError(t, err)
	result, stop := readinessRuntime(t, db, source, false)
	return &scanBrowserFixture{db: db, raw: raw, f: f, source: source, baseline: baseline, stop: stop, router: readinessRouter(db, f, result)}
}
