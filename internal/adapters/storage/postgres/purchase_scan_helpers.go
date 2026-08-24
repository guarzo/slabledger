package postgres

import (
	"database/sql"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// purchaseColumns is the canonical SELECT column list for campaign_purchases (no table alias).
const purchaseColumns = `id, campaign_id, card_name, cert_number, card_number, set_name,
	grader, grade_value,
	cl_value_cents, cl_value_updated_at, buy_cost_cents, psa_sourcing_fee_cents,
	population, purchase_date, created_at, updated_at,
	last_sold_cents, lowest_list_cents, conservative_cents, median_cents,
	active_listings, sales_last_30d, trend_30d, snapshot_date, snapshot_json,
	received_at, psa_ship_date, invoice_date, was_refunded, front_image_url, back_image_url, purchase_source,
	psa_listing_title, snapshot_status, snapshot_retry_count,
	override_price_cents, override_source, override_set_at,
	ai_suggested_price_cents, ai_suggested_at,
	card_year, ebay_export_flagged_at,
	reviewed_price_cents, reviewed_at, review_source,
	dh_card_id, dh_inventory_id, dh_cert_status, dh_listing_price_cents, dh_channels_json, dh_status, dh_push_status, dh_push_attempts, dh_candidates, dh_hold_reason,
	gem_rate_id, psa_spec_id,
	card_player, card_variation, card_category, cl_synced_at, cl_last_error, dh_last_synced_at,
	mid_price_cents, last_sold_date, dh_unlisted_detected_at,
	cl_value_at_purchase_cents,
	cl_value_at_purchase_observed_at, cl_value_at_purchase_source, cl_card_confidence_at_purchase,
	cl_policy_confidence_min_at_purchase, population_at_purchase, dh_confidence_at_purchase,
	source_count_at_purchase, active_listings_at_purchase, sales_last_30d_at_purchase,
	buy_terms_cl_pct_at_purchase,
	psa_campaign_name, attribution_source,
	dh_sale_conflict, dh_sale_conflict_at`

// purchaseColumnsAliased is the same column list with the "p." table alias for JOIN queries.
const purchaseColumnsAliased = `p.id, p.campaign_id, p.card_name, p.cert_number, p.card_number, p.set_name,
	p.grader, p.grade_value,
	p.cl_value_cents, p.cl_value_updated_at, p.buy_cost_cents, p.psa_sourcing_fee_cents,
	p.population, p.purchase_date, p.created_at, p.updated_at,
	p.last_sold_cents, p.lowest_list_cents, p.conservative_cents, p.median_cents,
	p.active_listings, p.sales_last_30d, p.trend_30d, p.snapshot_date, p.snapshot_json,
	p.received_at, p.psa_ship_date, p.invoice_date, p.was_refunded, p.front_image_url, p.back_image_url, p.purchase_source,
	p.psa_listing_title, p.snapshot_status, p.snapshot_retry_count,
	p.override_price_cents, p.override_source, p.override_set_at,
	p.ai_suggested_price_cents, p.ai_suggested_at,
	p.card_year, p.ebay_export_flagged_at,
	p.reviewed_price_cents, p.reviewed_at, p.review_source,
	p.dh_card_id, p.dh_inventory_id, p.dh_cert_status, p.dh_listing_price_cents, p.dh_channels_json, p.dh_status, p.dh_push_status, p.dh_push_attempts, p.dh_candidates, p.dh_hold_reason,
		p.gem_rate_id, p.psa_spec_id,
		p.card_player, p.card_variation, p.card_category, p.cl_synced_at, p.cl_last_error, p.dh_last_synced_at,
		p.mid_price_cents, p.last_sold_date, p.dh_unlisted_detected_at,
		p.cl_value_at_purchase_cents,
		p.cl_value_at_purchase_observed_at, p.cl_value_at_purchase_source, p.cl_card_confidence_at_purchase,
		p.cl_policy_confidence_min_at_purchase, p.population_at_purchase, p.dh_confidence_at_purchase,
		p.source_count_at_purchase, p.active_listings_at_purchase, p.sales_last_30d_at_purchase,
		p.buy_terms_cl_pct_at_purchase,
		p.psa_campaign_name, p.attribution_source,
		p.dh_sale_conflict, p.dh_sale_conflict_at`

// saleColumns is the canonical column list for campaign_sales queries (no table alias).
// It is the single source of truth for the table's column set and order: the
// JOIN variant is derived from it (saleColumnsAliased) and both scan paths read
// it through saleScanDests. Adding a column here means adding it to
// saleScanDests and (*saleNulls).sale, which the compiler does not enforce but
// TestSaleColumnsMatchScanDests does.
const saleColumns = `id, purchase_id, sale_channel, sale_price_cents, sale_fee_cents,
	sale_date, days_to_sell, net_profit_cents, created_at, updated_at,
	last_sold_cents, lowest_list_cents, conservative_cents, median_cents,
	active_listings, sales_last_30d, trend_30d, snapshot_date, snapshot_json,
	original_list_price_cents, price_reductions, days_listed, sold_at_asking_price,
	was_cracked, order_id, forced_liquidation, sale_reason, cl_value_at_sale_cents, channel_fee_pct_at_sale,
	their_comp_cents, price_source, cl_value_at_sale_observed_at, cl_value_at_sale_source,
	dh_idempotency_key, dh_sale_id, dh_sale_recorded_at`

// saleColumnsAliased is saleColumns with the "s." table alias, for LEFT JOIN
// queries. Derived rather than hand-maintained: the two lists had drifted by six
// columns (original_list_price_cents, price_reductions, days_listed,
// sold_at_asking_price, was_cracked, order_id), so every JOIN-loaded Sale came
// back with those fields zeroed and indistinguishable from a genuine zero
// (SLA-85).
var saleColumnsAliased = aliasColumns(saleColumns, "s")

// aliasColumns prefixes every bare column name in a canonical list with
// "<alias>.". It handles only plain identifiers — the column lists in this
// package contain no expressions, functions, or AS clauses.
func aliasColumns(columns, alias string) string {
	parts := strings.Split(columns, ",")
	for i, p := range parts {
		parts[i] = alias + "." + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}

// scanner abstracts *sql.Row and *sql.Rows so scanPurchase works with both.
type scanner interface {
	Scan(dest ...any) error
}

// saleNulls holds one nullable destination per saleColumns entry. Both sale scan
// paths use it: the direct SELECT (scanSale) and the LEFT JOIN (scanPurchaseWithSale),
// where every column is NULL for a purchase with no sale.
type saleNulls struct {
	id                      sql.NullString
	purchaseID              sql.NullString
	saleChannel             sql.NullString
	salePriceCents          sql.NullInt64
	saleFeeCents            sql.NullInt64
	saleDate                sql.NullString
	daysToSell              sql.NullInt64
	netProfitCents          sql.NullInt64
	createdAt               sql.NullTime
	updatedAt               sql.NullTime
	lastSoldCents           sql.NullInt64
	lowestListCents         sql.NullInt64
	conservativeCents       sql.NullInt64
	medianCents             sql.NullInt64
	activeListings          sql.NullInt64
	salesLast30d            sql.NullInt64
	trend30d                sql.NullFloat64
	snapshotDate            sql.NullString
	snapshotJSON            sql.NullString
	originalListPriceCents  sql.NullInt64
	priceReductions         sql.NullInt64
	daysListed              sql.NullInt64
	soldAtAskingPrice       sql.NullBool
	wasCracked              sql.NullBool
	orderID                 sql.NullString
	forcedLiquidation       sql.NullBool
	saleReason              sql.NullString
	clValueAtSaleCents      sql.NullInt64
	channelFeePctAtSale     sql.NullFloat64
	theirCompCents          sql.NullInt64
	priceSource             sql.NullString
	clValueAtSaleObservedAt sql.NullString
	clValueAtSaleSource     sql.NullString
	dhIdempotencyKey        sql.NullString
	dhSaleID                sql.NullString
	dhSaleRecordedAt        sql.NullTime
}

// saleScanDests returns the ordered scan destinations for a Sale.
// The order matches saleColumns exactly.
func saleScanDests(n *saleNulls) []any {
	return []any{
		&n.id, &n.purchaseID, &n.saleChannel, &n.salePriceCents, &n.saleFeeCents,
		&n.saleDate, &n.daysToSell, &n.netProfitCents, &n.createdAt, &n.updatedAt,
		&n.lastSoldCents, &n.lowestListCents, &n.conservativeCents, &n.medianCents,
		&n.activeListings, &n.salesLast30d, &n.trend30d, &n.snapshotDate, &n.snapshotJSON,
		&n.originalListPriceCents, &n.priceReductions, &n.daysListed, &n.soldAtAskingPrice,
		&n.wasCracked, &n.orderID, &n.forcedLiquidation,
		&n.saleReason, &n.clValueAtSaleCents, &n.channelFeePctAtSale,
		&n.theirCompCents, &n.priceSource,
		&n.clValueAtSaleObservedAt, &n.clValueAtSaleSource,
		&n.dhIdempotencyKey, &n.dhSaleID, &n.dhSaleRecordedAt,
	}
}

// sale materializes the scanned nulls into a Sale. NULL becomes the Go zero
// value for every field except channel_fee_pct_at_sale, whose nil is meaningful
// (see the Sale struct's note on the two "unknown" encodings).
func (n *saleNulls) sale() inventory.Sale {
	s := inventory.Sale{
		ID:                      n.id.String,
		PurchaseID:              n.purchaseID.String,
		SaleChannel:             inventory.SaleChannel(n.saleChannel.String),
		SalePriceCents:          int(n.salePriceCents.Int64),
		SaleFeeCents:            int(n.saleFeeCents.Int64),
		SaleDate:                n.saleDate.String,
		DaysToSell:              int(n.daysToSell.Int64),
		NetProfitCents:          int(n.netProfitCents.Int64),
		CreatedAt:               n.createdAt.Time,
		UpdatedAt:               n.updatedAt.Time,
		OrderID:                 n.orderID.String,
		OriginalListPriceCents:  int(n.originalListPriceCents.Int64),
		PriceReductions:         int(n.priceReductions.Int64),
		DaysListed:              int(n.daysListed.Int64),
		SoldAtAskingPrice:       n.soldAtAskingPrice.Bool,
		WasCracked:              n.wasCracked.Bool,
		ForcedLiquidation:       n.forcedLiquidation.Bool,
		SaleReason:              n.saleReason.String,
		CLValueAtSaleCents:      int(n.clValueAtSaleCents.Int64),
		TheirCompCents:          int(n.theirCompCents.Int64),
		PriceSource:             n.priceSource.String,
		CLValueAtSaleObservedAt: n.clValueAtSaleObservedAt.String,
		CLValueAtSaleSource:     n.clValueAtSaleSource.String,
		DHIdempotencyKey:        n.dhIdempotencyKey.String,
		DHSaleID:                n.dhSaleID.String,
	}
	s.LastSoldCents = int(n.lastSoldCents.Int64)
	s.LowestListCents = int(n.lowestListCents.Int64)
	s.ConservativeCents = int(n.conservativeCents.Int64)
	s.MedianCents = int(n.medianCents.Int64)
	s.ActiveListings = int(n.activeListings.Int64)
	s.SalesLast30d = int(n.salesLast30d.Int64)
	s.Trend30d = n.trend30d.Float64
	s.SnapshotDate = n.snapshotDate.String
	s.SnapshotJSON = n.snapshotJSON.String
	if n.channelFeePctAtSale.Valid {
		v := n.channelFeePctAtSale.Float64
		s.ChannelFeePctAtSale = &v
	}
	if n.dhSaleRecordedAt.Valid {
		v := n.dhSaleRecordedAt.Time
		s.DHSaleRecordedAt = &v
	}
	return s
}

// scanSale scans a single Sale row matching saleColumns order.
func scanSale(s scanner) (inventory.Sale, error) {
	var n saleNulls
	if err := s.Scan(saleScanDests(&n)...); err != nil {
		return inventory.Sale{}, err
	}
	return n.sale(), nil
}

// purchaseScanDests returns the ordered slice of scan destinations for a Purchase.
// The order matches purchaseColumns exactly. psaCampaignName and attributionSource
// are nullable columns (migration 000023 did not backfill psa_campaign_name and
// does not enforce NOT NULL on either), so they scan into sql.NullString; see
// scanPurchase for the assignment back onto the struct.
func purchaseScanDests(p *inventory.Purchase, psaCampaignName, attributionSource *sql.NullString) []any {
	return []any{
		&p.ID, &p.CampaignID, &p.CardName, &p.CertNumber, &p.CardNumber, &p.SetName,
		&p.Grader, &p.GradeValue,
		&p.CLValueCents, &p.CLValueUpdatedAt, &p.BuyCostCents, &p.PSASourcingFeeCents,
		&p.Population, &p.PurchaseDate, &p.CreatedAt, &p.UpdatedAt,
		&p.LastSoldCents, &p.LowestListCents, &p.ConservativeCents, &p.MedianCents,
		&p.ActiveListings, &p.SalesLast30d, &p.Trend30d, &p.SnapshotDate, &p.SnapshotJSON,
		&p.ReceivedAt, &p.PSAShipDate, &p.InvoiceDate, &p.WasRefunded, &p.FrontImageURL, &p.BackImageURL, &p.PurchaseSource,
		&p.PSAListingTitle, &p.SnapshotStatus, &p.SnapshotRetryCount,
		&p.OverridePriceCents, &p.OverrideSource, &p.OverrideSetAt,
		&p.AISuggestedPriceCents, &p.AISuggestedAt,
		&p.CardYear, &p.EbayExportFlaggedAt,
		&p.ReviewedPriceCents, &p.ReviewedAt, &p.ReviewSource,
		&p.DHCardID, &p.DHInventoryID, &p.DHCertStatus, &p.DHListingPriceCents, &p.DHChannelsJSON, &p.DHStatus, &p.DHPushStatus, &p.DHPushAttempts, &p.DHCandidatesJSON, &p.DHHoldReason,
		&p.GemRateID, &p.PSASpecID,
		&p.CardPlayer, &p.CardVariation, &p.CardCategory, &p.CLSyncedAt, &p.CLLastError, &p.DHLastSyncedAt,
		&p.MidPriceCents, &p.LastSoldDate, &p.DHUnlistedDetectedAt,
		&p.CLValueAtPurchaseCents,
		&p.CLValueAtPurchaseObservedAt, &p.CLValueAtPurchaseSource, &p.CLCardConfidenceAtPurchase,
		&p.CLPolicyConfidenceMinAtPurchase, &p.PopulationAtPurchase, &p.DHConfidenceAtPurchase,
		&p.SourceCountAtPurchase, &p.ActiveListingsAtPurchase, &p.SalesLast30dAtPurchase,
		&p.BuyTermsCLPctAtPurchase,
		psaCampaignName, attributionSource,
		&p.DHSaleConflict, &p.DHSaleConflictAt,
	}
}

// scanPurchase scans a single row into a Purchase struct.
// The row must contain exactly the columns listed in purchaseColumns, in order.
func scanPurchase(s scanner, p *inventory.Purchase) error {
	var psaCampaignName, attributionSource sql.NullString
	if err := s.Scan(purchaseScanDests(p, &psaCampaignName, &attributionSource)...); err != nil {
		return err
	}
	p.PSACampaignName = psaCampaignName.String
	p.AttributionSource = attributionSource.String
	return nil
}

// scanPurchaseWithSale scans a row containing purchase columns followed by sale columns
// (from a LEFT JOIN). Sale columns use sql.Null* types to handle NULL when no sale exists.
func scanPurchaseWithSale(s scanner) (inventory.PurchaseWithSale, error) {
	var pws inventory.PurchaseWithSale
	var (
		sale              saleNulls
		psaCampaignName   sql.NullString
		attributionSource sql.NullString
	)

	// Build combined dest slice: purchase fields + sale fields.
	dests := append(
		purchaseScanDests(&pws.Purchase, &psaCampaignName, &attributionSource),
		saleScanDests(&sale)...,
	)

	if err := s.Scan(dests...); err != nil {
		return pws, err
	}
	pws.Purchase.PSACampaignName = psaCampaignName.String
	pws.Purchase.AttributionSource = attributionSource.String

	// A NULL id means the LEFT JOIN matched no sale row.
	if sale.id.Valid {
		loaded := sale.sale()
		pws.Sale = &loaded
	}

	return pws, nil
}
