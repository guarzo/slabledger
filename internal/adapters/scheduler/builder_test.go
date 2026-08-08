package scheduler

import (
	"context"
	"reflect"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// Stubs for the package-local interfaces that gate schedulers in BuildGroup.
// They only need to satisfy the interface — BuildGroup never calls them.

type builderStubSyncStateStore struct{}

func (builderStubSyncStateStore) Get(context.Context, string) (string, error) { return "", nil }
func (builderStubSyncStateStore) Set(context.Context, string, string) error   { return nil }

type builderStubInventoryLister struct{}

func (builderStubInventoryLister) ListUnsoldInventory(context.Context) ([]InventoryPurchase, error) {
	return nil, nil
}

type builderStubSnapshotRefresher struct{}

func (builderStubSnapshotRefresher) RefreshSnapshot(context.Context, InventoryPurchase) bool {
	return false
}

type builderStubSnapshotEnrichService struct{}

func (builderStubSnapshotEnrichService) ProcessPendingSnapshots(context.Context, int) (int, int, int, error) {
	return 0, 0, 0, nil
}

func (builderStubSnapshotEnrichService) RetryFailedSnapshots(context.Context, int) (int, int, int, error) {
	return 0, 0, 0, nil
}

type builderStubDHReconciler struct{}

func (builderStubDHReconciler) Reconcile(context.Context) (dhlisting.ReconcileResult, error) {
	return dhlisting.ReconcileResult{}, nil
}

type builderStubDHInventoryListClient struct{}

func (builderStubDHInventoryListClient) ListInventory(context.Context, dh.InventoryFilters) (*dh.InventoryListResponse, error) {
	return nil, nil
}

type builderStubDHFieldsUpdater struct{}

func (builderStubDHFieldsUpdater) UpdatePurchaseDHFields(context.Context, string, inventory.DHFieldsUpdate) error {
	return nil
}

type builderStubPurchaseByCertLookup struct{}

func (builderStubPurchaseByCertLookup) GetDHStatusByCertNumber(context.Context, string) (string, string, error) {
	return "", "", nil
}

type builderStubRowProvider struct{}

func (builderStubRowProvider) FetchRows(context.Context) ([]inventory.PSAExportRow, error) {
	return nil, nil
}

type builderStubPSAImporter struct{}

func (builderStubPSAImporter) ImportPSAExportGlobal(context.Context, []inventory.PSAExportRow) (*inventory.PSAImportResult, error) {
	return nil, nil
}

// defaultCfg returns the default configuration as a pointer, which is what
// BuildGroup takes.
func defaultCfg() *config.Config {
	cfg := config.Default()
	return &cfg
}

// minimalDeps returns the smallest BuildDeps that BuildGroup accepts: every
// optional dependency is nil, so only unconditional schedulers are built.
func minimalDeps() BuildDeps {
	return BuildDeps{Logger: observability.NewNoopLogger()}
}

// hasSchedulerOfType reports whether the group holds an entry of type T.
func hasSchedulerOfType[T Scheduler](g *Group) bool {
	for _, s := range g.schedulers {
		if _, ok := s.(T); ok {
			return true
		}
	}
	return false
}

// TestBuildGroup_NoNilEntries guards the typed-nil trap: the per-scheduler build
// helpers return concrete pointer types, so storing an unbuilt (nil) scheduler in
// the Scheduler interface would produce an entry that compares non-nil but panics
// on Start. Every entry must be a live scheduler.
func TestBuildGroup_NoNilEntries(t *testing.T) {
	tests := []struct {
		name string
		deps BuildDeps
	}{
		{name: "minimal deps", deps: minimalDeps()},
		{name: "all cheap gates satisfied", deps: fullyGatedDeps()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildGroup(defaultCfg(), tt.deps)

			if len(result.Group.schedulers) == 0 {
				t.Fatal("expected at least the unconditional price refresh scheduler")
			}
			for i, s := range result.Group.schedulers {
				if s == nil {
					t.Errorf("scheduler[%d] is a nil interface", i)
					continue
				}
				if v := reflect.ValueOf(s); v.Kind() == reflect.Ptr && v.IsNil() {
					t.Errorf("scheduler[%d] (%T) is a typed-nil pointer", i, s)
				}
			}
		})
	}
}

// TestBuildGroup_MinimalDeps checks that with no optional dependencies the group
// contains only the unconditional schedulers and every optional BuildResult field
// is left nil.
func TestBuildGroup_MinimalDeps(t *testing.T) {
	result := BuildGroup(defaultCfg(), minimalDeps())

	if !hasSchedulerOfType[*PriceRefreshScheduler](result.Group) {
		t.Error("price refresh scheduler is unconditional but is missing")
	}

	// config.Default() enables access log cleanup with a 30-day retention.
	if !hasSchedulerOfType[*AccessLogCleanupScheduler](result.Group) {
		t.Error("access log cleanup is enabled by default but is missing")
	}

	if hasSchedulerOfType[*GapCleanupScheduler](result.Group) {
		t.Error("gap cleanup was built without a GapStore")
	}
	if hasSchedulerOfType[*InventoryRefreshScheduler](result.Group) {
		t.Error("inventory refresh was built without its dependencies")
	}

	if result.CardLadderRefresh != nil {
		t.Error("CardLadderRefresh should be nil without Card Ladder deps")
	}
	if result.PSASync != nil {
		t.Error("PSASync should be nil without PSA deps")
	}
	if result.CertEnrichJob != nil {
		t.Error("CertEnrichJob should be nil without cert lookup deps")
	}
	if result.DHOrdersPoll != nil {
		t.Error("DHOrdersPoll should be nil without DH orders deps")
	}
	if result.DHReconcile != nil {
		t.Error("DHReconcile should be nil without a DH reconciler")
	}
}

// TestBuildGroup_AccessLogCleanupGate covers the one scheduler gated purely on
// config rather than on a dependency.
func TestBuildGroup_AccessLogCleanupGate(t *testing.T) {
	tests := []struct {
		name          string
		enabled       bool
		retentionDays int
		want          bool
	}{
		{name: "enabled with retention", enabled: true, retentionDays: 30, want: true},
		{name: "disabled", enabled: false, retentionDays: 30, want: false},
		{name: "zero retention", enabled: true, retentionDays: 0, want: false},
		{name: "negative retention", enabled: true, retentionDays: -1, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.Maintenance.AccessLogCleanupEnabled = tt.enabled
			cfg.Maintenance.AccessLogRetentionDays = tt.retentionDays

			result := BuildGroup(&cfg, minimalDeps())

			if got := hasSchedulerOfType[*AccessLogCleanupScheduler](result.Group); got != tt.want {
				t.Errorf("access log cleanup present = %v, want %v", got, tt.want)
			}
		})
	}
}

// fullyGatedDeps satisfies every gate that can be met without a live *dh.Client
// or Postgres store. The DH-client-gated and Card Ladder schedulers are excluded.
func fullyGatedDeps() BuildDeps {
	logger := observability.NewNoopLogger()
	purchaseRepo := &mocks.PurchaseRepositoryMock{}
	return BuildDeps{
		Logger:                logger,
		GapStore:              mocks.NewMockGapStore(),
		InventoryLister:       builderStubInventoryLister{},
		SnapshotRefresher:     builderStubSnapshotRefresher{},
		SnapshotEnrichService: builderStubSnapshotEnrichService{},
		DHPriceSyncService:    &mocks.MockDHPriceSyncService{},
		DHReconciler:          builderStubDHReconciler{},
		PurchaseRepo:          purchaseRepo,
		SyncStateStore:        builderStubSyncStateStore{},
		DHInventoryListClient: builderStubDHInventoryListClient{},
		DHFieldsUpdater:       builderStubDHFieldsUpdater{},
		PurchaseByCertLookup:  builderStubPurchaseByCertLookup{},
		PSARowProvider:        builderStubRowProvider{},
		PSAImporter:           builderStubPSAImporter{},
		CertEnrichJobPrebuilt: NewCertEnrichJob(&mocks.MockCertLookup{}, purchaseRepo, logger),
	}
}

// TestBuildGroup_DependencyGates checks that each dependency-gated scheduler is
// absent under minimal deps and present once its dependencies are supplied.
func TestBuildGroup_DependencyGates(t *testing.T) {
	cfg := defaultCfg()
	minimal := BuildGroup(cfg, minimalDeps())
	full := BuildGroup(cfg, fullyGatedDeps())

	tests := []struct {
		name    string
		present func(*Group) bool
	}{
		{"gap cleanup", hasSchedulerOfType[*GapCleanupScheduler]},
		{"inventory refresh", hasSchedulerOfType[*InventoryRefreshScheduler]},
		{"snapshot enrich", hasSchedulerOfType[*SnapshotEnrichScheduler]},
		{"dh price sync", hasSchedulerOfType[*DHPriceSyncScheduler]},
		{"dh reconcile", hasSchedulerOfType[*DHReconcileScheduler]},
		{"dh sold reconciler", hasSchedulerOfType[*DHSoldReconcilerScheduler]},
		{"dh inventory poll", hasSchedulerOfType[*DHInventoryPollScheduler]},
		{"psa sync", hasSchedulerOfType[*PSASyncScheduler]},
		{"cert enrich", hasSchedulerOfType[*CertEnrichJob]},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.present(minimal.Group) {
				t.Error("built without its dependencies")
			}
			if !tt.present(full.Group) {
				t.Error("not built despite its dependencies being present")
			}
		})
	}
}

// TestBuildGroup_NamedResultsMatchGroup checks that each optional BuildResult
// field points at the same instance that was added to the group, so callers
// holding the reference drive the scheduler the group actually runs.
func TestBuildGroup_NamedResultsMatchGroup(t *testing.T) {
	result := BuildGroup(defaultCfg(), fullyGatedDeps())

	contains := func(want Scheduler) bool {
		for _, s := range result.Group.schedulers {
			if s == want {
				return true
			}
		}
		return false
	}

	if result.PSASync == nil || !contains(result.PSASync) {
		t.Error("PSASync is not the instance registered in the group")
	}
	if result.CertEnrichJob == nil || !contains(result.CertEnrichJob) {
		t.Error("CertEnrichJob is not the instance registered in the group")
	}
	if result.DHReconcile == nil || !contains(result.DHReconcile) {
		t.Error("DHReconcile is not the instance registered in the group")
	}
}

// TestBuildCertEnrichJob_PrefersPrebuilt checks that the pre-built job wins over
// constructing a fresh one, so the instance injected into inventory.service via
// WithCertEnrichEnqueuer is the one the group runs.
func TestBuildCertEnrichJob_PrefersPrebuilt(t *testing.T) {
	logger := observability.NewNoopLogger()
	purchaseRepo := &mocks.PurchaseRepositoryMock{}
	prebuilt := NewCertEnrichJob(&mocks.MockCertLookup{}, purchaseRepo, logger)

	got := buildCertEnrichJob(BuildDeps{
		Logger:                logger,
		CertEnrichJobPrebuilt: prebuilt,
		CertLookup:            &mocks.MockCertLookup{},
		PurchaseRepo:          purchaseRepo,
	})

	if got != prebuilt {
		t.Error("expected the pre-built cert enrich job to be reused")
	}

	fresh := buildCertEnrichJob(BuildDeps{
		Logger:       logger,
		CertLookup:   &mocks.MockCertLookup{},
		PurchaseRepo: purchaseRepo,
	})
	if fresh == nil {
		t.Error("expected a cert enrich job to be built from CertLookup + PurchaseRepo")
	}

	if buildCertEnrichJob(BuildDeps{Logger: logger, CertLookup: &mocks.MockCertLookup{}}) != nil {
		t.Error("expected nil without a PurchaseRepo")
	}
}
