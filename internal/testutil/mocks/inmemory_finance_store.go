package mocks

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// --- FinanceRepository ---

func (m *InMemoryCampaignStore) CreateInvoice(_ context.Context, inv *inventory.Invoice) error {
	m.Invoices[inv.ID] = inv
	return nil
}

func (m *InMemoryCampaignStore) GetInvoice(_ context.Context, id string) (*inventory.Invoice, error) {
	if inv, ok := m.Invoices[id]; ok {
		return inv, nil
	}
	return nil, inventory.ErrInvoiceNotFound
}

func (m *InMemoryCampaignStore) ListInvoices(_ context.Context) ([]inventory.Invoice, error) {
	result := make([]inventory.Invoice, 0, len(m.Invoices))
	for _, inv := range m.Invoices {
		result = append(result, *inv)
	}
	return result, nil
}

func (m *InMemoryCampaignStore) UpdateInvoice(_ context.Context, inv *inventory.Invoice) error {
	existing, ok := m.Invoices[inv.ID]
	if !ok {
		return inventory.ErrInvoiceNotFound
	}
	*existing = *inv
	return nil
}

func (m *InMemoryCampaignStore) SumPurchaseCostByInvoiceDate(_ context.Context, invoiceDate string) (int, error) {
	total := 0
	for _, p := range m.Purchases {
		if p.InvoiceDate == invoiceDate && !p.WasRefunded {
			total += p.BuyCostCents + p.PSASourcingFeeCents
		}
	}
	return total, nil
}

func (m *InMemoryCampaignStore) GetPendingReceiptByInvoiceDate(_ context.Context, invoiceDates []string) (map[string]int, error) {
	result := make(map[string]int)
	dateSet := make(map[string]bool)
	for _, d := range invoiceDates {
		dateSet[d] = true
	}
	for _, p := range m.Purchases {
		if dateSet[p.InvoiceDate] && !p.WasRefunded && p.ReceivedAt == nil {
			result[p.InvoiceDate] += p.BuyCostCents
		}
	}
	return result, nil
}

func (m *InMemoryCampaignStore) GetCashflowConfig(_ context.Context) (*inventory.CashflowConfig, error) {
	if m.CashflowConfig != nil {
		cfg := *m.CashflowConfig
		return &cfg, nil
	}
	return &inventory.CashflowConfig{CapitalBudgetCents: 5000000, CashBufferCents: 1000000}, nil
}

func (m *InMemoryCampaignStore) UpdateCashflowConfig(ctx context.Context, cfg *inventory.CashflowConfig) error {
	if m.UpdateCashflowConfigFn != nil {
		return m.UpdateCashflowConfigFn(ctx, cfg)
	}
	if cfg == nil {
		return nil
	}
	cp := *cfg
	m.CashflowConfig = &cp
	return nil
}

func (m *InMemoryCampaignStore) GetCapitalRawData(ctx context.Context) (*inventory.CapitalRawData, error) {
	if m.GetCapitalRawDataFn != nil {
		return m.GetCapitalRawDataFn(ctx)
	}
	return &inventory.CapitalRawData{OutstandingCents: 0, RecoveryRate30dCents: 0, RecoveryRate30dPriorCents: 0}, nil
}

func (m *InMemoryCampaignStore) GetInvoiceSellThrough(ctx context.Context, invoiceDate string) (inventory.InvoiceSellThrough, error) {
	if m.GetInvoiceSellThroughFn != nil {
		return m.GetInvoiceSellThroughFn(ctx, invoiceDate)
	}
	var result inventory.InvoiceSellThrough
	for _, p := range m.Purchases {
		if p.InvoiceDate != invoiceDate || p.WasRefunded {
			continue
		}
		if p.ReceivedAt == nil {
			continue
		}
		result.TotalPurchaseCount++
		result.TotalCostCents += p.BuyCostCents
		if m.PurchaseSales[p.ID] {
			result.SoldCount++
			for _, s := range m.Sales {
				if s.PurchaseID == p.ID {
					result.SaleRevenueCents += s.SalePriceCents
					break
				}
			}
		}
	}
	return result, nil
}

func (m *InMemoryCampaignStore) CreateRevocationFlag(_ context.Context, _ *inventory.RevocationFlag) error {
	return nil
}

func (m *InMemoryCampaignStore) ListRevocationFlags(_ context.Context) ([]inventory.RevocationFlag, error) {
	return []inventory.RevocationFlag{}, nil
}

func (m *InMemoryCampaignStore) GetLatestRevocationFlag(_ context.Context) (*inventory.RevocationFlag, error) {
	return nil, nil
}

func (m *InMemoryCampaignStore) UpdateRevocationFlagStatus(_ context.Context, _ string, _ string, _ *time.Time) error {
	return nil
}
