package sqlite

import (
	"context"
	"database/sql"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"strings"
)

// AnalyticsStore implements inventory.AnalyticsRepository operations.
type AnalyticsStore struct {
	base
}

// NewAnalyticsStore creates a new Analytics store.
func NewAnalyticsStore(db *sql.DB, logger observability.Logger) *AnalyticsStore {
	return &AnalyticsStore{base{db: db, logger: logger}}
}

var _ inventory.AnalyticsRepository = (*AnalyticsStore)(nil)

func (as *AnalyticsStore) GetCampaignPNL(ctx context.Context, campaignID string) (*inventory.CampaignPNL, error) {
	query := `
		SELECT
			COALESCE(SUM(p.buy_cost_cents + p.psa_sourcing_fee_cents), 0),
			COALESCE(SUM(CASE WHEN s.id IS NOT NULL THEN s.sale_price_cents ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN s.id IS NOT NULL THEN s.sale_fee_cents ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN s.id IS NOT NULL THEN s.net_profit_cents ELSE 0 END), 0),
			COALESCE(AVG(CASE WHEN s.id IS NOT NULL THEN s.days_to_sell END), 0),
			COUNT(p.id),
			COUNT(s.id)
		FROM campaign_purchases p
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE p.campaign_id = ?
	`
	pnl := &inventory.CampaignPNL{CampaignID: campaignID}
	err := as.db.QueryRowContext(ctx, query, campaignID).Scan(
		&pnl.TotalSpendCents, &pnl.TotalRevenueCents, &pnl.TotalFeesCents,
		&pnl.NetProfitCents, &pnl.AvgDaysToSell, &pnl.TotalPurchases, &pnl.TotalSold,
	)
	if err != nil {
		return nil, err
	}
	pnl.TotalUnsold = pnl.TotalPurchases - pnl.TotalSold
	if pnl.TotalSpendCents > 0 {
		pnl.ROI = float64(pnl.NetProfitCents) / float64(pnl.TotalSpendCents)
	}
	if pnl.TotalPurchases > 0 {
		pnl.SellThroughPct = float64(pnl.TotalSold) / float64(pnl.TotalPurchases)
	}
	return pnl, nil
}

func (as *AnalyticsStore) GetPNLByChannel(ctx context.Context, campaignID string) ([]inventory.ChannelPNL, error) {
	query := `
		SELECT s.sale_channel, COUNT(*) AS sale_count,
			SUM(s.sale_price_cents) AS revenue,
			SUM(s.sale_fee_cents) AS fees,
			SUM(s.net_profit_cents) AS net_profit,
			AVG(s.days_to_sell) AS avg_days
		FROM campaign_sales s
		INNER JOIN campaign_purchases p ON p.id = s.purchase_id
		WHERE p.campaign_id = ?
		GROUP BY s.sale_channel
		ORDER BY net_profit DESC
	`
	rows, err := as.db.QueryContext(ctx, query, campaignID)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (inventory.ChannelPNL, error) {
		var ch inventory.ChannelPNL
		err := rs.Scan(&ch.Channel, &ch.SaleCount, &ch.RevenueCents, &ch.FeesCents, &ch.NetProfitCents, &ch.AvgDaysToSell)
		return ch, err
	})
}

func (as *AnalyticsStore) GetDailySpend(ctx context.Context, campaignID string, days int) ([]inventory.DailySpend, error) {
	query := `
		SELECT purchase_date,
			SUM(buy_cost_cents + psa_sourcing_fee_cents) AS spend,
			COUNT(*) AS purchase_count
		FROM campaign_purchases
		WHERE campaign_id = ? AND purchase_date >= date('now', '-' || ? || ' days')
		GROUP BY purchase_date
		ORDER BY purchase_date ASC
	`
	rows, err := as.db.QueryContext(ctx, query, campaignID, days)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (inventory.DailySpend, error) {
		var ds inventory.DailySpend
		err := rs.Scan(&ds.Date, &ds.SpendCents, &ds.PurchaseCount)
		return ds, err
	})
}

func (as *AnalyticsStore) GetDaysToSellDistribution(ctx context.Context, campaignID string) ([]inventory.DaysToSellBucket, error) {
	query := `
		SELECT
			CASE
				WHEN s.days_to_sell BETWEEN 0 AND 7 THEN '0-7'
				WHEN s.days_to_sell BETWEEN 8 AND 14 THEN '8-14'
				WHEN s.days_to_sell BETWEEN 15 AND 30 THEN '15-30'
				WHEN s.days_to_sell BETWEEN 31 AND 60 THEN '31-60'
				ELSE '60+'
			END AS label,
			CASE
				WHEN s.days_to_sell BETWEEN 0 AND 7 THEN 0
				WHEN s.days_to_sell BETWEEN 8 AND 14 THEN 8
				WHEN s.days_to_sell BETWEEN 15 AND 30 THEN 15
				WHEN s.days_to_sell BETWEEN 31 AND 60 THEN 31
				ELSE 61
			END AS min_val,
			CASE
				WHEN s.days_to_sell BETWEEN 0 AND 7 THEN 7
				WHEN s.days_to_sell BETWEEN 8 AND 14 THEN 14
				WHEN s.days_to_sell BETWEEN 15 AND 30 THEN 30
				WHEN s.days_to_sell BETWEEN 31 AND 60 THEN 60
				ELSE 999
			END AS max_val,
			COUNT(*) AS cnt
		FROM campaign_sales s
		INNER JOIN campaign_purchases p ON p.id = s.purchase_id
		WHERE p.campaign_id = ?
		GROUP BY label
		ORDER BY min_val ASC
	`
	rows, err := as.db.QueryContext(ctx, query, campaignID)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (inventory.DaysToSellBucket, error) {
		var b inventory.DaysToSellBucket
		err := rs.Scan(&b.Label, &b.Min, &b.Max, &b.Count)
		return b, err
	})
}

func (as *AnalyticsStore) GetPerformanceByGrade(ctx context.Context, campaignID string) ([]inventory.GradePerformance, error) {
	query := `
		SELECT
			p.grade_value AS grade,
			COUNT(p.id) AS purchase_count,
			COUNT(s.id) AS sold_count,
			COALESCE(AVG(CASE WHEN s.id IS NOT NULL THEN s.days_to_sell END), 0) AS avg_days_to_sell,
			SUM(p.buy_cost_cents + p.psa_sourcing_fee_cents) AS total_spend_cents,
			COALESCE(SUM(s.sale_price_cents), 0) AS total_revenue_cents,
			COALESCE(SUM(s.sale_fee_cents), 0) AS total_fees_cents,
			COALESCE(SUM(s.net_profit_cents), 0) AS net_profit_cents,
			AVG(CASE WHEN p.cl_value_cents > 0 THEN CAST(p.buy_cost_cents AS REAL) / p.cl_value_cents ELSE NULL END) AS avg_buy_pct_of_cl
		FROM campaign_purchases p
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE p.campaign_id = ?
		GROUP BY p.grade_value
		ORDER BY p.grade_value
	`
	rows, err := as.db.QueryContext(ctx, query, campaignID)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (inventory.GradePerformance, error) {
		var g inventory.GradePerformance
		var avgBuyPct sql.NullFloat64
		if err := rs.Scan(
			&g.Grade, &g.PurchaseCount, &g.SoldCount, &g.AvgDaysToSell,
			&g.TotalSpendCents, &g.TotalRevenueCents, &g.TotalFeesCents,
			&g.NetProfitCents, &avgBuyPct,
		); err != nil {
			return g, err
		}
		if avgBuyPct.Valid {
			g.AvgBuyPctOfCL = avgBuyPct.Float64
		}
		if g.PurchaseCount > 0 {
			g.SellThroughPct = float64(g.SoldCount) / float64(g.PurchaseCount)
		}
		if g.TotalSpendCents > 0 {
			g.ROI = float64(g.NetProfitCents) / float64(g.TotalSpendCents)
		}
		return g, nil
	})
}

func (as *AnalyticsStore) GetPurchasesWithSales(ctx context.Context, campaignID string) ([]inventory.PurchaseWithSale, error) {
	query := `SELECT ` + purchaseColumnsAliased + `, ` + saleColumnsAliased + `
		FROM campaign_purchases p
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE p.campaign_id = ?
		ORDER BY p.purchase_date DESC`
	rows, err := as.db.QueryContext(ctx, query, campaignID)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (inventory.PurchaseWithSale, error) {
		return scanPurchaseWithSale(rs)
	})
}

func (as *AnalyticsStore) GetAllPurchasesWithSales(ctx context.Context, opts ...inventory.PurchaseFilterOpt) ([]inventory.PurchaseWithSale, error) {
	var f inventory.PurchaseFilter
	for _, opt := range opts {
		opt(&f)
	}

	query := `SELECT ` + purchaseColumnsAliased + `, ` + saleColumnsAliased + `
		FROM campaign_purchases p
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id`

	var conditions []string
	var args []any

	if f.SinceDate != "" {
		conditions = append(conditions, "p.purchase_date >= ?")
		args = append(args, f.SinceDate)
	}
	if f.ExcludeArchived {
		conditions = append(conditions, `p.campaign_id NOT IN (SELECT id FROM campaigns WHERE phase = 'closed')`)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY p.purchase_date DESC"

	rows, err := as.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (inventory.PurchaseWithSale, error) {
		return scanPurchaseWithSale(rs)
	})
}

func (as *AnalyticsStore) GetPortfolioChannelVelocity(ctx context.Context) ([]inventory.ChannelVelocity, error) {
	rows, err := as.db.QueryContext(ctx,
		`SELECT s.sale_channel, COUNT(*), AVG(s.days_to_sell), SUM(s.sale_price_cents)
		FROM campaign_sales s
		GROUP BY s.sale_channel
		ORDER BY COUNT(*) DESC`)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (inventory.ChannelVelocity, error) {
		var cv inventory.ChannelVelocity
		err := rs.Scan(&cv.Channel, &cv.SaleCount, &cv.AvgDaysToSell, &cv.RevenueCents)
		return cv, err
	})
}

func (as *AnalyticsStore) GetDailyCapitalTimeSeries(ctx context.Context) ([]inventory.DailyCapitalPoint, error) {
	query := `
		WITH daily AS (
			SELECT date, SUM(spend) AS spend, SUM(recovery) AS recovery FROM (
				SELECT purchase_date AS date, SUM(buy_cost_cents + psa_sourcing_fee_cents) AS spend, 0 AS recovery
				FROM campaign_purchases WHERE was_refunded = 0 GROUP BY purchase_date
				UNION ALL
				SELECT s.sale_date AS date, 0 AS spend, SUM(s.sale_price_cents) AS recovery
				FROM campaign_sales s GROUP BY s.sale_date
			) GROUP BY date ORDER BY date
		)
		SELECT date,
			SUM(spend) OVER (ORDER BY date) AS cumulative_spend,
			SUM(recovery) OVER (ORDER BY date) AS cumulative_recovery,
			SUM(spend) OVER (ORDER BY date) - SUM(recovery) OVER (ORDER BY date) AS outstanding
		FROM daily ORDER BY date
	`
	rows, err := as.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (inventory.DailyCapitalPoint, error) {
		var dp inventory.DailyCapitalPoint
		err := rs.Scan(&dp.Date, &dp.CumulativeSpendCents, &dp.CumulativeRecoveryCents, &dp.OutstandingCents)
		return dp, err
	})
}

func (as *AnalyticsStore) GetGlobalPNLByChannel(ctx context.Context) ([]inventory.ChannelPNL, error) {
	query := `
		SELECT s.sale_channel, COUNT(*) AS sale_count,
			SUM(s.sale_price_cents) AS revenue,
			SUM(s.sale_fee_cents) AS fees,
			SUM(s.net_profit_cents) AS net_profit,
			AVG(s.days_to_sell) AS avg_days
		FROM campaign_sales s
		GROUP BY s.sale_channel
		ORDER BY net_profit DESC
	`
	rows, err := as.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (inventory.ChannelPNL, error) {
		var ch inventory.ChannelPNL
		err := rs.Scan(&ch.Channel, &ch.SaleCount, &ch.RevenueCents, &ch.FeesCents, &ch.NetProfitCents, &ch.AvgDaysToSell)
		return ch, err
	})
}
