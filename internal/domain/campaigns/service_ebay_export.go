package campaigns

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// ListEbayExportItems returns items eligible for eBay export.
func (s *service) ListEbayExportItems(ctx context.Context, flaggedOnly bool) (*EbayExportListResponse, error) {
	var purchases []Purchase
	var err error

	if flaggedOnly {
		purchases, err = s.repo.ListEbayFlaggedPurchases(ctx)
	} else {
		purchases, err = s.repo.ListAllUnsoldPurchases(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("list purchases for ebay export: %w", err)
	}

	items := make([]EbayExportItem, 0, len(purchases))
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

		items = append(items, EbayExportItem{
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
		})
	}

	return &EbayExportListResponse{Items: items}, nil
}

// GenerateEbayCSV generates a CSV file for eBay bulk upload.
func (s *service) GenerateEbayCSV(ctx context.Context, items []EbayExportGenerateItem) ([]byte, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no items to export")
	}

	for _, item := range items {
		if item.PriceCents <= 0 {
			return nil, fmt.Errorf("item %s has invalid price %d: must be > 0", item.PurchaseID, item.PriceCents)
		}
	}

	purchaseIDs := make([]string, len(items))
	priceMap := make(map[string]int, len(items))
	for i, item := range items {
		purchaseIDs[i] = item.PurchaseID
		priceMap[item.PurchaseID] = item.PriceCents
	}

	purchases := make(map[string]*Purchase, len(items))
	for _, id := range purchaseIDs {
		p, err := s.repo.GetPurchase(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("get purchase %s: %w", id, err)
		}
		purchases[id] = p
	}

	var buf bytes.Buffer
	buf.WriteString("Info,Version=1.0.0,Template=fx_category_template_EBAY_US\n")

	w := csv.NewWriter(&buf)
	headers := []string{
		"*Action(SiteID=US|Country=US|Currency=USD|Version=1193|CC=UTF-8)",
		"CustomLabel", "*Title", "*C:Card Name", "*C:Set", "*C:Card Number",
		"CD:Grade - (ID: 27502)", "CD:Professional Grader - (ID: 27501)",
		"CDA:Certification Number - (ID: 27503)", "CD:Card Condition - (ID: 40001)",
		"*C:Rarity", "C:Year Manufactured", "C:Language", "*StartPrice", "PicURL", "*Description",
	}
	if err := w.Write(headers); err != nil {
		return nil, fmt.Errorf("write headers: %w", err)
	}

	for _, item := range items {
		p := purchases[item.PurchaseID]
		priceDollars := mathutil.ToDollars(int64(item.PriceCents))

		setPrefix := "Pokemon "
		if isJapaneseSet(p.SetName) {
			setPrefix = "Pokemon Japanese "
		}

		gradeStr := formatGrade(p.GradeValue)
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

	// Best-effort: clear flags after successful CSV generation
	if err := s.repo.ClearEbayExportFlags(ctx, purchaseIDs); err != nil && s.logger != nil {
		s.logger.Warn(ctx, "failed to clear ebay export flags",
			observability.Err(err))
	}

	return buf.Bytes(), nil
}

func isJapaneseSet(setName string) bool {
	upper := strings.ToUpper(setName)
	return strings.Contains(upper, "JAPANESE") || strings.HasPrefix(upper, "JA ")
}

func formatGrade(grade float64) string {
	if grade == float64(int(grade)) {
		return fmt.Sprintf("%d", int(grade))
	}
	return fmt.Sprintf("%.1f", grade)
}

func buildEbayTitle(cardName, setName, cardNumber, grade string) string {
	return fmt.Sprintf("%s Pokemon %s %s PSA %s", cardName, setName, cardNumber, grade)
}
