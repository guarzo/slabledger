// Package csvimport turns exported CSV files — PSA order exports, eBay and
// Shopify order reports — into campaign purchases and sales.
//
// It was carved out of internal/domain/inventory (SLA-35). The parsers carry a
// large vocabulary of row and result types that only file intake cares about,
// and while they lived in inventory every consumer of an unrelated inventory
// method compiled against them too. The dependency runs one way: csvimport
// imports inventory for the domain types it writes, and inventory has no
// knowledge of this package.
package csvimport

import (
	"context"
	"sync"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// Service ingests CSV exports. Each method takes rows already parsed by the
// Parse* functions in this package, so parsing and persistence stay separable
// (handlers parse, then import; tests can do either alone).
type Service interface {
	ImportPSAExportGlobal(ctx context.Context, rows []PSAExportRow) (*PSAImportResult, error)
	ImportExternalCSV(ctx context.Context, rows []ShopifyExportRow) (*ExternalImportResult, error)
	ImportOrdersSales(ctx context.Context, rows []OrdersExportRow) (*OrdersImportResult, error)
	ImportEbayOrdersSales(ctx context.Context, rows []EbayOrderRow) (*OrdersImportResult, error)
	ConfirmOrdersSales(ctx context.Context, items []OrdersConfirmItem) (*inventory.BulkSaleResult, error)
	ReconcilePSAAttribution(ctx context.Context, rows []PSAExportRow) (ReconcileResult, error)

	// Close waits for background work started by an import (card ID backfill)
	// to finish. Callers that wire this service own calling it at shutdown.
	Close()
}

// Deps are the collaborators an import Service needs. The repositories and inv
// are required; the rest are optional and nil-checked at their use sites, which
// is how the same import path serves a fully-wired server and a bare test.
type Deps struct {
	Campaigns    inventory.CampaignRepository
	Purchases    inventory.PurchaseRepository
	Sales        inventory.SaleRepository
	Finance      inventory.FinanceRepository
	PendingItems inventory.PendingItemRepository

	// Inventory lands parsed rows as purchases; see inventory.IntakeSupportService.
	Inventory inventory.IntakeSupportService

	// PriceLookup is only consulted for its presence: when a price provider is
	// wired, new purchases are marked snapshot-pending for the background
	// snapshot worker instead of being priced inline during the import.
	PriceLookup     inventory.PriceLookup
	CardIDResolver  inventory.CardIDResolver
	CertEnrichQueue inventory.CertEnrichEnqueuer
	DHSoldNotifier  inventory.DHSoldNotifier
	PSAResolver     inventory.PSACampaignResolver

	IDGen  func() string
	Logger observability.Logger
}

type service struct {
	campaigns       inventory.CampaignRepository
	purchases       inventory.PurchaseRepository
	sales           inventory.SaleRepository
	finance         inventory.FinanceRepository
	pendingItemRepo inventory.PendingItemRepository

	inv inventory.IntakeSupportService

	priceProv       inventory.PriceLookup
	cardIDResolver  inventory.CardIDResolver
	certEnrichQueue inventory.CertEnrichEnqueuer
	dhSoldNotifier  inventory.DHSoldNotifier
	psaResolver     inventory.PSACampaignResolver

	idGen  func() string
	logger observability.Logger

	// wg tracks background goroutines started during an import (card ID backfill).
	wg sync.WaitGroup
}

// Close waits for background import work to finish.
func (s *service) Close() { s.wg.Wait() }

var _ Service = (*service)(nil)

// NewService builds an import service. IDGen is required, matching
// inventory.NewService — a nil generator produces empty IDs, which surfaces far
// downstream as a duplicate-key error rather than as the wiring bug it is.
func NewService(d Deps) Service {
	if d.IDGen == nil {
		panic("csvimport.NewService: IDGen is required")
	}
	return &service{
		campaigns:       d.Campaigns,
		purchases:       d.Purchases,
		sales:           d.Sales,
		finance:         d.Finance,
		pendingItemRepo: d.PendingItems,
		inv:             d.Inventory,
		priceProv:       d.PriceLookup,
		cardIDResolver:  d.CardIDResolver,
		certEnrichQueue: d.CertEnrichQueue,
		dhSoldNotifier:  d.DHSoldNotifier,
		psaResolver:     d.PSAResolver,
		idGen:           d.IDGen,
		logger:          d.Logger,
	}
}
