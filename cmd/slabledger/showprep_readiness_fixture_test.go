package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/google/uuid"
	"github.com/guarzo/slabledger/internal/adapters/clients/google"
	"github.com/guarzo/slabledger/internal/adapters/httpserver"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

const readinessDBURL = "postgres://showprep:showprep_test@127.0.0.1:44620/showprep_readiness_e2e?sslmode=disable"
const readinessToken = "show-readiness-local-fixture"

func readinessPurchase(i int) string { return fmt.Sprintf("11111111-1111-4111-8111-%012d", i) }

// This harness cannot be linked into the app. Reset is allowed only against the
// exact disposable database owned by this test, never DATABASE_URL or PG defaults.
func readinessDB(t *testing.T, raw string) *postgres.DB {
	t.Helper()
	require.Equal(t, readinessDBURL, raw, "refusing any database except the owned e2e fixture")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := openReadinessDB(ctx, raw)
	require.NoError(t, err)
	return db
}

func openReadinessDB(ctx context.Context, raw string) (*postgres.DB, error) {
	if raw != readinessDBURL {
		return nil, fmt.Errorf("refusing any database except the owned e2e fixture")
	}
	db, err := postgres.Open(ctx, raw, mocks.NewMockLogger())
	if err != nil {
		return nil, err
	}
	var name, user, owner string
	err = db.QueryRowContext(ctx, `SELECT current_database(),current_user,pg_get_userbyid(datdba) FROM pg_database WHERE datname=current_database()`).Scan(&name, &user, &owner)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("read fixture database owner: %w", err)
	}
	if name != "showprep_readiness_e2e" || user != "showprep" || owner != user {
		_ = db.Close()
		return nil, fmt.Errorf("fixture database owner mismatch: name=%q user=%q owner=%q", name, user, owner)
	}
	return db, nil
}

func seedReadinessUpgrade(t *testing.T, db *postgres.DB, now time.Time) map[string]string {
	t.Helper()
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	driver, err := migratepgx.WithInstance(db.DB, &migratepgx.Config{})
	require.NoError(t, err)
	files, err := iofs.New(postgres.MigrationsFS, "migrations")
	require.NoError(t, err)
	migration, err := migrate.NewWithInstance("iofs", files, "pgx5", driver)
	require.NoError(t, err)
	// Seed the old ledger before upgrading: legacy comps do not certify evidence.
	require.NoError(t, migration.Migrate(45))
	_, err = db.ExecContext(ctx, `INSERT INTO campaigns(id,name,phase) VALUES('readiness-campaign','Readiness fixture','active')`)
	require.NoError(t, err)
	for i := 1; i <= 26; i++ {
		name, profile := fmt.Sprintf("Readiness slab %02d", i), fmt.Sprintf("psa-%d", (i+1)/2)
		asking, override := 30000, 29000
		if i == 25 {
			name, profile = "Outside acquisition scope", "outside"
		}
		if i == 26 {
			name, profile = "Readiness missing price", "no-price"
			asking, override = 0, 0
		}
		_, err = db.ExecContext(ctx, `INSERT INTO campaign_purchases
		(id,campaign_id,card_name,cert_number,grader,grade_value,purchase_date,received_at,gem_rate_id,
		buy_cost_cents,cl_value_cents,override_price_cents,reviewed_price_cents,dh_card_id,dh_inventory_id,dh_status,dh_push_status,dh_listing_price_cents,dh_channels_json)
		VALUES($1,'readiness-campaign',$2,$3,'PSA',10,$4,$4,$5,18000,31000,$8,$7,$6,$6,'listed','synced',40000,'["ebay"]')`,
			readinessPurchase(i), name, fmt.Sprintf("910000%02d", i), now.AddDate(0, 0, -10).Format(time.DateOnly), profile, 1000+i, asking, override)
		require.NoError(t, err)
	}
	// A real historical sale also has to remain byte-for-byte unchanged.
	_, err = db.ExecContext(ctx, `INSERT INTO campaign_purchases(id,campaign_id,card_name,cert_number,grader,grade_value,purchase_date)
		VALUES($1,'readiness-campaign','Sold history','91999999','PSA',10,'2026-01-01');
		`, readinessPurchase(27))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_price_cents,sale_fee_cents,sale_date)
		VALUES('historical-sale',$1,'cardshow',25000,1000,'2026-02-01')`, readinessPurchase(27))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO cl_sales_comps(gem_rate_id,item_id,sale_date,price_cents,platform,condition)
		VALUES('psa-1','legacy-comp',$1,99900,'ebay','g10')`, now.Format(time.DateOnly))
	require.NoError(t, err)
	legacy, err := readinessLedger(ctx, db)
	require.NoError(t, err)
	require.NoError(t, migration.Up())
	upgraded, err := readinessLedger(ctx, db)
	require.NoError(t, err)
	require.Equal(t, legacy, upgraded, "45->47 must preserve every financial and legacy comp row")
	var version, count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT version FROM schema_migrations WHERE NOT dirty`).Scan(&version))
	require.Equal(t, 47, version)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM showprep_evidence`).Scan(&count))
	require.Zero(t, count)
	return legacy
}

type readinessSourceFixture struct {
	mu               sync.Mutex
	now              time.Time
	started          time.Time
	mode             string
	calls            []url.Values
	gate             chan struct{}
	workerDataset    bool
	providerRequests []map[string]string
}

func TestReadinessFixtureClockAdvances(t *testing.T) {
	base := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	f := &readinessSourceFixture{now: base, started: time.Now().Add(-time.Minute)}
	require.WithinDuration(t, base.Add(time.Minute), f.clock(), time.Second,
		"interactive checks must not become invalid when real source timestamps advance")
}

func (f *readinessSourceFixture) clock() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Match the source adapter's wall clock, including host clock corrections.
	return time.Now().UTC().Add(f.now.Sub(f.started))
}
func (f *readinessSourceFixture) setMode(mode string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gate != nil {
		close(f.gate)
		f.gate = nil
	}
	f.mode = mode
	if mode == "hold" {
		f.gate = make(chan struct{})
	}
}
func (f *readinessSourceFixture) serve(w http.ResponseWriter, r *http.Request) error {
	f.mu.Lock()
	f.calls = append(f.calls, r.URL.Query())
	f.providerRequests = append(f.providerRequests, map[string]string{"method": r.Method, "path": r.URL.Path})
	blocked := f.mode == "blocked"
	f.mu.Unlock()
	if blocked {
		http.Error(w, "provider access blocked during cached use", http.StatusServiceUnavailable)
		return nil
	}
	if r.Header.Get("Authorization") != "Bearer source-fixture" {
		return fmt.Errorf("source fixture token required")
	}
	q := r.URL.Query()
	for key, want := range map[string]string{"index": "salesarchive", "sort": "date", "direction": "desc", "limit": "100", "page": "0"} {
		if q.Get(key) != want {
			return fmt.Errorf("source parameter %s: got %q, want %q", key, q.Get(key), want)
		}
	}
	parts := strings.Split(q.Get("filters"), "|")
	if len(parts) != 3 || parts[0] != "condition:g10" || parts[2] != "gradingCompany:psa" || !strings.HasPrefix(parts[1], "profileId:") || parts[1] == "profileId:" {
		return fmt.Errorf("source filters: got %q, want condition:g10|profileId:<id>|gradingCompany:psa", q.Get("filters"))
	}
	profile := strings.TrimPrefix(parts[1], "profileId:")
	f.mu.Lock()
	now, mode, gate, workerDataset := time.Now().UTC().Add(f.now.Sub(f.started)), f.mode, f.gate, f.workerDataset
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-r.Context().Done():
			return nil
		}
	}
	if mode == "failed" {
		http.Error(w, "controlled source authorization failure", http.StatusUnauthorized)
		return nil
	}
	hits := []map[string]any{}
	for i := 0; i < 2; i++ {
		hits = append(hits, map[string]any{"itemId": fmt.Sprintf("%s-%s-%d", profile, now.Format(time.DateOnly), i),
			"profileId": profile, "condition": "g10", "gradingCompany": "psa", "date": now.Format(time.DateOnly),
			"price": 270.0 + float64(i)*20, "currency": "USD", "platform": "ebay", "listingType": "BestOffer"})
	}
	if workerDataset {
		switch profile {
		case "cached-31":
			hits = []map[string]any{}
		case "cached-32":
			hits = hits[:1]
		case "cached-33":
			hits[0]["price"], hits[1]["price"] = 200.0, 220.0
		case "cached-29":
			mode = "partial"
		}
	}
	total := len(hits)
	if mode == "partial" {
		total = 3
	} // Real adapter detects an incomplete page, not a fabricated evaluation.
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"hits": hits, "totalHits": total}); err != nil {
		return fmt.Errorf("encode source response for %s: %w", profile, err)
	}
	return nil
}

func readinessRouter(db *postgres.DB, f *readinessSourceFixture, result *scheduler.BuildResult, configured ...*handlers.CampaignsHandler) http.Handler {
	logger := mocks.NewMockLogger()
	// The same cached-only service capability as production. Only its business
	// clock is controlled here; source access belongs exclusively to scheduling.
	service := sp.NewService(postgres.NewShowPrepStore(db.DB), nil, f.clock)
	campaigns := readinessCampaigns(db)
	var campaignHandler *handlers.CampaignsHandler
	if len(configured) > 0 {
		campaignHandler = configured[0]
	}
	// OAuth transport is never called: actual LocalAPIToken middleware resolves
	// the fixture user through the real auth service and PostgreSQL repository.
	auth := google.NewOAuthService(postgres.NewAuthRepository(db.DB, nil), logger, "", "", "", nil)
	return httpserver.NewRouter(httpserver.RouterConfig{ShowPrepHandler: handlers.NewShowPrepHandler(service, logger),
		ShowPrepWorkerHandler: buildShowPrepWorkerHandler(handlerInputs{SchedulerResult: result}),
		CampaignsService:      campaigns, CampaignsHandler: campaignHandler, AuthService: auth, LocalAPIToken: readinessToken, GoogleOAuthEnv: "development",
		Logger: logger, SPAHandler: handlers.NewSPAHandler(logger), HealthHandler: handlers.NewHealthHandler(nil, nil, logger)}).Setup()
}

func readinessCampaigns(db *postgres.DB) inventory.Service {
	logger := mocks.NewMockLogger()
	return inventory.NewService(postgres.NewCampaignStore(db.DB, logger), postgres.NewPurchaseStore(db.DB, logger),
		postgres.NewSaleStore(db.DB, logger), postgres.NewAnalyticsStore(db.DB, logger), postgres.NewFinanceStore(db.DB, logger),
		postgres.NewPricingStore(db.DB, logger), postgres.NewDHStore(db.DB, logger),
		inventory.WithIDGenerator(uuid.NewString),
		inventory.WithCompSummaryProvider(inventory.NewCompositeCompProvider(postgres.NewCLSalesStore(db.DB), postgres.NewDHCompCacheStore(db.DB))))
}

// Whole persisted rows catch changes to every price/purchase/DH field, not just
// the few columns the UI happens to display. Ordering is explicit and stable.
func readinessLedger(ctx context.Context, db *postgres.DB) (map[string]string, error) {
	out := map[string]string{}
	for _, table := range []string{"campaigns", "campaign_purchases", "campaign_sales", "cl_sales_comps"} {
		var value string
		if err := db.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY id),'[]'::jsonb)::text FROM `+table+` t`).Scan(&value); err != nil {
			return nil, fmt.Errorf("read fixture ledger %s: %w", table, err)
		}
		out[table] = value
	}
	return out, nil
}

func readinessRows(ctx context.Context, db *postgres.DB, table string) (json.RawMessage, error) {
	var value string
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(t)),'[]'::jsonb)::text FROM `+table+` t`).Scan(&value); err != nil {
		return nil, fmt.Errorf("read fixture rows %s: %w", table, err)
	}
	return json.RawMessage(value), nil
}

func startReadinessSource(t *testing.T, f *readinessSourceFixture) *httptest.Server {
	t.Helper()
	source := httptest.NewServer(readinessHandler(t.Errorf, f.serve))
	t.Cleanup(source.Close)
	t.Cleanup(func() { f.setMode("complete") })
	return source
}
