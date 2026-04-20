// Command sqlite-to-postgres is a one-shot data migration tool for the
// wanderer SQLite → Supabase Postgres cutover. It is deleted in Phase 6
// after the migration is verified.
//
// Usage:
//
//	SQLITE_PATH=/workspace/data/slabledger.db \
//	DATABASE_URL_DIRECT=postgresql://postgres:<pw>@db.<ref>.supabase.co:5432/postgres?sslmode=require \
//	go run ./cmd/sqlite-to-postgres
//
// The tool TRUNCATEs each destination table before load, so it is
// idempotent and safe to rerun on failure.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	_ "github.com/mattn/go-sqlite3"
)

// loadOrder lists tables in FK-safe dependency order (parents before
// children). Views and golang-migrate's schema_migrations table are
// excluded. 35 tables total, matches the initial Postgres schema.
var loadOrder = []string{
	"users",
	"user_sessions",
	"user_tokens",
	"oauth_states",
	"allowed_emails",
	"campaigns",
	"campaign_purchases",
	"campaign_sales",
	"invoices",
	"cashflow_config",
	"revocation_flags",
	"price_flags",
	"card_access_log",
	"card_id_mappings",
	"sync_state",
	"advisor_cache",
	"ai_calls",
	"api_calls",
	"api_rate_limits",
	"cardladder_config",
	"cl_card_mappings",
	"cl_sales_comps",
	"marketmovers_config",
	"mm_card_mappings",
	"market_intelligence",
	"dh_suggestions",
	"scoring_data_gaps",
	"sell_sheet_items",
	"dh_push_config",
	"dh_card_cache",
	"dh_character_cache",
	"dh_state_events",
	"card_price_trajectory",
	"psa_pending_items",
	"scheduler_run_stats",
}

// booleanColumns maps "<table>.<column>" entries that are stored as
// INTEGER in SQLite but re-typed to BOOLEAN in the Postgres schema.
// Values 0/1 must be coerced to false/true before INSERT.
var booleanColumns = map[string]map[string]struct{}{
	"campaigns":          {"exclusion_mode": {}},
	"campaign_purchases": {"was_refunded": {}},
	"campaign_sales":     {"sold_at_asking_price": {}, "was_cracked": {}},
	"users":              {"is_admin": {}},
	"dh_suggestions":     {"is_manual": {}},
}

// textColumns maps columns whose Postgres type is TEXT but the SQLite
// column is typed as something else (REAL, INTEGER, DATETIME affinity).
// Values are formatted as strings before INSERT to match the TEXT type.
// See "drift fix" comments in 000001_initial_schema.up.sql.
var textColumns = map[string]map[string]struct{}{
	// drift fix #1: cl_confidence REAL → TEXT ("2.5-4" ranges)
	"campaigns": {"cl_confidence": {}},
	// drift fix #4: received_at DATETIME-affinity → TEXT (*string, not timestamp)
	"campaign_purchases": {"received_at": {}},
	// unflagged drift: sale_date is DATE in SQLite (auto-parsed to time.Time by
	// mattn) but TEXT in Postgres (stored as YYYY-MM-DD).
	"cl_sales_comps": {"sale_date": {}},
}

// serialTables lists tables with BIGSERIAL id columns that need their
// sequence reset to MAX(id) after bulk import.
var serialTables = []string{
	"users",
	"user_tokens",
	"price_flags",
	"card_access_log",
	"advisor_cache",
	"ai_calls",
	"api_calls",
	"cl_sales_comps",
	"scoring_data_gaps",
	"dh_state_events",
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("%v", err)
	}
}

func run() error {
	sqlitePath := mustEnv("SQLITE_PATH")
	pgURL := mustEnv("DATABASE_URL_DIRECT")

	ctx := context.Background()

	sqliteDB, err := sql.Open("sqlite3", sqlitePath+"?mode=ro")
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	defer func() { _ = sqliteDB.Close() }()
	if err := sqliteDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sqlite: %w", err)
	}

	pgCfg, err := pgx.ParseConfig(pgURL)
	if err != nil {
		return fmt.Errorf("parse postgres url: %w", err)
	}
	pgConn, err := pgx.ConnectConfig(ctx, pgCfg)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer func() { _ = pgConn.Close(ctx) }()

	log.Printf("sqlite:   %s", sqlitePath)
	log.Printf("postgres: %s", redactURL(pgURL))
	log.Printf("loading %d tables", len(loadOrder))

	results := make([]tableResult, 0, len(loadOrder))
	start := time.Now()

	for _, table := range loadOrder {
		r, err := migrateTable(ctx, sqliteDB, pgConn, table)
		if err != nil {
			return fmt.Errorf("migrate %s: %w", table, err)
		}
		results = append(results, r)
		log.Printf("  %-25s rows=%d→%d  (%.2fs)", table, r.sqliteCount, r.postgresCount, r.duration.Seconds())
	}

	log.Printf("resetting %d sequences", len(serialTables))
	for _, table := range serialTables {
		seq := table + "_id_seq"
		q := fmt.Sprintf(
			"SELECT setval('%s', COALESCE((SELECT MAX(id) FROM %s), 1), COALESCE((SELECT MAX(id) FROM %s) IS NOT NULL, false))",
			seq, table, table,
		)
		if _, err := pgConn.Exec(ctx, q); err != nil {
			return fmt.Errorf("setval %s: %w", seq, err)
		}
		var next int64
		if err := pgConn.QueryRow(ctx, fmt.Sprintf("SELECT last_value FROM %s", seq)).Scan(&next); err != nil {
			return fmt.Errorf("read %s: %w", seq, err)
		}
		log.Printf("  %-25s → %d", seq, next)
	}

	mismatches := 0
	for _, r := range results {
		if r.sqliteCount != r.postgresCount {
			log.Printf("ROW COUNT MISMATCH  %s  sqlite=%d  postgres=%d", r.table, r.sqliteCount, r.postgresCount)
			mismatches++
		}
	}
	if mismatches > 0 {
		return fmt.Errorf("%d tables had row count mismatches", mismatches)
	}
	log.Printf("done in %s — all %d tables verified", time.Since(start).Round(time.Millisecond), len(results))
	return nil
}

type tableResult struct {
	table         string
	sqliteCount   int64
	postgresCount int64
	duration      time.Duration
}

func migrateTable(ctx context.Context, src *sql.DB, dst *pgx.Conn, table string) (tableResult, error) {
	started := time.Now()

	cols, err := sqliteColumns(ctx, src, table)
	if err != nil {
		return tableResult{}, fmt.Errorf("describe: %w", err)
	}
	if len(cols) == 0 {
		return tableResult{}, fmt.Errorf("no columns for %s", table)
	}

	if _, err := dst.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", table)); err != nil {
		return tableResult{}, fmt.Errorf("truncate: %w", err)
	}

	colList := strings.Join(cols, ", ")
	rows, err := src.QueryContext(ctx, fmt.Sprintf("SELECT %s FROM %s", colList, table))
	if err != nil {
		return tableResult{}, fmt.Errorf("select: %w", err)
	}
	defer func() { _ = rows.Close() }()

	boolCols := booleanColumns[table]
	txtCols := textColumns[table]
	src2 := &coercingSource{
		rows:     rows,
		cols:     cols,
		boolCols: boolCols,
		txtCols:  txtCols,
		table:    table,
	}

	copied, err := dst.CopyFrom(ctx, pgx.Identifier{table}, cols, src2)
	if err != nil {
		return tableResult{}, fmt.Errorf("copy: %w", err)
	}
	if src2.err != nil {
		return tableResult{}, fmt.Errorf("iterate: %w", src2.err)
	}

	var pgCount int64
	if err := dst.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&pgCount); err != nil {
		return tableResult{}, fmt.Errorf("count: %w", err)
	}

	return tableResult{
		table:         table,
		sqliteCount:   copied,
		postgresCount: pgCount,
		duration:      time.Since(started),
	}, nil
}

// coercingSource adapts a *sql.Rows iterator to pgx.CopyFromSource with
// per-column BOOLEAN and TEXT coercions applied inline.
type coercingSource struct {
	rows     *sql.Rows
	cols     []string
	boolCols map[string]struct{}
	txtCols  map[string]struct{}
	table    string
	row      int64
	current  []any
	err      error
}

func (s *coercingSource) Next() bool {
	if !s.rows.Next() {
		s.err = s.rows.Err()
		return false
	}
	s.row++
	values := make([]any, len(s.cols))
	pointers := make([]any, len(s.cols))
	for i := range values {
		pointers[i] = &values[i]
	}
	if err := s.rows.Scan(pointers...); err != nil {
		s.err = fmt.Errorf("scan row %d: %w", s.row, err)
		return false
	}
	for i, c := range s.cols {
		if values[i] == nil {
			continue
		}
		if _, ok := s.boolCols[c]; ok {
			switch v := values[i].(type) {
			case int64:
				values[i] = v != 0
			case int:
				values[i] = v != 0
			case bool:
				// already bool
			default:
				s.err = fmt.Errorf("row %d: cannot coerce %s.%s (%T=%v) to bool", s.row, s.table, c, v, v)
				return false
			}
			continue
		}
		if _, ok := s.txtCols[c]; ok {
			switch v := values[i].(type) {
			case string:
				// already string
			case []byte:
				values[i] = string(v)
			case float64:
				values[i] = strconv.FormatFloat(v, 'f', -1, 64)
			case int64:
				values[i] = strconv.FormatInt(v, 10)
			case int:
				values[i] = strconv.Itoa(v)
			case bool:
				values[i] = strconv.FormatBool(v)
			case time.Time:
				values[i] = formatTextTime(s.table, c, v)
			default:
				s.err = fmt.Errorf("row %d: cannot coerce %s.%s (%T=%v) to text", s.row, s.table, c, v, v)
				return false
			}
			continue
		}
	}
	s.current = values
	return true
}

func (s *coercingSource) Values() ([]any, error) {
	return s.current, nil
}

func (s *coercingSource) Err() error {
	return s.err
}

// formatTextTime chooses the string format for a time.Time value that
// needs to land in a Postgres TEXT column. Date-only columns (SQLite
// DATE affinity) use YYYY-MM-DD to match how the app writes them.
func formatTextTime(table, col string, t time.Time) string {
	if table == "cl_sales_comps" && col == "sale_date" {
		return t.Format("2006-01-02")
	}
	return t.Format("2006-01-02 15:04:05")
}

func sqliteColumns(ctx context.Context, db *sql.DB, table string) ([]string, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var cols []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("env %s is required", key)
	}
	return v
}

// redactURL hides the password portion for log output.
func redactURL(u string) string {
	at := strings.LastIndex(u, "@")
	if at < 0 {
		return u
	}
	scheme := strings.Index(u[:at], "//")
	if scheme < 0 {
		return u
	}
	colon := strings.Index(u[scheme+2:at], ":")
	if colon < 0 {
		return u
	}
	return u[:scheme+2+colon+1] + "***" + u[at:]
}
