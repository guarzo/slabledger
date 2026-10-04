package inventory

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

func (s *service) CreateSale(ctx context.Context, sa *Sale, campaign *Campaign, purchase *Purchase) error {
	if err := ValidateSale(sa); err != nil {
		return err
	}
	if sa.ID == "" {
		sa.ID = s.idGen()
	}

	// Mint the idempotency key at creation, before any DH call (design §5a).
	// The row does not exist yet, so no compare-and-set is needed here — a
	// legacy sale predating this column mints lazily instead, via the §5b
	// recovery pass (Task 11).
	sa.DHIdempotencyKey = NewDHIdempotencyKey(s.idGen)
	// HandleCreateSale decodes the client request body straight into this
	// struct with no scrub. DHSaleID/DHSaleRecordedAt are server-owned: they
	// must only ever be set by recordDHSale after a confirmed DH response.
	// Leaving a client-forged value in place would let a client hide a sale
	// from ListSalesNeedingDHRecord and would target an un-sell's void at an
	// arbitrary DH sale id.
	sa.DHSaleID = ""
	sa.DHSaleRecordedAt = nil

	sa.SaleFeeCents = CalculateSaleFee(sa.SaleChannel, sa.SalePriceCents, campaign)

	purchaseDate, err := time.Parse("2006-01-02", purchase.PurchaseDate)
	if err == nil {
		saleDate, err2 := time.Parse("2006-01-02", sa.SaleDate)
		if err2 == nil {
			if saleDate.Before(purchaseDate) {
				return ErrSaleDateBeforePurchase
			}
			sa.DaysToSell = int(saleDate.Sub(purchaseDate).Hours() / 24)
		}
	}

	sa.NetProfitCents = CalculateNetProfit(
		sa.SalePriceCents, purchase.BuyCostCents,
		purchase.PSASourcingFeeCents, sa.SaleFeeCents,
	)

	invoices, invErr := s.finance.ListInvoices(ctx)
	if invErr != nil {
		invoices = nil // heuristic degrades to false; never block a sale on invoice lookup
	}
	if sa.PriceSource == "" {
		sa.PriceSource = PriceSourceManual
	}
	if err := FreezeSaleProvenance(sa, purchase, campaign, IsForcedLiquidation(sa.SaleChannel, sa.SaleDate, invoices)); err != nil {
		return err
	}

	// Best-effort: capture market snapshot at time of sale
	_, _ = s.captureMarketSnapshot(ctx, sa, purchase.ToCardIdentity(), purchase.GradeValue, purchase.CLValueCents)

	now := time.Now()
	sa.CreatedAt = now
	sa.UpdatedAt = now
	if err := s.sales.CreateSale(ctx, sa); err != nil {
		return err
	}

	// Best-effort: mark DH status as sold so inventory UI reflects reality
	if dhErr := s.markCurrentSaleSold(ctx, sa, purchase); dhErr != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "create sale: failed to update dh_status to sold",
				observability.String("purchaseID", sa.PurchaseID),
				observability.Err(dhErr))
		}
	}

	// Best-effort: clear eBay export flag since the card is now sold
	if clearErr := s.purchases.ClearEbayExportFlags(ctx, []string{sa.PurchaseID}); clearErr != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "create sale: failed to clear ebay export flag",
				observability.String("purchaseID", sa.PurchaseID),
				observability.Err(clearErr))
		}
	}

	s.recordDHSale(ctx, "create sale", sa, purchase)

	return nil
}
