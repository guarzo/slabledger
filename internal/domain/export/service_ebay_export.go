package export

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
)

// ListEbayExportItems returns items eligible for eBay export.
func (s *service) ListEbayExportItems(ctx context.Context, flaggedOnly bool) (*inventory.EbayExportListResponse, error) {
	var purchases []inventory.Purchase
	var err error

	if flaggedOnly {
		purchases, err = s.repo.ListEbayFlaggedPurchases(ctx)
	} else {
		purchases, err = s.repo.ListAllUnsoldPurchases(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("list purchases for ebay export: %w", err)
	}

	items := make([]inventory.EbayExportItem, 0, len(purchases))
	for _, p := range purchases {
		if p.Grader != "PSA" {
			continue
		}

		hasCL := p.CLValueCents > 0
		hasMarket := p.MedianCents > 0

		suggested := p.CLValueCents
		if suggested <= 0 {
			suggested = p.MedianCents
		}

		items = append(items, inventory.EbayExportItem{
			PurchaseID:          p.ID,
			CertNumber:          p.CertNumber,
			CardName:            p.CardName,
			SetName:             p.SetName,
			CardNumber:          p.CardNumber,
			CardYear:            p.CardYear,
			GradeValue:          p.GradeValue,
			Grader:              p.Grader,
			CLValueCents:        p.CLValueCents,
			MarketMedianCents:   p.MedianCents,
			SuggestedPriceCents: suggested,
			HasCLValue:          hasCL,
			HasMarketData:       hasMarket,
			FrontImageURL:       p.FrontImageURL,
			BackImageURL:        p.BackImageURL,
			CostBasisCents:      p.BuyCostCents + p.PSASourcingFeeCents,
			LastSoldCents:       p.LastSoldCents,
			ReviewedPriceCents:  p.ReviewedPriceCents,
			ReviewedAt:          p.ReviewedAt,
		})
	}

	return &inventory.EbayExportListResponse{Items: items}, nil
}

// GenerateEbayCSV generates a CSV file for eBay bulk upload.
func (s *service) GenerateEbayCSV(ctx context.Context, items []inventory.EbayExportGenerateItem) ([]byte, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no items to export")
	}

	seen := make(map[string]bool, len(items))
	purchaseIDs := make([]string, 0, len(items))
	for _, item := range items {
		if item.PriceCents <= 0 {
			return nil, fmt.Errorf("item %s has invalid price %d: must be > 0", item.PurchaseID, item.PriceCents)
		}
		if seen[item.PurchaseID] {
			return nil, fmt.Errorf("duplicate purchase ID %s in export request", item.PurchaseID)
		}
		seen[item.PurchaseID] = true
		purchaseIDs = append(purchaseIDs, item.PurchaseID)
	}

	purchases, err := s.repo.GetPurchasesByIDs(ctx, purchaseIDs)
	if err != nil {
		return nil, fmt.Errorf("get purchases for export: %w", err)
	}
	for _, id := range purchaseIDs {
		if _, ok := purchases[id]; !ok {
			return nil, fmt.Errorf("purchase %s not found", id)
		}
	}

	var buf bytes.Buffer
	buf.WriteString("Info,Version=1.0.0,Template=fx_category_template_EBAY_US\n")

	w := csv.NewWriter(&buf)
	headers := []string{
		"*Action(SiteID=US|Country=US|Currency=USD|Version=1193|CC=UTF-8)",
		"CustomLabel", "*Title", "*C:Card Name", "*C:Set", "*C:Card Number",
		"CD:Grade - (ID: 27502)", "CD:Professional Grader - (ID: 27501)",
		"CDA:Certification Number - (ID: 27503)", "CD:Card Condition - (ID: 40001)",
		"C:Rarity", "C:Year Manufactured", "C:Language", "*StartPrice", "PicURL", "*Description",
	}
	if err := w.Write(headers); err != nil {
		return nil, fmt.Errorf("write headers: %w", err)
	}

	for _, item := range items {
		p := purchases[item.PurchaseID]
		if p.Grader != "PSA" {
			return nil, fmt.Errorf("purchase %s is graded by %s, not PSA", item.PurchaseID, p.Grader)
		}
		priceDollars := mathutil.ToDollars(int64(item.PriceCents))

		setPrefix := "Pokemon "
		if isJapaneseSet(p.SetName) {
			setPrefix = "Pokemon Japanese "
		}

		gradeStr := mathutil.FormatGrade(p.GradeValue)
		title := buildEbayTitle(p.CardName, p.SetName, p.CardNumber, gradeStr)

		var pics []string
		if p.FrontImageURL != "" {
			pics = append(pics, p.FrontImageURL)
		}
		if p.BackImageURL != "" {
			pics = append(pics, p.BackImageURL)
		}
		picURL := strings.Join(pics, " | ")

		description := fmt.Sprintf("<p>%s from %s, card number %s. PSA %s.</p>",
			p.CardName, p.SetName, p.CardNumber, gradeStr)

		row := []string{
			"Add",
			fmt.Sprintf("PSA-%s", p.CertNumber),
			title,
			p.CardName,
			setPrefix + p.SetName,
			p.CardNumber,
			gradeStr,
			"Professional Sports Authenticator (PSA)",
			p.CertNumber,
			"", "", p.CardYear, "",
			fmt.Sprintf("%.2f", priceDollars),
			picURL,
			description,
		}
		if err := w.Write(row); err != nil {
			return nil, fmt.Errorf("write row: %w", err)
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("csv flush: %w", err)
	}

	if err := s.repo.ClearEbayExportFlags(ctx, purchaseIDs); err != nil {
		return nil, fmt.Errorf("csv generated but failed to clear export flags (re-export may cause duplicates): %w", err)
	}

	return buf.Bytes(), nil
}

func isJapaneseSet(setName string) bool {
	lower := strings.ToLower(setName)
	return strings.Contains(lower, "japanese") || strings.HasPrefix(lower, "ja ")
}

const ebayMaxTitleLen = 80

func buildEbayTitle(cardName, setName, cardNumber, grade string) string {
	title := fmt.Sprintf("%s Pokemon %s %s PSA %s", cardName, setName, cardNumber, grade)
	if len([]rune(title)) <= ebayMaxTitleLen {
		return title
	}
	// Try without "Pokemon " prefix to save space.
	title = fmt.Sprintf("%s %s %s PSA %s", cardName, setName, cardNumber, grade)
	if len([]rune(title)) <= ebayMaxTitleLen {
		return title
	}
	// Truncate to fit eBay's limit (rune-safe for multi-byte characters).
	return string([]rune(title)[:ebayMaxTitleLen])
}
