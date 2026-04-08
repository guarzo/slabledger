package sqlite

import (
	"database/sql"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

// purchaseColumns is the canonical SELECT column list for campaign_purchases (no table alias).
const purchaseColumns = `id, campaign_id, card_name, cert_number, card_number, set_name,
	grader, grade_value,
	cl_value_cents, mm_value_cents, buy_cost_cents, psa_sourcing_fee_cents,
	population, purchase_date, created_at, updated_at,
	last_sold_cents, lowest_list_cents, conservative_cents, median_cents,
	active_listings, sales_last_30d, trend_30d, snapshot_date, snapshot_json,
	vault_status, invoice_date, was_refunded, front_image_url, back_image_url, purchase_source,
	psa_listing_title, snapshot_status, snapshot_retry_count,
	override_price_cents, override_source, override_set_at,
	ai_suggested_price_cents, ai_suggested_at,
	card_year, ebay_export_flagged_at,
	reviewed_price_cents, reviewed_at, review_source,
	dh_card_id, dh_inventory_id, dh_cert_status, dh_listing_price_cents, dh_channels_json, dh_status, dh_push_status, dh_candidates, dh_hold_reason,
		gem_rate_id, psa_spec_id,
		card_player, card_variation, card_category`

// purchaseColumnsAliased is the same column list with the "p." table alias for JOIN queries.
const purchaseColumnsAliased = `p.id, p.campaign_id, p.card_name, p.cert_number, p.card_number, p.set_name,
	p.grader, p.grade_value,
	p.cl_value_cents, p.mm_value_cents, p.buy_cost_cents, p.psa_sourcing_fee_cents,
	p.population, p.purchase_date, p.created_at, p.updated_at,
	p.last_sold_cents, p.lowest_list_cents, p.conservative_cents, p.median_cents,
	p.active_listings, p.sales_last_30d, p.trend_30d, p.snapshot_date, p.snapshot_json,
	p.vault_status, p.invoice_date, p.was_refunded, p.front_image_url, p.back_image_url, p.purchase_source,
	p.psa_listing_title, p.snapshot_status, p.snapshot_retry_count,
	p.override_price_cents, p.override_source, p.override_set_at,
	p.ai_suggested_price_cents, p.ai_suggested_at,
	p.card_year, p.ebay_export_flagged_at,
	p.reviewed_price_cents, p.reviewed_at, p.review_source,
	p.dh_card_id, p.dh_inventory_id, p.dh_cert_status, p.dh_listing_price_cents, p.dh_channels_json, p.dh_status, p.dh_push_status, p.dh_candidates, p.dh_hold_reason,
		p.gem_rate_id, p.psa_spec_id,
		p.card_player, p.card_variation, p.card_category`

// saleColumnsAliased is the SELECT column list for campaign_sales with "s." alias, used in LEFT JOIN queries.
const saleColumnsAliased = `s.id, s.purchase_id, s.sale_channel, s.sale_price_cents, s.sale_fee_cents,
	s.sale_date, s.days_to_sell, s.net_profit_cents, s.created_at, s.updated_at,
	s.last_sold_cents, s.lowest_list_cents, s.conservative_cents,
	s.median_cents, s.active_listings, s.sales_last_30d, s.trend_30d, s.snapshot_date, s.snapshot_json`

// scanner abstracts *sql.Row and *sql.Rows so scanPurchase works with both.
type scanner interface {
	Scan(dest ...any) error
}

// purchaseScanDests returns the ordered slice of scan destinations for a Purchase.
// The order matches purchaseColumns exactly.
func purchaseScanDests(p *campaigns.Purchase) []any {
	return []any{
		&p.ID, &p.CampaignID, &p.CardName, &p.CertNumber, &p.CardNumber, &p.SetName,
		&p.Grader, &p.GradeValue,
		&p.CLValueCents, &p.MMValueCents, &p.BuyCostCents, &p.PSASourcingFeeCents,
		&p.Population, &p.PurchaseDate, &p.CreatedAt, &p.UpdatedAt,
		&p.LastSoldCents, &p.LowestListCents, &p.ConservativeCents, &p.MedianCents,
		&p.ActiveListings, &p.SalesLast30d, &p.Trend30d, &p.SnapshotDate, &p.SnapshotJSON,
		&p.VaultStatus, &p.InvoiceDate, &p.WasRefunded, &p.FrontImageURL, &p.BackImageURL, &p.PurchaseSource,
		&p.PSAListingTitle, &p.SnapshotStatus, &p.SnapshotRetryCount,
		&p.OverridePriceCents, &p.OverrideSource, &p.OverrideSetAt,
		&p.AISuggestedPriceCents, &p.AISuggestedAt,
		&p.CardYear, &p.EbayExportFlaggedAt,
		&p.ReviewedPriceCents, &p.ReviewedAt, &p.ReviewSource,
		&p.DHCardID, &p.DHInventoryID, &p.DHCertStatus, &p.DHListingPriceCents, &p.DHChannelsJSON, &p.DHStatus, &p.DHPushStatus, &p.DHCandidatesJSON, &p.DHHoldReason,
		&p.GemRateID, &p.PSASpecID,
		&p.CardPlayer, &p.CardVariation, &p.CardCategory,
	}
}

// scanPurchase scans a single row into a Purchase struct.
// The row must contain exactly the columns listed in purchaseColumns, in order.
func scanPurchase(s scanner, p *campaigns.Purchase) error {
	return s.Scan(purchaseScanDests(p)...)
}

// scanPurchaseWithSale scans a row containing purchase columns followed by sale columns
// (from a LEFT JOIN). Sale columns use sql.Null* types to handle NULL when no sale exists.
func scanPurchaseWithSale(s scanner) (campaigns.PurchaseWithSale, error) {
	var pws campaigns.PurchaseWithSale
	var (
		sID             sql.NullString
		sPurchaseID     sql.NullString
		sSaleChannel    sql.NullString
		sSalePriceCents sql.NullInt64
		sSaleFeeCents   sql.NullInt64
		sSaleDate       sql.NullString
		sDaysToSell     sql.NullInt64
		sNetProfitCents sql.NullInt64
		sCreatedAt      sql.NullTime
		sUpdatedAt      sql.NullTime
		sLastSold       sql.NullInt64
		sLowestList     sql.NullInt64
		sConservative   sql.NullInt64
		sMedian         sql.NullInt64
		sActiveListings sql.NullInt64
		sSalesLast30d   sql.NullInt64
		sTrend30d       sql.NullFloat64
		sSnapshotDate   sql.NullString
		sSnapshotJSON   sql.NullString
	)

	// Build combined dest slice: purchase fields + sale fields.
	dests := append(
		purchaseScanDests(&pws.Purchase),
		&sID, &sPurchaseID, &sSaleChannel, &sSalePriceCents, &sSaleFeeCents,
		&sSaleDate, &sDaysToSell, &sNetProfitCents, &sCreatedAt, &sUpdatedAt,
		&sLastSold, &sLowestList, &sConservative, &sMedian,
		&sActiveListings, &sSalesLast30d, &sTrend30d, &sSnapshotDate, &sSnapshotJSON,
	)

	if err := s.Scan(dests...); err != nil {
		return pws, err
	}

	if sID.Valid {
		sale := &campaigns.Sale{
			ID:             sID.String,
			PurchaseID:     sPurchaseID.String,
			SaleChannel:    campaigns.SaleChannel(sSaleChannel.String),
			SalePriceCents: int(sSalePriceCents.Int64),
			SaleFeeCents:   int(sSaleFeeCents.Int64),
			SaleDate:       sSaleDate.String,
			DaysToSell:     int(sDaysToSell.Int64),
			NetProfitCents: int(sNetProfitCents.Int64),
		}
		if sCreatedAt.Valid {
			sale.CreatedAt = sCreatedAt.Time
		}
		if sUpdatedAt.Valid {
			sale.UpdatedAt = sUpdatedAt.Time
		}
		sale.LastSoldCents = int(sLastSold.Int64)
		sale.LowestListCents = int(sLowestList.Int64)
		sale.ConservativeCents = int(sConservative.Int64)
		sale.MedianCents = int(sMedian.Int64)
		sale.ActiveListings = int(sActiveListings.Int64)
		sale.SalesLast30d = int(sSalesLast30d.Int64)
		if sTrend30d.Valid {
			sale.Trend30d = sTrend30d.Float64
		}
		if sSnapshotDate.Valid {
			sale.SnapshotDate = sSnapshotDate.String
		}
		if sSnapshotJSON.Valid {
			sale.SnapshotJSON = sSnapshotJSON.String
		}
		pws.Sale = sale
	}

	return pws, nil
}
