package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// unmatchedResponse is the JSON shape returned by HandleUnmatched.
type unmatchedResponse struct {
	Unmatched []unmatchedCard `json:"unmatched"`
	Count     int             `json:"count"`
}

type candidateInfo struct {
	DHCardID   int    `json:"dh_card_id"`
	CardName   string `json:"card_name"`
	SetName    string `json:"set_name"`
	CardNumber string `json:"card_number"`
	ImageURL   string `json:"image_url"`
}

type unmatchedCard struct {
	PurchaseID   string          `json:"purchase_id"`
	CardName     string          `json:"card_name"`
	SetName      string          `json:"set_name"`
	CardNumber   string          `json:"card_number"`
	CertNumber   string          `json:"cert_number"`
	Grade        float64         `json:"grade"`
	CLValueCents int             `json:"cl_value_cents"`
	Candidates   []candidateInfo `json:"candidates,omitempty"`
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

	var unmatched []unmatchedCard
	for _, p := range purchases {
		if p.DHPushStatus != campaigns.DHPushStatusUnmatched {
			continue
		}
		card := unmatchedCard{
			PurchaseID:   p.ID,
			CardName:     p.CardName,
			SetName:      p.SetName,
			CardNumber:   p.CardNumber,
			CertNumber:   p.CertNumber,
			Grade:        p.GradeValue,
			CLValueCents: p.CLValueCents,
		}
		if p.DHCandidatesJSON != "" {
			var raw []candidateInfo
			if err := json.Unmarshal([]byte(p.DHCandidatesJSON), &raw); err == nil {
				card.Candidates = raw
			}
		}
		unmatched = append(unmatched, card)
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

	type exportRow struct {
		certNumber string
		cardName   string
		setName    string
		priceCents int
		costCents  int
	}
	var rows []exportRow
	for _, p := range purchases {
		if p.DHPushStatus != campaigns.DHPushStatusUnmatched {
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
