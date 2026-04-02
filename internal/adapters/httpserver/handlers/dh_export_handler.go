package handlers

import (
	"encoding/csv"
	"fmt"
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// unmatchedResponse is the JSON shape returned by HandleUnmatched.
type unmatchedResponse struct {
	Unmatched []unmatchedCard `json:"unmatched"`
	Count     int             `json:"count"`
}

type unmatchedCard struct {
	CardName   string `json:"card_name"`
	SetName    string `json:"set_name"`
	CardNumber string `json:"card_number"`
	CertNumber string `json:"cert_number"`
}

// HandleUnmatched returns cards that do not yet have a DH mapping.
func (h *DHHandler) HandleUnmatched(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	ctx := r.Context()

	purchases, err := h.purchaseLister.ListAllUnsoldPurchases(ctx)
	if err != nil {
		h.logger.Error(ctx, "unmatched: list purchases", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to list purchases")
		return
	}

	mappedSet, err := h.cardIDSaver.GetMappedSet(ctx, pricing.SourceDH)
	if err != nil {
		h.logger.Error(ctx, "unmatched: load mapped set", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to load mappings")
		return
	}

	var unmatched []unmatchedCard
	seen := make(map[string]bool)
	for _, p := range purchases {
		key := dhCardKey(p.CardName, p.SetName, p.CardNumber)
		if seen[key] {
			continue
		}
		seen[key] = true

		if mappedSet[key] != "" {
			continue
		}

		unmatched = append(unmatched, unmatchedCard{
			CardName:   p.CardName,
			SetName:    p.SetName,
			CardNumber: p.CardNumber,
			CertNumber: p.CertNumber,
		})
	}

	if unmatched == nil {
		unmatched = []unmatchedCard{}
	}
	writeJSON(w, http.StatusOK, unmatchedResponse{Unmatched: unmatched, Count: len(unmatched)})
}

// HandleExportUnmatched generates a CSV download of unmatched cards.
func (h *DHHandler) HandleExportUnmatched(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	ctx := r.Context()

	purchases, err := h.purchaseLister.ListAllUnsoldPurchases(ctx)
	if err != nil {
		h.logger.Error(ctx, "export unmatched: list purchases", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to list purchases")
		return
	}

	mappedSet, err := h.cardIDSaver.GetMappedSet(ctx, pricing.SourceDH)
	if err != nil {
		h.logger.Error(ctx, "export unmatched: load mapped set", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to load mappings")
		return
	}

	type exportRow struct {
		certNumber string
		cardName   string
		setName    string
		priceCents int
		costCents  int
	}
	var rows []exportRow
	for _, p := range purchases {
		if mappedSet[dhCardKey(p.CardName, p.SetName, p.CardNumber)] != "" {
			continue
		}
		price := p.CLValueCents
		if p.OverridePriceCents > 0 {
			price = p.OverridePriceCents
		}
		rows = append(rows, exportRow{
			certNumber: p.CertNumber,
			cardName:   p.CardName,
			setName:    p.SetName,
			priceCents: price,
			costCents:  p.BuyCostCents,
		})
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="dh_unmatched.csv"`)
	w.WriteHeader(http.StatusOK)

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"cert_number", "card_name", "set_name", "price", "cost"})
	for _, row := range rows {
		_ = cw.Write([]string{
			sanitizeCSVCell(row.certNumber),
			sanitizeCSVCell(row.cardName),
			sanitizeCSVCell(row.setName),
			centsToDollarStr(row.priceCents),
			centsToDollarStr(row.costCents),
		})
	}
	cw.Flush()
}

// sanitizeCSVCell prefixes values that start with formula-triggering characters
// to prevent CSV injection when opened in spreadsheet software.
func sanitizeCSVCell(s string) string {
	if len(s) > 0 {
		switch s[0] {
		case '=', '+', '-', '@':
			return "'" + s
		}
	}
	return s
}

// centsToDollarStr formats cents as a dollar string (e.g. 12345 -> "123.45").
func centsToDollarStr(cents int) string {
	return fmt.Sprintf("%.2f", float64(cents)/100)
}
