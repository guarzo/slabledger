# DH Push Safety Gates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add safety gates to DH inventory re-pushes — use recommended price hierarchy, hold re-pushes on large price swings or source disagreement, make thresholds admin-configurable, and surface held items in a renamed "Needs Attention" inventory tab.

**Architecture:** Domain function `ResolveMarketValueCents` replaces raw `CLValueCents` in all 5 push sites. New `held` status in the DH push state machine with `dh_hold_reason` column. Configurable thresholds stored in `dh_push_config` table. Frontend renames "Exceptions" tab to "Needs Attention" and adds held-item approve/adjust actions.

**Tech Stack:** Go 1.26, SQLite, React/TypeScript, TanStack Query

---

### Task 1: Migration — Add `dh_hold_reason` column and `dh_push_config` table

**Files:**
- Create: `internal/adapters/storage/sqlite/migrations/000044_dh_push_safety.up.sql`
- Create: `internal/adapters/storage/sqlite/migrations/000044_dh_push_safety.down.sql`

- [ ] **Step 1: Create up migration**

```sql
-- 000044_dh_push_safety.up.sql
ALTER TABLE campaign_purchases ADD COLUMN dh_hold_reason TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS dh_push_config (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    swing_pct_threshold INTEGER NOT NULL DEFAULT 20,
    swing_min_cents INTEGER NOT NULL DEFAULT 5000,
    disagreement_pct_threshold INTEGER NOT NULL DEFAULT 25,
    unreviewed_change_pct_threshold INTEGER NOT NULL DEFAULT 15,
    unreviewed_change_min_cents INTEGER NOT NULL DEFAULT 3000,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO dh_push_config (id) VALUES (1);
```

- [ ] **Step 2: Create down migration**

```sql
-- 000044_dh_push_safety.down.sql
DROP TABLE IF EXISTS dh_push_config;

ALTER TABLE campaign_purchases DROP COLUMN dh_hold_reason;
```

- [ ] **Step 3: Run migration test**

Run: `go test ./internal/adapters/storage/sqlite/... -run TestMigrations -count=1`
Expected: PASS (migrations apply and roll back cleanly)

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/storage/sqlite/migrations/000044_dh_push_safety.up.sql \
        internal/adapters/storage/sqlite/migrations/000044_dh_push_safety.down.sql
git commit -m "feat: add migration 000044 for DH push safety gates"
```

---

### Task 2: Domain types — `DHPushConfig`, `DHPushStatusHeld`, `ResolveMarketValueCents`

**Files:**
- Modify: `internal/domain/campaigns/types.go` (add `DHPushStatusHeld`, `DHHoldReason`, `DHPushConfig`)
- Create: `internal/domain/campaigns/dh_push_safety.go` (hold evaluation + price resolution)
- Create: `internal/domain/campaigns/dh_push_safety_test.go`

- [ ] **Step 1: Write failing tests for `ResolveMarketValueCents`**

Create `internal/domain/campaigns/dh_push_safety_test.go`:

```go
package campaigns

import "testing"

func TestResolveMarketValueCents(t *testing.T) {
	tests := []struct {
		name     string
		purchase Purchase
		want     int
	}{
		{
			name:     "reviewed price takes priority",
			purchase: Purchase{ReviewedPriceCents: 5000, CLValueCents: 3000},
			want:     5000,
		},
		{
			name:     "falls back to CL value",
			purchase: Purchase{CLValueCents: 3000},
			want:     3000,
		},
		{
			name:     "returns zero when nothing set",
			purchase: Purchase{},
			want:     0,
		},
		{
			name:     "reviewed price zero falls to CL",
			purchase: Purchase{ReviewedPriceCents: 0, CLValueCents: 4000},
			want:     4000,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveMarketValueCents(&tt.purchase)
			if got != tt.want {
				t.Errorf("ResolveMarketValueCents() = %d, want %d", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/campaigns/ -run TestResolveMarketValueCents -v`
Expected: FAIL — `ResolveMarketValueCents` undefined

- [ ] **Step 3: Write failing tests for `EvaluateHoldTriggers`**

Add to `internal/domain/campaigns/dh_push_safety_test.go`:

```go
func TestEvaluateHoldTriggers(t *testing.T) {
	defaultCfg := DHPushConfig{
		SwingPctThreshold:              20,
		SwingMinCents:                  5000,
		DisagreementPctThreshold:       25,
		UnreviewedChangePctThreshold:   15,
		UnreviewedChangeMinCents:       3000,
	}

	tests := []struct {
		name    string
		p       Purchase
		cfg     DHPushConfig
		wantHeld bool
		wantContains string
	}{
		{
			name: "first time push - never held",
			p:    Purchase{CLValueCents: 10000, DHInventoryID: 0},
			cfg:  defaultCfg,
			wantHeld: false,
		},
		{
			name: "small price change - not held",
			p: Purchase{
				CLValueCents:        10500,
				DHInventoryID:       1,
				DHListingPriceCents: 10000,
			},
			cfg:  defaultCfg,
			wantHeld: false,
		},
		{
			name: "large swing triggers hold",
			p: Purchase{
				CLValueCents:        15000,
				DHInventoryID:       1,
				DHListingPriceCents: 10000,
			},
			cfg:      defaultCfg,
			wantHeld: true,
			wantContains: "price_swing",
		},
		{
			name: "swing pct met but absolute below threshold - not held",
			p: Purchase{
				CLValueCents:        1500,
				DHInventoryID:       1,
				DHListingPriceCents: 1000,
			},
			cfg:      defaultCfg,
			wantHeld: false,
		},
		{
			name: "source disagreement - CL vs reviewed",
			p: Purchase{
				CLValueCents:        30000,
				ReviewedPriceCents:  45000,
				DHInventoryID:       1,
				DHListingPriceCents: 30000,
			},
			cfg:      defaultCfg,
			wantHeld: true,
			wantContains: "source_disagreement",
		},
		{
			name: "unreviewed CL change triggers hold",
			p: Purchase{
				CLValueCents:        12000,
				DHInventoryID:       1,
				DHListingPriceCents: 10000,
			},
			cfg:      defaultCfg,
			wantHeld: true,
			wantContains: "unreviewed_cl_change",
		},
		{
			name: "unreviewed small CL change - not held",
			p: Purchase{
				CLValueCents:        10200,
				DHInventoryID:       1,
				DHListingPriceCents: 10000,
			},
			cfg:      defaultCfg,
			wantHeld: false,
		},
		{
			name: "reviewed price set and stable - not held even with CL change",
			p: Purchase{
				CLValueCents:        12000,
				ReviewedPriceCents:  10000,
				DHInventoryID:       1,
				DHListingPriceCents: 10000,
			},
			cfg:      defaultCfg,
			wantHeld: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason := EvaluateHoldTriggers(&tt.p, tt.cfg)
			held := reason != ""
			if held != tt.wantHeld {
				t.Errorf("held = %v, want %v (reason=%q)", held, tt.wantHeld, reason)
			}
			if tt.wantContains != "" && !held {
				t.Errorf("expected hold with reason containing %q", tt.wantContains)
			}
			if tt.wantContains != "" && held {
				if !containsSubstring(reason, tt.wantContains) {
					t.Errorf("reason = %q, want it to contain %q", reason, tt.wantContains)
				}
			}
		})
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && contains(s, sub))
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Implement domain types and functions**

Add to `internal/domain/campaigns/types.go` after the existing `DHPushStatusManual` constant:

```go
	DHPushStatusHeld      DHPushStatus = "held"
```

Update `NeedsDHPush()` to also exclude `held`:

```go
func (p *Purchase) NeedsDHPush() bool {
	return p.DHInventoryID == 0 &&
		p.DHPushStatus != DHPushStatusPending &&
		p.DHPushStatus != DHPushStatusUnmatched &&
		p.DHPushStatus != DHPushStatusManual &&
		p.DHPushStatus != DHPushStatusHeld
}
```

Add `DHHoldReason` field to `Purchase` struct (after `DHPushStatus`):

```go
	DHHoldReason    string       `json:"dhHoldReason,omitempty"`        // Why a re-push was held
```

Create `internal/domain/campaigns/dh_push_safety.go`:

```go
package campaigns

import (
	"fmt"
	"math"
	"time"
)

// DHPushConfig holds admin-configurable thresholds for DH push safety gates.
type DHPushConfig struct {
	SwingPctThreshold            int       `json:"swingPctThreshold"`
	SwingMinCents                int       `json:"swingMinCents"`
	DisagreementPctThreshold     int       `json:"disagreementPctThreshold"`
	UnreviewedChangePctThreshold int       `json:"unreviewedChangePctThreshold"`
	UnreviewedChangeMinCents     int       `json:"unreviewedChangeMinCents"`
	UpdatedAt                    time.Time `json:"updatedAt"`
}

// DefaultDHPushConfig returns sensible defaults for push safety thresholds.
func DefaultDHPushConfig() DHPushConfig {
	return DHPushConfig{
		SwingPctThreshold:            20,
		SwingMinCents:                5000,
		DisagreementPctThreshold:     25,
		UnreviewedChangePctThreshold: 15,
		UnreviewedChangeMinCents:     3000,
	}
}

// ResolveMarketValueCents returns the best available price for DH push:
// reviewed price > CL value > 0.
func ResolveMarketValueCents(p *Purchase) int {
	if p.ReviewedPriceCents > 0 {
		return p.ReviewedPriceCents
	}
	if p.CLValueCents > 0 {
		return p.CLValueCents
	}
	return 0
}

// EvaluateHoldTriggers checks whether a re-push should be held for review.
// Returns empty string if the push should proceed, or a reason string if held.
// Only applies to re-pushes (DHInventoryID != 0).
func EvaluateHoldTriggers(p *Purchase, cfg DHPushConfig) string {
	if p.DHInventoryID == 0 {
		return ""
	}

	newValue := ResolveMarketValueCents(p)
	lastPushed := p.DHListingPriceCents
	if lastPushed == 0 || newValue == 0 {
		return ""
	}

	// Trigger 1: Price swing
	if reason := checkPriceSwing(newValue, lastPushed, cfg.SwingPctThreshold, cfg.SwingMinCents); reason != "" {
		return reason
	}

	// Trigger 2: Source disagreement
	if reason := checkSourceDisagreement(p, cfg.DisagreementPctThreshold); reason != "" {
		return reason
	}

	// Trigger 3: Unreviewed CL change
	if reason := checkUnreviewedCLChange(p, lastPushed, cfg.UnreviewedChangePctThreshold, cfg.UnreviewedChangeMinCents); reason != "" {
		return reason
	}

	return ""
}

func checkPriceSwing(newValue, lastPushed, pctThreshold, minCents int) string {
	delta := newValue - lastPushed
	absDelta := int(math.Abs(float64(delta)))
	pct := float64(delta) / float64(lastPushed) * 100

	if math.Abs(pct) > float64(pctThreshold) && absDelta > minCents {
		return fmt.Sprintf("price_swing:%+.0f%%", pct)
	}
	return ""
}

func checkSourceDisagreement(p *Purchase, pctThreshold int) string {
	prices := make(map[string]int)
	if p.CLValueCents > 0 {
		prices["cl"] = p.CLValueCents
	}
	if p.ReviewedPriceCents > 0 {
		prices["reviewed"] = p.ReviewedPriceCents
	}
	if p.LastSoldCents > 0 {
		prices["last_sold"] = p.LastSoldCents
	}

	if len(prices) < 2 {
		return ""
	}

	for nameA, a := range prices {
		for nameB, b := range prices {
			if nameA >= nameB {
				continue
			}
			maxVal := max(a, b)
			minVal := min(a, b)
			if maxVal > 0 {
				pct := float64(maxVal-minVal) / float64(maxVal) * 100
				if pct > float64(pctThreshold) {
					return fmt.Sprintf("source_disagreement:%s=%d,%s=%d", nameA, a, nameB, b)
				}
			}
		}
	}
	return ""
}

func checkUnreviewedCLChange(p *Purchase, lastPushed, pctThreshold, minCents int) string {
	if p.ReviewedPriceCents > 0 {
		return ""
	}
	delta := p.CLValueCents - lastPushed
	absDelta := int(math.Abs(float64(delta)))
	if lastPushed == 0 {
		return ""
	}
	pct := float64(delta) / float64(lastPushed) * 100

	if math.Abs(pct) > float64(pctThreshold) && absDelta > minCents {
		return fmt.Sprintf("unreviewed_cl_change:%+.0f%%", pct)
	}
	return ""
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/domain/campaigns/ -run "TestResolveMarketValueCents|TestEvaluateHoldTriggers" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/domain/campaigns/types.go \
        internal/domain/campaigns/dh_push_safety.go \
        internal/domain/campaigns/dh_push_safety_test.go
git commit -m "feat: add ResolveMarketValueCents and hold trigger evaluation"
```

---

### Task 3: Repository — DHPushConfig CRUD and hold reason persistence

**Files:**
- Create: `internal/adapters/storage/sqlite/dh_push_config_repository.go`
- Modify: `internal/adapters/storage/sqlite/purchase_scan_helpers.go` (add `dh_hold_reason` to scan)
- Modify: `internal/adapters/storage/sqlite/purchases_repository_dh.go` (add `UpdatePurchaseDHHoldReason`)
- Modify: `internal/domain/campaigns/repository.go` (add new repo methods)
- Modify: `internal/testutil/mocks/campaign_repository.go` (add mock methods)

- [ ] **Step 1: Add repository interface methods**

Add to `internal/domain/campaigns/repository.go` in the DH section (after `UpdatePurchaseDHCandidates`):

```go
	UpdatePurchaseDHHoldReason(ctx context.Context, id string, reason string) error
	GetDHPushConfig(ctx context.Context) (*DHPushConfig, error)
	SaveDHPushConfig(ctx context.Context, cfg *DHPushConfig) error
```

- [ ] **Step 2: Create dh_push_config repository**

Create `internal/adapters/storage/sqlite/dh_push_config_repository.go`:

```go
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

func (r *CampaignsRepository) GetDHPushConfig(ctx context.Context) (*campaigns.DHPushConfig, error) {
	query := `SELECT swing_pct_threshold, swing_min_cents, disagreement_pct_threshold,
		unreviewed_change_pct_threshold, unreviewed_change_min_cents, updated_at
		FROM dh_push_config WHERE id = 1`
	var cfg campaigns.DHPushConfig
	err := r.db.QueryRowContext(ctx, query).Scan(
		&cfg.SwingPctThreshold, &cfg.SwingMinCents, &cfg.DisagreementPctThreshold,
		&cfg.UnreviewedChangePctThreshold, &cfg.UnreviewedChangeMinCents, &cfg.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		d := campaigns.DefaultDHPushConfig()
		return &d, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (r *CampaignsRepository) SaveDHPushConfig(ctx context.Context, cfg *campaigns.DHPushConfig) error {
	cfg.UpdatedAt = time.Now()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO dh_push_config (id, swing_pct_threshold, swing_min_cents, disagreement_pct_threshold,
			unreviewed_change_pct_threshold, unreviewed_change_min_cents, updated_at) VALUES (1, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			swing_pct_threshold = ?, swing_min_cents = ?, disagreement_pct_threshold = ?,
			unreviewed_change_pct_threshold = ?, unreviewed_change_min_cents = ?, updated_at = ?`,
		cfg.SwingPctThreshold, cfg.SwingMinCents, cfg.DisagreementPctThreshold,
		cfg.UnreviewedChangePctThreshold, cfg.UnreviewedChangeMinCents, cfg.UpdatedAt,
		cfg.SwingPctThreshold, cfg.SwingMinCents, cfg.DisagreementPctThreshold,
		cfg.UnreviewedChangePctThreshold, cfg.UnreviewedChangeMinCents, cfg.UpdatedAt,
	)
	return err
}
```

- [ ] **Step 3: Add `UpdatePurchaseDHHoldReason` to DH purchases repository**

Add to `internal/adapters/storage/sqlite/purchases_repository_dh.go`:

```go
func (r *CampaignsRepository) UpdatePurchaseDHHoldReason(ctx context.Context, id string, reason string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET dh_hold_reason = ?, updated_at = ? WHERE id = ?`,
		reason, time.Now(), id,
	)
	return err
}
```

- [ ] **Step 4: Add `dh_hold_reason` to purchase scan helpers**

In `internal/adapters/storage/sqlite/purchase_scan_helpers.go`, add `dh_hold_reason` to the column list (after `dh_push_status`) and add `&p.DHHoldReason` to the corresponding `Scan` call.

- [ ] **Step 5: Add mock methods**

Add to `internal/testutil/mocks/campaign_repository.go`:

```go
	UpdatePurchaseDHHoldReasonFn func(ctx context.Context, id string, reason string) error
	GetDHPushConfigFn            func(ctx context.Context) (*campaigns.DHPushConfig, error)
	SaveDHPushConfigFn           func(ctx context.Context, cfg *campaigns.DHPushConfig) error
```

And the corresponding method implementations:

```go
func (m *MockCampaignRepository) UpdatePurchaseDHHoldReason(ctx context.Context, id string, reason string) error {
	if m.UpdatePurchaseDHHoldReasonFn != nil {
		return m.UpdatePurchaseDHHoldReasonFn(ctx, id, reason)
	}
	return nil
}

func (m *MockCampaignRepository) GetDHPushConfig(ctx context.Context) (*campaigns.DHPushConfig, error) {
	if m.GetDHPushConfigFn != nil {
		return m.GetDHPushConfigFn(ctx)
	}
	d := campaigns.DefaultDHPushConfig()
	return &d, nil
}

func (m *MockCampaignRepository) SaveDHPushConfig(ctx context.Context, cfg *campaigns.DHPushConfig) error {
	if m.SaveDHPushConfigFn != nil {
		return m.SaveDHPushConfigFn(ctx, cfg)
	}
	return nil
}
```

Also add the same stubs to the in-package `mockRepo` in `internal/domain/campaigns/mock_repo_test.go`.

- [ ] **Step 6: Run tests**

Run: `go test ./internal/adapters/storage/sqlite/... ./internal/domain/campaigns/... -count=1`
Expected: PASS (compilation succeeds, existing tests still pass)

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/storage/sqlite/dh_push_config_repository.go \
        internal/adapters/storage/sqlite/purchases_repository_dh.go \
        internal/adapters/storage/sqlite/purchase_scan_helpers.go \
        internal/domain/campaigns/repository.go \
        internal/testutil/mocks/campaign_repository.go \
        internal/domain/campaigns/mock_repo_test.go
git commit -m "feat: add DHPushConfig repository and hold reason persistence"
```

---

### Task 4: Push scheduler — Integrate hold gate and price resolution

**Files:**
- Modify: `internal/adapters/scheduler/dh_push.go`
- Create: `internal/adapters/scheduler/dh_push_test.go` (or add to existing)

- [ ] **Step 1: Write failing test for hold gate in scheduler**

Create `internal/adapters/scheduler/dh_push_hold_test.go`:

```go
package scheduler

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

type mockStatusUpdater struct {
	statuses map[string]string
}

func (m *mockStatusUpdater) UpdatePurchaseDHPushStatus(_ context.Context, id string, status string) error {
	m.statuses[id] = status
	return nil
}

type mockConfigLoader struct {
	cfg *campaigns.DHPushConfig
}

func (m *mockConfigLoader) GetDHPushConfig(_ context.Context) (*campaigns.DHPushConfig, error) {
	if m.cfg != nil {
		return m.cfg, nil
	}
	d := campaigns.DefaultDHPushConfig()
	return &d, nil
}

type mockHoldReasonUpdater struct {
	reasons map[string]string
}

func (m *mockHoldReasonUpdater) UpdatePurchaseDHHoldReason(_ context.Context, id string, reason string) error {
	m.reasons[id] = reason
	return nil
}

func TestProcessPurchase_HoldsOnLargeSwing(t *testing.T) {
	statusUpdater := &mockStatusUpdater{statuses: make(map[string]string)}
	holdUpdater := &mockHoldReasonUpdater{reasons: make(map[string]string)}
	configLoader := &mockConfigLoader{}

	s := &DHPushScheduler{
		statusUpdater:     statusUpdater,
		holdReasonUpdater: holdUpdater,
		configLoader:      configLoader,
		logger:            observability.NewNoopLogger(),
	}

	p := campaigns.Purchase{
		ID:                  "p1",
		CertNumber:          "12345",
		CLValueCents:        15000,
		DHInventoryID:       1,
		DHCardID:            100,
		DHListingPriceCents: 10000,
		GradeValue:          9,
	}

	result := s.processPurchase(context.Background(), p, map[string]string{})
	if result != processHeld {
		t.Errorf("expected processHeld, got %v", result)
	}
	if statusUpdater.statuses["p1"] != campaigns.DHPushStatusHeld {
		t.Errorf("status = %q, want %q", statusUpdater.statuses["p1"], campaigns.DHPushStatusHeld)
	}
	if holdUpdater.reasons["p1"] == "" {
		t.Error("expected hold reason to be set")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/scheduler/ -run TestProcessPurchase_HoldsOnLargeSwing -v`
Expected: FAIL — `processHeld`, `holdReasonUpdater`, `configLoader` undefined

- [ ] **Step 3: Add new interfaces and fields to DHPushScheduler**

Add new interfaces to `internal/adapters/scheduler/dh_push.go`:

```go
// DHPushConfigLoader loads DH push safety config.
type DHPushConfigLoader interface {
	GetDHPushConfig(ctx context.Context) (*campaigns.DHPushConfig, error)
}

// DHPushHoldReasonUpdater persists hold reasons on purchases.
type DHPushHoldReasonUpdater interface {
	UpdatePurchaseDHHoldReason(ctx context.Context, id string, reason string) error
}
```

Add `processHeld` to `processResult`:

```go
const (
	processMatched processResult = iota
	processUnmatched
	processSkipped
	processHeld
)
```

Add fields to `DHPushScheduler`:

```go
	configLoader      DHPushConfigLoader
	holdReasonUpdater DHPushHoldReasonUpdater
```

Add functional options:

```go
func WithDHPushConfigLoader(loader DHPushConfigLoader) DHPushOption {
	return func(s *DHPushScheduler) { s.configLoader = loader }
}

func WithDHPushHoldReasonUpdater(updater DHPushHoldReasonUpdater) DHPushOption {
	return func(s *DHPushScheduler) { s.holdReasonUpdater = updater }
}
```

- [ ] **Step 4: Update `push()` to load config and track held count**

In the `push()` method, load config at the start and pass to `processPurchase`:

```go
func (s *DHPushScheduler) push(ctx context.Context) {
	// ... existing pending fetch ...

	var pushCfg campaigns.DHPushConfig
	if s.configLoader != nil {
		loaded, err := s.configLoader.GetDHPushConfig(ctx)
		if err != nil {
			s.logger.Warn(ctx, "dh push: failed to load push config, using defaults", observability.Err(err))
			pushCfg = campaigns.DefaultDHPushConfig()
		} else {
			pushCfg = *loaded
		}
	} else {
		pushCfg = campaigns.DefaultDHPushConfig()
	}

	// ... existing mappedSet load ...

	matched, unmatched, skipped, held := 0, 0, 0, 0

	for _, p := range pending {
		switch s.processPurchase(ctx, p, mappedSet, pushCfg) {
		case processMatched:
			matched++
		case processUnmatched:
			unmatched++
		case processSkipped:
			skipped++
		case processHeld:
			held++
		}
	}

	s.logger.Info(ctx, "dh push completed",
		observability.Int("total", len(pending)),
		observability.Int("matched", matched),
		observability.Int("unmatched", unmatched),
		observability.Int("skipped", skipped),
		observability.Int("held", held),
	)
}
```

- [ ] **Step 5: Update `processPurchase` with hold gate and price resolution**

Update the method signature to accept `pushCfg campaigns.DHPushConfig`.

Replace the existing price resolution block (lines ~295-303) with:

```go
	marketValue := campaigns.ResolveMarketValueCents(&p)
	if marketValue == 0 {
		s.logger.Debug(ctx, "dh push: no market value yet, leaving as pending",
			observability.String("purchaseID", p.ID),
			observability.String("cert", p.CertNumber))
		return processSkipped
	}

	// Hold gate: evaluate re-push safety triggers
	if holdReason := campaigns.EvaluateHoldTriggers(&p, pushCfg); holdReason != "" {
		s.logger.Info(ctx, "dh push: holding re-push for review",
			observability.String("purchaseID", p.ID),
			observability.String("cert", p.CertNumber),
			observability.String("reason", holdReason))
		if updateErr := s.statusUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, campaigns.DHPushStatusHeld); updateErr != nil {
			s.logger.Warn(ctx, "dh push: failed to set held status",
				observability.String("purchaseID", p.ID), observability.Err(updateErr))
		}
		if s.holdReasonUpdater != nil {
			if updateErr := s.holdReasonUpdater.UpdatePurchaseDHHoldReason(ctx, p.ID, holdReason); updateErr != nil {
				s.logger.Warn(ctx, "dh push: failed to set hold reason",
					observability.String("purchaseID", p.ID), observability.Err(updateErr))
			}
		}
		return processHeld
	}

	item := dh.InventoryItem{
		DHCardID:         dhCardID,
		CertNumber:       p.CertNumber,
		GradingCompany:   dh.GraderPSA,
		Grade:            p.GradeValue,
		CostBasisCents:   p.BuyCostCents,
		MarketValueCents: dh.IntPtr(marketValue),
		Status:           dh.InventoryStatusInStock,
	}
```

Note: `CostBasisCents` now uses `p.BuyCostCents` (actual cost) instead of `p.CLValueCents`.

- [ ] **Step 6: Run tests**

Run: `go test ./internal/adapters/scheduler/ -run TestProcessPurchase -v`
Expected: PASS

Run: `go test ./... -count=1` to check nothing else broke
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/scheduler/dh_push.go \
        internal/adapters/scheduler/dh_push_hold_test.go
git commit -m "feat: integrate hold gate and price resolution into DH push scheduler"
```

---

### Task 5: Update all handler push sites to use `ResolveMarketValueCents`

**Files:**
- Modify: `internal/adapters/httpserver/handlers/dh_match_handler.go`
- Modify: `internal/adapters/httpserver/handlers/campaigns_dh_listing.go`
- Modify: `internal/adapters/httpserver/handlers/dh_select_match_handler.go`
- Modify: `internal/adapters/httpserver/handlers/dh_fix_match_handler.go`

- [ ] **Step 1: Update bulk match handler**

In `internal/adapters/httpserver/handlers/dh_match_handler.go`, replace:

```go
		CostBasisCents:   p.CLValueCents,
		MarketValueCents: dh.IntPtr(p.CLValueCents),
```

with:

```go
		CostBasisCents:   p.BuyCostCents,
		MarketValueCents: dh.IntPtr(campaigns.ResolveMarketValueCents(&p)),
```

Also update the `CLValueCents == 0` guard to use `ResolveMarketValueCents`:

```go
		if p.CertNumber == "" || p.DHInventoryID != 0 || campaigns.ResolveMarketValueCents(&p) == 0 {
```

Add import for `campaigns` package if not already present.

- [ ] **Step 2: Update inline match handler**

In `internal/adapters/httpserver/handlers/campaigns_dh_listing.go`, replace the `CLValueCents == 0` guard and price assignment similarly:

```go
	marketValue := campaigns.ResolveMarketValueCents(&p)
	if marketValue == 0 {
		return 0
	}

	item := dh.InventoryItem{
		DHCardID:         dhCardID,
		CertNumber:       p.CertNumber,
		GradingCompany:   dh.GraderPSA,
		Grade:            p.GradeValue,
		CostBasisCents:   p.BuyCostCents,
		MarketValueCents: dh.IntPtr(marketValue),
		Status:           dh.InventoryStatusInStock,
	}
```

- [ ] **Step 3: Update select match handler**

In `internal/adapters/httpserver/handlers/dh_select_match_handler.go`, same pattern:

```go
	marketValue := campaigns.ResolveMarketValueCents(purchase)
	if marketValue == 0 {
		writeError(w, http.StatusBadRequest, "purchase has no market value yet")
		return
	}

	item := dh.InventoryItem{
		DHCardID:         req.DHCardID,
		CertNumber:       purchase.CertNumber,
		GradingCompany:   dh.GraderPSA,
		Grade:            purchase.GradeValue,
		CostBasisCents:   purchase.BuyCostCents,
		MarketValueCents: dh.IntPtr(marketValue),
		Status:           dh.InventoryStatusInStock,
	}
```

- [ ] **Step 4: Update fix match handler**

In `internal/adapters/httpserver/handlers/dh_fix_match_handler.go`, same pattern:

```go
	marketValue := campaigns.ResolveMarketValueCents(purchase)
	if marketValue == 0 {
		writeError(w, http.StatusBadRequest, "purchase has no market value yet")
		return
	}

	item := dh.InventoryItem{
		DHCardID:         dhCardID,
		CertNumber:       purchase.CertNumber,
		GradingCompany:   dh.GraderPSA,
		Grade:            purchase.GradeValue,
		CostBasisCents:   purchase.BuyCostCents,
		MarketValueCents: dh.IntPtr(marketValue),
		Status:           dh.InventoryStatusInStock,
	}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/adapters/httpserver/... -count=1`
Expected: PASS

Run: `go test ./... -count=1`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/httpserver/handlers/dh_match_handler.go \
        internal/adapters/httpserver/handlers/campaigns_dh_listing.go \
        internal/adapters/httpserver/handlers/dh_select_match_handler.go \
        internal/adapters/httpserver/handlers/dh_fix_match_handler.go
git commit -m "feat: use ResolveMarketValueCents and BuyCostCents in all DH push sites"
```

---

### Task 6: API endpoints — Approve held push + config CRUD

**Files:**
- Modify: `internal/adapters/httpserver/handlers/dh_match_handler.go` (or create new handler file)
- Modify: `internal/adapters/httpserver/routes.go`
- Modify: `internal/domain/campaigns/service_interfaces.go`
- Modify: `internal/domain/campaigns/service_finance.go` (or appropriate service file)

- [ ] **Step 1: Add service interface methods**

Add to the appropriate service interface in `internal/domain/campaigns/service_interfaces.go` (e.g., `FinanceService` or a new section in `PricingService`):

```go
	ApproveDHPush(ctx context.Context, purchaseID string) error
	GetDHPushConfig(ctx context.Context) (*DHPushConfig, error)
	SaveDHPushConfig(ctx context.Context, cfg *DHPushConfig) error
```

- [ ] **Step 2: Implement service methods**

Add to the appropriate service file (could be `internal/domain/campaigns/service_finance.go` or a new `service_dh.go` if appropriate):

```go
func (s *service) ApproveDHPush(ctx context.Context, purchaseID string) error {
	p, err := s.repo.GetPurchase(ctx, purchaseID)
	if err != nil {
		return err
	}
	if p.DHPushStatus != DHPushStatusHeld {
		return errors.NewAppError(ErrCodeCampaignValidation, "purchase is not in held status")
	}
	if err := s.repo.UpdatePurchaseDHHoldReason(ctx, purchaseID, ""); err != nil {
		return err
	}
	return s.repo.UpdatePurchaseDHPushStatus(ctx, purchaseID, DHPushStatusPending)
}

func (s *service) GetDHPushConfig(ctx context.Context) (*DHPushConfig, error) {
	return s.repo.GetDHPushConfig(ctx)
}

func (s *service) SaveDHPushConfig(ctx context.Context, cfg *DHPushConfig) error {
	return s.repo.SaveDHPushConfig(ctx, cfg)
}
```

- [ ] **Step 3: Add handler methods**

Add to the DH handler (e.g., in `internal/adapters/httpserver/handlers/dh_match_handler.go` or a new `dh_safety_handler.go`):

```go
// HandleApproveDHPush handles POST /api/dh/approve/{purchaseId}.
func (h *DHHandler) HandleApproveDHPush(w http.ResponseWriter, r *http.Request) {
	purchaseID := r.PathValue("purchaseId")
	if purchaseID == "" {
		writeError(w, http.StatusBadRequest, "missing purchaseId")
		return
	}
	if err := h.service.ApproveDHPush(r.Context(), purchaseID); err != nil {
		h.logger.Error(r.Context(), "approve dh push failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to approve push")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

// HandleGetDHPushConfig handles GET /api/admin/dh-push-config.
func (h *DHHandler) HandleGetDHPushConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.service.GetDHPushConfig(r.Context())
	if err != nil {
		h.logger.Error(r.Context(), "get dh push config failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to get config")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// HandleSaveDHPushConfig handles PUT /api/admin/dh-push-config.
func (h *DHHandler) HandleSaveDHPushConfig(w http.ResponseWriter, r *http.Request) {
	var cfg campaigns.DHPushConfig
	if !decodeBody(w, r, &cfg) {
		return
	}
	if err := h.service.SaveDHPushConfig(r.Context(), &cfg); err != nil {
		h.logger.Error(r.Context(), "save dh push config failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}
```

- [ ] **Step 4: Register routes**

Add to `internal/adapters/httpserver/routes.go` in the DH section:

```go
	mux.Handle("POST /api/dh/approve/{purchaseId}", authRoute(rt.dhHandler.HandleApproveDHPush))
	mux.Handle("GET /api/admin/dh-push-config", authRoute(rt.dhHandler.HandleGetDHPushConfig))
	mux.Handle("PUT /api/admin/dh-push-config", authRoute(rt.dhHandler.HandleSaveDHPushConfig))
```

- [ ] **Step 5: Add mock service methods**

Add to `internal/testutil/mocks/campaign_service.go`:

```go
	ApproveDHPushFn    func(ctx context.Context, purchaseID string) error
	GetDHPushConfigFn  func(ctx context.Context) (*campaigns.DHPushConfig, error)
	SaveDHPushConfigFn func(ctx context.Context, cfg *campaigns.DHPushConfig) error
```

And the corresponding method stubs.

- [ ] **Step 6: Run tests**

Run: `go test ./... -count=1`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/httpserver/handlers/ \
        internal/adapters/httpserver/routes.go \
        internal/domain/campaigns/service_interfaces.go \
        internal/domain/campaigns/service_finance.go \
        internal/testutil/mocks/campaign_service.go
git commit -m "feat: add DH push approve and config CRUD endpoints"
```

---

### Task 7: Wire hold gate into scheduler builder

**Files:**
- Modify: `internal/adapters/scheduler/builder.go`
- Modify: `cmd/slabledger/init.go` (if scheduler wiring happens there)

- [ ] **Step 1: Pass config loader and hold reason updater to DH push scheduler**

In the scheduler builder / wiring code, inject the new optional dependencies:

```go
WithDHPushConfigLoader(campaignsRepo),
WithDHPushHoldReasonUpdater(campaignsRepo),
```

The `CampaignsRepository` already satisfies both interfaces via the methods added in Task 3.

- [ ] **Step 2: Run full test suite**

Run: `go test ./... -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/adapters/scheduler/builder.go cmd/slabledger/init.go
git commit -m "feat: wire hold gate dependencies into DH push scheduler"
```

---

### Task 8: Frontend — Add `dhHoldReason` to types and API client

**Files:**
- Modify: `web/src/types/campaigns/core.ts`
- Modify: `web/src/js/api/admin.ts`
- Modify: `web/src/types/apiStatus.ts` (add DHPushConfig type)

- [ ] **Step 1: Add `dhHoldReason` to Purchase type**

In `web/src/types/campaigns/core.ts`, add after `dhPushStatus`:

```typescript
  dhHoldReason?: string;
```

- [ ] **Step 2: Add DHPushConfig type**

In `web/src/types/apiStatus.ts`, add:

```typescript
export interface DHPushConfig {
  swingPctThreshold: number;
  swingMinCents: number;
  disagreementPctThreshold: number;
  unreviewedChangePctThreshold: number;
  unreviewedChangeMinCents: number;
  updatedAt: string;
}
```

- [ ] **Step 3: Add API client methods**

In `web/src/js/api/admin.ts`, add to the declaration merging section:

```typescript
    approveDHPush(purchaseId: string): Promise<{ status: string }>;
    getDHPushConfig(): Promise<DHPushConfig>;
    saveDHPushConfig(config: DHPushConfig): Promise<{ status: string }>;
```

And the prototype implementations:

```typescript
proto.approveDHPush = async function (this: APIClient, purchaseId: string): Promise<{ status: string }> {
  return this.post<{ status: string }>(`/dh/approve/${purchaseId}`);
};

proto.getDHPushConfig = async function (this: APIClient): Promise<DHPushConfig> {
  return this.get<DHPushConfig>('/admin/dh-push-config');
};

proto.saveDHPushConfig = async function (this: APIClient, config: DHPushConfig): Promise<{ status: string }> {
  return this.put<{ status: string }>('/admin/dh-push-config', config);
};
```

Import `DHPushConfig` from the types file.

- [ ] **Step 4: Run lint/typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: zero errors

- [ ] **Step 5: Commit**

```bash
git add web/src/types/campaigns/core.ts \
        web/src/types/apiStatus.ts \
        web/src/js/api/admin.ts
git commit -m "feat: add DH push safety types and API client methods"
```

---

### Task 9: Frontend — Rename "Exceptions" to "Needs Attention" and add held items

**Files:**
- Modify: `web/src/react/pages/campaign-detail/inventory/inventoryCalcs.ts`
- Modify: `web/src/react/pages/campaign-detail/inventory/useInventoryState.ts`
- Modify: `web/src/react/pages/campaign-detail/InventoryTab.tsx`
- Modify: `web/src/react/pages/campaign-detail/inventory/utils.ts`

- [ ] **Step 1: Update FilterTab and tab counts**

In `web/src/react/pages/campaign-detail/inventory/inventoryCalcs.ts`:

Rename `exceptions` to `needs_attention` in the `FilterTab` type:

```typescript
export type FilterTab = 'needs_attention' | 'sell_sheet' | 'all' | 'card_show';
```

Update `TabCounts`:

```typescript
export interface TabCounts {
  needs_attention: number;
  card_show: number;
  all: number;
}
```

Update `computeInventoryMeta` to also count held items:

```typescript
export function computeInventoryMeta(items: AgingItem[]): InventoryMeta {
  const stats: ReviewStats = { total: items.length, needsReview: 0, reviewed: 0, flagged: 0 };
  const counts: TabCounts = { needs_attention: 0, card_show: 0, all: items.length };
  let totalCost = 0;
  let totalMarket = 0;
  for (const item of items) {
    if (item.hasOpenFlag) stats.flagged++;
    if (item.purchase.reviewedAt) stats.reviewed++;
    else stats.needsReview++;

    const status = getReviewStatus(item);
    if ((EXCEPTION_STATUSES as readonly string[]).includes(status) || isDHHeld(item)) {
      counts.needs_attention++;
    }
    if (isCardShowCandidate(item)) counts.card_show++;

    totalCost += costBasis(item.purchase);
    if (item.currentMarket) totalMarket += bestPrice(item.currentMarket);
  }
  return {
    reviewStats: stats,
    tabCounts: counts,
    summary: { totalCost, totalMarket, totalPL: totalMarket > 0 ? totalMarket - totalCost : 0 },
  };
}
```

Add `isDHHeld` helper:

```typescript
export function isDHHeld(item: AgingItem): boolean {
  return item.purchase.dhPushStatus === 'held';
}
```

Update `filterAndSortItems` to use `needs_attention`:

```typescript
      if (filterTab === 'needs_attention') {
        return (EXCEPTION_STATUSES as readonly string[]).includes(getReviewStatus(i)) || isDHHeld(i);
      }
```

- [ ] **Step 2: Update tab label in InventoryTab.tsx**

In `web/src/react/pages/campaign-detail/InventoryTab.tsx`, rename the tab:

```typescript
{ key: 'needs_attention' as const, label: 'Needs Attention', color: 'var(--warning)' },
```

- [ ] **Step 3: Update default filter tab in useInventoryState.ts**

```typescript
const [filterTab, setFilterTab] = useState<FilterTab>('needs_attention');
```

- [ ] **Step 4: Add held-item badge to utils.ts**

In `web/src/react/pages/campaign-detail/inventory/utils.ts`, update `statusBadge`:

```typescript
export function statusBadge(item: AgingItem): { label: string; color: string } {
  // DH hold takes priority
  if (item.purchase.dhPushStatus === 'held') {
    return { label: 'DH Held', color: 'var(--warning)' };
  }
  const status = getReviewStatus(item);
  // ... rest unchanged
}
```

- [ ] **Step 5: Run lint/typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: zero errors

- [ ] **Step 6: Commit**

```bash
git add web/src/react/pages/campaign-detail/inventory/inventoryCalcs.ts \
        web/src/react/pages/campaign-detail/inventory/useInventoryState.ts \
        web/src/react/pages/campaign-detail/InventoryTab.tsx \
        web/src/react/pages/campaign-detail/inventory/utils.ts
git commit -m "feat: rename Exceptions to Needs Attention and surface held DH pushes"
```

---

### Task 10: Frontend — Held item approve/adjust actions in ExpandedDetail

**Files:**
- Modify: `web/src/react/pages/campaign-detail/inventory/ExpandedDetail.tsx`

- [ ] **Step 1: Add approve button and hold reason to expanded detail**

In the `ExpandedDetail` component, add a section that shows when `purchase.dhPushStatus === 'held'`:

```tsx
{item.purchase.dhPushStatus === 'held' && (
  <div className="mt-3 p-3 rounded-lg bg-amber-500/10 border border-amber-500/30">
    <div className="flex items-center justify-between">
      <div>
        <span className="text-xs font-semibold text-amber-400 uppercase">DH Push Held</span>
        <p className="text-sm text-[var(--text-muted)] mt-0.5">
          {formatHoldReason(item.purchase.dhHoldReason ?? '')}
        </p>
      </div>
      <div className="flex gap-2">
        <Button size="sm" variant="secondary" onClick={() => onSetPrice()}>
          Adjust Price
        </Button>
        <Button size="sm" onClick={() => onApproveDHPush(item.purchase.id)}>
          Approve Push
        </Button>
      </div>
    </div>
  </div>
)}
```

Add a helper function:

```typescript
function formatHoldReason(reason: string): string {
  if (reason.startsWith('price_swing:')) return `Price swing: ${reason.split(':')[1]}`;
  if (reason.startsWith('source_disagreement:')) return `Source disagreement: ${reason.split(':')[1]}`;
  if (reason.startsWith('unreviewed_cl_change:')) return `Unreviewed CL change: ${reason.split(':')[1]}`;
  return reason || 'Unknown reason';
}
```

- [ ] **Step 2: Thread `onApproveDHPush` callback through**

Add `onApproveDHPush` to ExpandedDetail's props and wire it up from `useInventoryState`:

In `useInventoryState.ts`, add:

```typescript
const handleApproveDHPush = useCallback(async (purchaseId: string) => {
  try {
    await api.approveDHPush(purchaseId);
    toast.success('DH push approved — will push on next cycle');
    invalidateInventory();
  } catch (err) {
    toast.error(getErrorMessage(err, 'Failed to approve DH push'));
  }
}, [toast, invalidateInventory]);
```

Return it from the hook and pass it through InventoryTab → ExpandedDetail.

- [ ] **Step 3: Run lint/typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: zero errors

- [ ] **Step 4: Commit**

```bash
git add web/src/react/pages/campaign-detail/inventory/ExpandedDetail.tsx \
        web/src/react/pages/campaign-detail/inventory/useInventoryState.ts \
        web/src/react/pages/campaign-detail/InventoryTab.tsx
git commit -m "feat: add approve/adjust actions for held DH pushes in expanded detail"
```

---

### Task 11: Frontend — Admin DH Push Config card

**Files:**
- Create: `web/src/react/pages/admin/DHPushConfigCard.tsx`
- Modify: `web/src/react/pages/admin/IntegrationsTab.tsx`

- [ ] **Step 1: Create DHPushConfigCard component**

Create `web/src/react/pages/admin/DHPushConfigCard.tsx`:

```tsx
import { useState, useEffect } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../js/api';
import { CardShell } from '../../ui/CardShell';
import Button from '../../ui/Button';
import { useToast } from '../../contexts/ToastContext';
import type { DHPushConfig } from '../../../types/apiStatus';
import { formatCents } from '../../utils/formatters';

function ConfigField({ label, value, onChange, suffix }: {
  label: string;
  value: number;
  onChange: (v: number) => void;
  suffix: string;
}) {
  return (
    <div>
      <label className="block text-xs text-[var(--text-muted)] mb-1">{label}</label>
      <div className="flex items-center gap-2">
        <input
          type="number"
          min={0}
          value={value}
          onChange={(e) => onChange(parseInt(e.target.value, 10) || 0)}
          className="w-24 px-2 py-1.5 text-sm rounded-lg bg-[var(--surface-0)] border border-[var(--surface-2)] text-[var(--text)]"
        />
        <span className="text-xs text-[var(--text-muted)]">{suffix}</span>
      </div>
    </div>
  );
}

export function DHPushConfigCard() {
  const toast = useToast();
  const queryClient = useQueryClient();

  const { data: config, isLoading } = useQuery({
    queryKey: ['admin', 'dh-push-config'],
    queryFn: () => api.getDHPushConfig(),
  });

  const [form, setForm] = useState<DHPushConfig | null>(null);

  useEffect(() => {
    if (config && !form) setForm(config);
  }, [config, form]);

  const saveMutation = useMutation({
    mutationFn: (cfg: DHPushConfig) => api.saveDHPushConfig(cfg),
    onSuccess: () => {
      toast.success('DH push config saved');
      queryClient.invalidateQueries({ queryKey: ['admin', 'dh-push-config'] });
    },
    onError: () => toast.error('Failed to save config'),
  });

  if (isLoading || !form) {
    return <CardShell padding="lg"><p className="text-[var(--text-muted)]">Loading...</p></CardShell>;
  }

  return (
    <CardShell padding="lg">
      <h4 className="text-sm font-semibold text-[var(--text)] mb-3">Push Safety Thresholds</h4>
      <p className="text-xs text-[var(--text-muted)] mb-4">
        Re-pushes that exceed these thresholds are held for manual approval.
      </p>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <ConfigField
          label="Price Swing %"
          value={form.swingPctThreshold}
          onChange={(v) => setForm({ ...form, swingPctThreshold: v })}
          suffix="%"
        />
        <ConfigField
          label="Price Swing Min"
          value={form.swingMinCents}
          onChange={(v) => setForm({ ...form, swingMinCents: v })}
          suffix={`(${formatCents(form.swingMinCents)})`}
        />
        <ConfigField
          label="Source Disagreement %"
          value={form.disagreementPctThreshold}
          onChange={(v) => setForm({ ...form, disagreementPctThreshold: v })}
          suffix="%"
        />
        <ConfigField
          label="Unreviewed CL Change %"
          value={form.unreviewedChangePctThreshold}
          onChange={(v) => setForm({ ...form, unreviewedChangePctThreshold: v })}
          suffix="%"
        />
        <ConfigField
          label="Unreviewed CL Change Min"
          value={form.unreviewedChangeMinCents}
          onChange={(v) => setForm({ ...form, unreviewedChangeMinCents: v })}
          suffix={`(${formatCents(form.unreviewedChangeMinCents)})`}
        />
      </div>
      <div className="mt-4">
        <Button
          size="sm"
          onClick={() => saveMutation.mutate(form)}
          loading={saveMutation.isPending}
        >
          Save
        </Button>
      </div>
    </CardShell>
  );
}
```

- [ ] **Step 2: Add to IntegrationsTab**

In `web/src/react/pages/admin/IntegrationsTab.tsx`, import and add:

```tsx
import { DHPushConfigCard } from './DHPushConfigCard';

// Inside the DH section, after <DHTab />:
<div className="mt-4">
  <DHPushConfigCard />
</div>
```

- [ ] **Step 3: Run lint/typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: zero errors

- [ ] **Step 4: Commit**

```bash
git add web/src/react/pages/admin/DHPushConfigCard.tsx \
        web/src/react/pages/admin/IntegrationsTab.tsx
git commit -m "feat: add DH push safety config card to admin page"
```

---

### Task 12: Update documentation

**Files:**
- Modify: `docs/DH_INVENTORY.md`

- [ ] **Step 1: Update DH_INVENTORY.md**

Add `held` to the pipeline states table:

```markdown
| `held` | Re-push blocked by safety gate | User approves or adjusts price |
```

Add a new section after "Re-Push on Price Change":

```markdown
## Push Safety Gates

Re-pushes are evaluated against configurable safety thresholds before proceeding. If any threshold is exceeded, the push is held for manual review.

### Price Selection

All push sites use `ResolveMarketValueCents()` which returns:
1. `ReviewedPriceCents` (if > 0)
2. `CLValueCents` (if > 0)
3. `0` (push skipped)

`cost_basis_cents` is set to `BuyCostCents` (actual purchase cost).

### Hold Triggers

| Trigger | Condition | Default Threshold |
|---------|-----------|-------------------|
| Price swing | New market value vs last pushed value | >20% AND >$50 |
| Source disagreement | Any two of (CL, reviewed, last sold) differ | >25% |
| Unreviewed CL change | No reviewed price + CL change from last push | >15% AND >$30 |

First-time pushes are never held.

### Configuration

Thresholds are stored in `dh_push_config` table and editable from Admin > Integrations > DH.

### Approval

Held items appear in the inventory "Needs Attention" tab. Users can:
- **Approve**: re-queues as `pending` for the next scheduler cycle
- **Adjust**: opens price dialog, then re-queues
```

Update the "All sites send CLValueCents" line to:

```markdown
All sites use `ResolveMarketValueCents()` (reviewed price > CL value) as `market_value_cents` and `BuyCostCents` as `cost_basis_cents`.
```

- [ ] **Step 2: Commit**

```bash
git add docs/DH_INVENTORY.md
git commit -m "docs: update DH inventory pipeline with safety gates"
```

---

### Task 13: Quality checks

- [ ] **Step 1: Run full Go test suite with race detection**

Run: `go test -race -timeout 10m ./...`
Expected: PASS

- [ ] **Step 2: Run linter**

Run: `make check`
Expected: PASS

- [ ] **Step 3: Run frontend checks**

Run: `cd web && npx tsc --noEmit && npm run lint`
Expected: PASS for both

- [ ] **Step 4: Final commit if any fixes needed**

Fix any issues found and commit.
