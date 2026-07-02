package portfolio

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// ComputeAnalysis produces a full portfolio analysis response from raw data.
// It is pure computation — no I/O, no repo calls.
//
// Campaigns with ID == "external" are skipped defensively; callers should also
// exclude external-campaign rows before calling this function.
//
// The since parameter is an optional "YYYY-MM-DD" date string. When non-empty,
// SessionDeltas filters purchases, sales, campaigns, and invoices relative to it.
func ComputeAnalysis(
	campaigns []inventory.Campaign,
	rows []inventory.PurchaseWithSale,
	invoices []inventory.Invoice,
	since string,
	now time.Time,
) *AnalysisResponse {
	// Group rows by campaign ID for O(1) lookup.
	byCampaign := make(map[string][]inventory.PurchaseWithSale, len(campaigns))
	for _, r := range rows {
		byCampaign[r.Purchase.CampaignID] = append(byCampaign[r.Purchase.CampaignID], r)
	}

	analyses := make([]CampaignAnalysis, 0, len(campaigns))
	for _, c := range campaigns {
		if c.ID == inventory.ExternalCampaignID {
			continue
		}
		cRows := byCampaign[c.ID]
		analyses = append(analyses, CampaignAnalysis{
			CampaignID:     c.ID,
			CampaignName:   c.Name,
			Phase:          c.Phase,
			BuyTermsCLPct:  c.BuyTermsCLPct,
			BPCLAtBuy:      computeBPCLAtBuy(cRows),
			PNL:            computeSplitPNL(cRows),
			WeeklyFill:     computeWeeklyFill(c, cRows, now),
			InScopeByGrade: computeInScopeByGrade(c, cRows),
		})
	}

	return &AnalysisResponse{
		GeneratedAt: now.Format(time.RFC3339),
		Since:       since,
		Campaigns:   analyses,
		Deltas:      computeDeltas(campaigns, rows, invoices, since),
	}
}

// computeBPCLAtBuy computes buy-price / CL-at-purchase statistics.
// Rows without a CLValueAtPurchaseCents snapshot contribute to Total but
// are excluded from DollarWeighted and MeanDriftPct.
func computeBPCLAtBuy(rows []inventory.PurchaseWithSale) BPCLStats {
	var sumBuyCost, sumCLAtBuy, sumDrift float64
	var n, total int

	for _, r := range rows {
		total++
		if r.Purchase.CLValueAtPurchaseCents == 0 {
			continue
		}
		n++
		sumBuyCost += float64(r.Purchase.BuyCostCents)
		clAtBuy := float64(r.Purchase.CLValueAtPurchaseCents)
		sumCLAtBuy += clAtBuy
		// Drift: how much has current CL moved relative to the purchase snapshot?
		sumDrift += (float64(r.Purchase.CLValueCents) - clAtBuy) / clAtBuy * 100
	}

	var dollarWeighted, meanDrift, coveragePct float64
	if sumCLAtBuy > 0 {
		dollarWeighted = sumBuyCost / sumCLAtBuy
	}
	if n > 0 {
		meanDrift = sumDrift / float64(n)
		coveragePct = float64(n) / float64(total) * 100
	}

	return BPCLStats{
		N:              n,
		Total:          total,
		CoveragePct:    coveragePct,
		DollarWeighted: dollarWeighted,
		MeanDriftPct:   meanDrift,
	}
}

// computeSplitPNL separates realised P&L into discretionary vs forced-liquidation.
func computeSplitPNL(rows []inventory.PurchaseWithSale) SplitPNL {
	var disc, forced PNLBlock
	for _, r := range rows {
		if r.Sale == nil {
			continue
		}
		b := &disc
		if r.Sale.ForcedLiquidation {
			b = &forced
		}
		b.SoldCount++
		b.RevenueCents += r.Sale.SalePriceCents
		b.NetProfitCents += r.Sale.NetProfitCents
	}
	disc.ROIPct = roiPct(disc.RevenueCents, disc.NetProfitCents)
	forced.ROIPct = roiPct(forced.RevenueCents, forced.NetProfitCents)
	return SplitPNL{Discretionary: disc, Forced: forced}
}

// roiPct computes netProfit/(revenue-netProfit)*100, returning 0 when cost basis ≤ 0.
func roiPct(revenue, netProfit int) float64 {
	cost := revenue - netProfit
	if cost <= 0 {
		return 0
	}
	return float64(netProfit) / float64(cost) * 100
}

// computeWeeklyFill produces 8 Monday-bucketed weekly fill entries covering the
// trailing 8 weeks from now. Purchases outside that window are silently ignored.
func computeWeeklyFill(c inventory.Campaign, rows []inventory.PurchaseWithSale, now time.Time) []WeeklyFill {
	thisMonday := mondayOf(now)
	eightWeeksAgo := thisMonday.AddDate(0, 0, -7*7)

	// Pre-build 8 ordered week keys (oldest → newest).
	weekKeys := make([]string, 8)
	for i := 0; i < 8; i++ {
		wk := thisMonday.AddDate(0, 0, -7*(7-i))
		weekKeys[i] = wk.Format("2006-01-02")
	}

	type bucket struct{ fills, spend int }
	buckets := make(map[string]*bucket, 8)
	for _, k := range weekKeys {
		buckets[k] = &bucket{}
	}

	for _, r := range rows {
		pDate, err := time.Parse("2006-01-02", r.Purchase.PurchaseDate)
		if err != nil {
			continue
		}
		wk := mondayOf(pDate)
		if wk.Before(eightWeeksAgo) || wk.After(thisMonday) {
			continue
		}
		key := wk.Format("2006-01-02")
		if b, ok := buckets[key]; ok {
			b.fills++
			b.spend += r.Purchase.BuyCostCents
		}
	}

	capCents := c.DailySpendCapCents * 7
	result := make([]WeeklyFill, 8)
	for i, key := range weekKeys {
		b := buckets[key]
		var util float64
		if capCents > 0 {
			util = float64(b.spend) / float64(capCents) * 100
		}
		result[i] = WeeklyFill{
			WeekStart:      key,
			Fills:          b.fills,
			SpendCents:     b.spend,
			CapCents:       capCents,
			UtilizationPct: util,
		}
	}
	return result
}

// computeInScopeByGrade groups in-scope purchases by grade and aggregates metrics.
// DollarWeightedBPCLAtBuy uses buyCost/clAtBuy for rows with a snapshot only.
// SoldCount and NetProfitCents count discretionary sales only.
func computeInScopeByGrade(c inventory.Campaign, rows []inventory.PurchaseWithSale) []GradeScopeRow {
	type gradeData struct {
		n          int
		sumBuyCost float64
		sumCLAtBuy float64
		soldCount  int
		netProfit  int
	}
	byGrade := make(map[float64]*gradeData)

	for _, r := range rows {
		if !inScope(c, r.Purchase) {
			continue
		}
		g := r.Purchase.GradeValue
		if byGrade[g] == nil {
			byGrade[g] = &gradeData{}
		}
		d := byGrade[g]
		d.n++
		if r.Purchase.CLValueAtPurchaseCents > 0 {
			d.sumBuyCost += float64(r.Purchase.BuyCostCents)
			d.sumCLAtBuy += float64(r.Purchase.CLValueAtPurchaseCents)
		}
		if r.Sale != nil && !r.Sale.ForcedLiquidation {
			d.soldCount++
			d.netProfit += r.Sale.NetProfitCents
		}
	}

	grades := make([]float64, 0, len(byGrade))
	for g := range byGrade {
		grades = append(grades, g)
	}
	sort.Float64s(grades)

	result := make([]GradeScopeRow, 0, len(grades))
	for _, g := range grades {
		d := byGrade[g]
		var dwBPCL float64
		if d.sumCLAtBuy > 0 {
			dwBPCL = d.sumBuyCost / d.sumCLAtBuy
		}
		result = append(result, GradeScopeRow{
			Grade:                   g,
			N:                       d.n,
			DollarWeightedBPCLAtBuy: dwBPCL,
			SoldCount:               d.soldCount,
			NetProfitCents:          d.netProfit,
		})
	}
	return result
}

// computeDeltas calculates what changed since the provided date.
// When since is empty, purchase/sale counts and campaign-updated are not filtered;
// all invoices are included.
func computeDeltas(
	campaigns []inventory.Campaign,
	rows []inventory.PurchaseWithSale,
	invoices []inventory.Invoice,
	since string,
) SessionDeltas {
	var d SessionDeltas

	for _, r := range rows {
		if since == "" || r.Purchase.PurchaseDate >= since {
			d.NewPurchases++
			d.NewPurchaseCents += r.Purchase.BuyCostCents
		}
		if r.Sale != nil {
			if since == "" || r.Sale.SaleDate >= since {
				d.NewSales++
				d.NewSaleCents += r.Sale.SalePriceCents
			}
		}
	}

	// Campaigns updated strictly after the start of the since date.
	if since != "" {
		if sinceTime, err := time.Parse("2006-01-02", since); err == nil {
			for _, c := range campaigns {
				if c.ID == inventory.ExternalCampaignID {
					continue
				}
				if c.UpdatedAt.After(sinceTime) {
					d.CampaignsUpdated = append(d.CampaignsUpdated, c.Name)
				}
			}
		}
	}

	for _, inv := range invoices {
		if since == "" || inv.InvoiceDate >= since {
			d.Invoices = append(d.Invoices, InvoiceSummary{
				InvoiceDate: inv.InvoiceDate,
				DueDate:     inv.DueDate,
				TotalCents:  inv.TotalCents,
				Status:      inv.Status,
			})
		}
	}

	return d
}

// mondayOf returns the Monday of the ISO week containing t, at midnight UTC.
func mondayOf(t time.Time) time.Time {
	wd := t.Weekday()
	offset := int(wd) - int(time.Monday)
	if offset < 0 {
		offset += 7
	}
	return time.Date(t.Year(), t.Month(), t.Day()-offset, 0, 0, 0, 0, t.Location())
}

// inScope reports whether a purchase satisfies the campaign's filter criteria.
//
// Rules (each absent/unparsable constraint means no filter on that dimension):
//   - GradeRange: GradeValue ∈ [min, max]
//   - PriceRange: BuyCostCents ∈ [min*100, max*100]  (range stored in dollars)
//   - YearRange:  CardYear (int) ∈ [min, max]; skipped if CardYear is empty or non-numeric
//   - InclusionList non-empty, ExclusionMode=false: CardPlayer (case-insensitive) must be in list
//   - InclusionList non-empty, ExclusionMode=true:  CardPlayer must NOT be in list
func inScope(c inventory.Campaign, p inventory.Purchase) bool {
	if minG, maxG, ok := parseRange(c.GradeRange); ok {
		if p.GradeValue < minG || p.GradeValue > maxG {
			return false
		}
	}

	if minP, maxP, ok := parseRange(c.PriceRange); ok {
		minCents := int(minP * 100)
		maxCents := int(maxP * 100)
		if p.BuyCostCents < minCents || p.BuyCostCents > maxCents {
			return false
		}
	}

	if minY, maxY, ok := parseRange(c.YearRange); ok && p.CardYear != "" {
		if year, err := strconv.Atoi(p.CardYear); err == nil {
			if float64(year) < minY || float64(year) > maxY {
				return false
			}
		}
	}

	if c.InclusionList != "" {
		playerLower := strings.ToLower(p.CardPlayer)
		inList := false
		for _, part := range strings.Split(c.InclusionList, ",") {
			if strings.TrimSpace(strings.ToLower(part)) == playerLower {
				inList = true
				break
			}
		}
		if !c.ExclusionMode && !inList {
			return false
		}
		if c.ExclusionMode && inList {
			return false
		}
	}

	return true
}

// parseRange parses a campaign range string into (min, max, ok).
// Accepted forms: "9-10" → (9, 10, true); "10" → (10, 10, true).
// Empty or unparsable input returns (0, 0, false), meaning no constraint.
func parseRange(s string) (float64, float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, false
	}
	// Look for a "-" separator, skipping the first character to avoid treating
	// a leading minus sign as a separator (all domain values are positive).
	if idx := strings.Index(s[1:], "-"); idx >= 0 {
		realIdx := idx + 1
		lo, err1 := strconv.ParseFloat(strings.TrimSpace(s[:realIdx]), 64)
		hi, err2 := strconv.ParseFloat(strings.TrimSpace(s[realIdx+1:]), 64)
		if err1 == nil && err2 == nil {
			return lo, hi, true
		}
	}
	// Single value: "10" → (10, 10, true).
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, 0, false
	}
	return v, v, true
}
