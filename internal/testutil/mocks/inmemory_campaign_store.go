package mocks

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// InMemoryCampaignStore is an in-memory store implementing all inventory repository interfaces.
// It uses the Fn-field pattern: set individual Fn fields to override default behaviour.
// The default implementations provide a minimal working in-memory store with
// cascade deletes and duplicate cert detection, suitable for service-layer tests.
//
// Pass the same *InMemoryCampaignStore for all 7 repository slots in inventory.NewService.
type InMemoryCampaignStore struct {
	Campaigns       map[string]*inventory.Campaign
	Purchases       map[string]*inventory.Purchase
	Sales           map[string]*inventory.Sale
	Invoices        map[string]*inventory.Invoice
	CertNumbers     map[string]bool
	PurchaseSales   map[string]bool // purchaseID -> has sale
	PNLData         map[string]*inventory.CampaignPNL
	ChannelVelocity []inventory.ChannelVelocity
	CashflowConfig  *inventory.CashflowConfig

	// Optional overrides (Fn-field pattern)
	CreateCampaignFn                    func(ctx context.Context, c *inventory.Campaign) error
	GetCampaignFn                       func(ctx context.Context, id string) (*inventory.Campaign, error)
	ListCampaignsFn                     func(ctx context.Context, activeOnly bool) ([]inventory.Campaign, error)
	UpdateCampaignFn                    func(ctx context.Context, c *inventory.Campaign) error
	UpdateCampaignIfUnchangedFn         func(ctx context.Context, c *inventory.Campaign, expectedUpdatedAt time.Time) error
	DeleteCampaignFn                    func(ctx context.Context, id string) error
	CreatePurchaseFn                    func(ctx context.Context, p *inventory.Purchase) error
	GetPurchaseFn                       func(ctx context.Context, id string) (*inventory.Purchase, error)
	DeletePurchaseFn                    func(ctx context.Context, id string) error
	ListPurchasesByCampaignFn           func(ctx context.Context, campaignID string, limit, offset int) ([]inventory.Purchase, error)
	ListUnsoldPurchasesFn               func(ctx context.Context, campaignID string) ([]inventory.Purchase, error)
	ListAllUnsoldPurchasesFn            func(ctx context.Context) ([]inventory.Purchase, error)
	CountPurchasesByCampaignFn          func(ctx context.Context, campaignID string) (int, error)
	CreateSaleFn                        func(ctx context.Context, s *inventory.Sale) error
	GetSaleByPurchaseIDFn               func(ctx context.Context, purchaseID string) (*inventory.Sale, error)
	ListSalesByCampaignFn               func(ctx context.Context, campaignID string, limit, offset int) ([]inventory.Sale, error)
	DeleteSaleFn                        func(ctx context.Context, saleID string) error
	DeleteSaleByPurchaseIDFn            func(ctx context.Context, purchaseID string) error
	UpdateSaleReasonFn                  func(ctx context.Context, campaignID, saleID, reason string, forcedLiquidation bool) error
	SetSaleIdempotencyKeyIfAbsentFn     func(ctx context.Context, saleID, key string) (string, error)
	SetSaleDHSaleIDFn                   func(ctx context.Context, saleID, dhSaleID string, recordedAt time.Time) error
	ListSalesNeedingDHRecordFn          func(ctx context.Context, limit int) ([]inventory.SaleNeedingDHRecord, error)
	GetCampaignPNLFn                    func(ctx context.Context, campaignID string) (*inventory.CampaignPNL, error)
	GetPNLByChannelFn                   func(ctx context.Context, campaignID string) ([]inventory.ChannelPNL, error)
	GetDailySpendFn                     func(ctx context.Context, campaignID string, days int) ([]inventory.DailySpend, error)
	GetDaysToSellDistributionFn         func(ctx context.Context, campaignID string) ([]inventory.DaysToSellBucket, error)
	GetPerformanceByGradeFn             func(ctx context.Context, campaignID string) ([]inventory.GradePerformance, error)
	GetPurchasesWithSalesFn             func(ctx context.Context, campaignID string) ([]inventory.PurchaseWithSale, error)
	GetPurchaseByCertNumberFn           func(ctx context.Context, grader, certNumber string) (*inventory.Purchase, error)
	UpdatePurchaseCLValueFn             func(ctx context.Context, id string, clValueCents int, population int, confidence *int) error
	UpdatePurchaseCLSyncedAtFn          func(ctx context.Context, id string, syncedAt string) error
	UpdatePurchaseCardMetadataFn        func(ctx context.Context, id string, cardName, cardNumber, setName string) error
	UpdatePurchaseImagesFn              func(ctx context.Context, id string, frontURL, backURL string) error
	UpdatePurchaseGradeFn               func(ctx context.Context, id string, gradeValue float64) error
	UpdateExternalPurchaseFieldsFn      func(ctx context.Context, id string, p *inventory.Purchase) error
	UpdatePurchaseMarketSnapshotFn      func(ctx context.Context, id string, snap inventory.MarketSnapshotData) error
	UpdatePurchaseCampaignFn            func(ctx context.Context, purchaseID, campaignID string, sourcingFeeCents int) error
	UpdatePurchasePSAFieldsFn           func(ctx context.Context, id string, fields inventory.PSAUpdateFields) error
	ReattributePurchaseFn               func(ctx context.Context, purchaseID string, r inventory.Reattribution) error
	UpdatePurchaseAttributionNameFn     func(ctx context.Context, purchaseID, psaName, source string) error
	GetAllPurchasesWithSalesFn          func(ctx context.Context, opts ...inventory.PurchaseFilterOpt) ([]inventory.PurchaseWithSale, error)
	GetGlobalPNLByChannelFn             func(ctx context.Context) ([]inventory.ChannelPNL, error)
	GetPurchasesByCertNumbersFn         func(ctx context.Context, certNumbers []string) (map[string]*inventory.Purchase, error)
	GetPurchasesByDHInventoryIDsFn      func(ctx context.Context, dhIDs []int) (map[int]*inventory.Purchase, error)
	UpdatePurchaseDHFieldsFn            func(ctx context.Context, id string, update inventory.DHFieldsUpdate) error
	GetPurchasesByDHCertStatusFn        func(ctx context.Context, status string, limit int) ([]inventory.Purchase, error)
	UpdatePurchaseDHPushStatusFn        func(ctx context.Context, id string, status string) error
	IncrementDHPushAttemptsFn           func(ctx context.Context, id string) (int, error)
	UpdatePurchaseDHStatusFn            func(ctx context.Context, id string, status string) error
	ListStaleDHStatusSoldPurchasesFn    func(ctx context.Context) ([]string, error)
	UpdatePurchaseDHCardIDFn            func(ctx context.Context, id string, cardID int) error
	UpdatePurchaseDHCandidatesFn        func(ctx context.Context, id string, candidatesJSON string) error
	UpdatePurchaseDHHoldReasonFn        func(ctx context.Context, id string, reason string) error
	SetHeldWithReasonFn                 func(ctx context.Context, purchaseID string, reason string) error
	ApproveHeldPurchaseFn               func(ctx context.Context, purchaseID string) error
	ResetDHFieldsForRepushFn            func(ctx context.Context, purchaseID string) error
	ResetDHFieldsForRepushDueToDeleteFn func(ctx context.Context, purchaseID string) error
	SetDHSaleConflictFn                 func(ctx context.Context, purchaseID, reason string) error
	ClearDHSaleConflictFn               func(ctx context.Context, purchaseID string) error
	ResetDHFieldsForRelistAfterVoidFn   func(ctx context.Context, purchaseID string) error
	UpdatePurchaseDHPriceSyncFn         func(ctx context.Context, id string, listingPriceCents int, syncedAt time.Time) error
	UnmatchPurchaseDHFn                 func(ctx context.Context, purchaseID string, pushStatus string) error
	ListDHPriceDriftFn                  func(ctx context.Context) ([]inventory.Purchase, error)
	GetDHPushConfigFn                   func(ctx context.Context) (*inventory.DHPushConfig, error)
	SaveDHPushConfigFn                  func(ctx context.Context, cfg *inventory.DHPushConfig) error
	GetPurchasesByDHPushStatusFn        func(ctx context.Context, status string, limit int) ([]inventory.Purchase, error)
	CountDHPipelineHealthFn             func(ctx context.Context) (inventory.DHPipelineHealth, error)
	GetSellSheetItemsFn                 func(ctx context.Context) ([]string, error)
	AddSellSheetItemsFn                 func(ctx context.Context, purchaseIDs []string) error
	RemoveSellSheetItemsFn              func(ctx context.Context, purchaseIDs []string) error
	ClearSellSheetFn                    func(ctx context.Context) error
	OpenFlagPurchaseIDsFn               func(ctx context.Context) (map[string]int64, error)
	GetCapitalRawDataFn                 func(ctx context.Context) (*inventory.CapitalRawData, error)
	GetInvoiceSellThroughFn             func(ctx context.Context, invoiceDate string) (inventory.InvoiceSellThrough, error)
	UpdateCashflowConfigFn              func(ctx context.Context, cfg *inventory.CashflowConfig) error
}

// Compile-time interface checks.
var _ inventory.CampaignRepository = (*InMemoryCampaignStore)(nil)
var _ inventory.PurchaseRepository = (*InMemoryCampaignStore)(nil)
var _ inventory.SaleRepository = (*InMemoryCampaignStore)(nil)
var _ inventory.AnalyticsRepository = (*InMemoryCampaignStore)(nil)
var _ inventory.FinanceRepository = (*InMemoryCampaignStore)(nil)
var _ inventory.PricingRepository = (*InMemoryCampaignStore)(nil)
var _ inventory.DHRepository = (*InMemoryCampaignStore)(nil)

// NewInMemoryCampaignStore creates a ready-to-use InMemoryCampaignStore with initialized maps.
func NewInMemoryCampaignStore() *InMemoryCampaignStore {
	return &InMemoryCampaignStore{
		Campaigns:     make(map[string]*inventory.Campaign),
		Purchases:     make(map[string]*inventory.Purchase),
		Sales:         make(map[string]*inventory.Sale),
		Invoices:      make(map[string]*inventory.Invoice),
		CertNumbers:   make(map[string]bool),
		PurchaseSales: make(map[string]bool),
		PNLData:       make(map[string]*inventory.CampaignPNL),
	}
}

// --- CampaignRepository ---

func (m *InMemoryCampaignStore) CreateCampaign(ctx context.Context, c *inventory.Campaign) error {
	if m.CreateCampaignFn != nil {
		return m.CreateCampaignFn(ctx, c)
	}
	m.Campaigns[c.ID] = c
	return nil
}

func (m *InMemoryCampaignStore) GetCampaign(ctx context.Context, id string) (*inventory.Campaign, error) {
	if m.GetCampaignFn != nil {
		return m.GetCampaignFn(ctx, id)
	}
	c, ok := m.Campaigns[id]
	if !ok {
		return nil, inventory.ErrCampaignNotFound
	}
	return c, nil
}

func (m *InMemoryCampaignStore) ListCampaigns(ctx context.Context, activeOnly bool) ([]inventory.Campaign, error) {
	if m.ListCampaignsFn != nil {
		return m.ListCampaignsFn(ctx, activeOnly)
	}
	var result []inventory.Campaign
	for _, c := range m.Campaigns {
		if activeOnly && c.Phase != inventory.PhaseActive {
			continue
		}
		result = append(result, *c)
	}
	return result, nil
}

func (m *InMemoryCampaignStore) DeleteCampaign(ctx context.Context, id string) error {
	if m.DeleteCampaignFn != nil {
		return m.DeleteCampaignFn(ctx, id)
	}
	if _, ok := m.Campaigns[id]; !ok {
		return inventory.ErrCampaignNotFound
	}
	for pid, p := range m.Purchases {
		if p.CampaignID == id {
			delete(m.CertNumbers, p.CertNumber)
			delete(m.PurchaseSales, pid)
			for sid, s := range m.Sales {
				if s.PurchaseID == pid {
					delete(m.Sales, sid)
				}
			}
			delete(m.Purchases, pid)
		}
	}
	delete(m.Campaigns, id)
	return nil
}

func (m *InMemoryCampaignStore) UpdateCampaign(ctx context.Context, c *inventory.Campaign) error {
	if m.UpdateCampaignFn != nil {
		return m.UpdateCampaignFn(ctx, c)
	}
	if _, ok := m.Campaigns[c.ID]; !ok {
		return inventory.ErrCampaignNotFound
	}
	m.Campaigns[c.ID] = c
	return nil
}

// UpdateCampaignIfUnchanged mirrors the store's conditional write: it compares
// against the currently held row rather than the one being written, so a test
// can reproduce a lost update by mutating m.Campaigns behind the caller's back.
func (m *InMemoryCampaignStore) UpdateCampaignIfUnchanged(ctx context.Context, c *inventory.Campaign, expectedUpdatedAt time.Time) error {
	if m.UpdateCampaignIfUnchangedFn != nil {
		return m.UpdateCampaignIfUnchangedFn(ctx, c, expectedUpdatedAt)
	}
	current, ok := m.Campaigns[c.ID]
	if !ok {
		return inventory.ErrCampaignNotFound
	}
	if !current.UpdatedAt.Equal(expectedUpdatedAt) {
		return inventory.ErrCampaignConflict
	}
	m.Campaigns[c.ID] = c
	return nil
}
