package handlers

import (
	"net/http"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

type saleByCertPurchase struct {
	ID         string  `json:"id"`
	CampaignID string  `json:"campaignId"`
	CardName   string  `json:"cardName"`
	CertNumber string  `json:"certNumber"`
	SetName    string  `json:"setName"`
	GradeValue float64 `json:"gradeValue"`
}

type saleByCertSale struct {
	ID             string `json:"id"`
	SaleChannel    string `json:"saleChannel"`
	SaleDate       string `json:"saleDate"`
	SalePriceCents int    `json:"salePriceCents"`
}

type saleByCertResponse struct {
	Purchase saleByCertPurchase `json:"purchase"`
	Sale     *saleByCertSale    `json:"sale"`
}

// HandleGetSaleByCert handles GET /api/sales/by-cert/{certNumber}.
// Read-only lookup so the UI can find a sold card by cert to un-sell it.
// sale is null when the card is unsold.
func (h *CampaignsHandler) HandleGetSaleByCert(w http.ResponseWriter, r *http.Request) {
	cert := strings.TrimSpace(r.PathValue("certNumber"))
	if cert == "" {
		writeError(w, http.StatusBadRequest, "Cert number is required")
		return
	}

	res, err := h.service.GetSaleStatusByCert(r.Context(), cert)
	if err != nil {
		if inventory.IsPurchaseNotFound(err) {
			writeError(w, http.StatusNotFound, "No purchase found for this cert")
			return
		}
		h.logger.Error(r.Context(), "sale by cert lookup failed", observability.Err(err), observability.String("cert", cert))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	p := res.Purchase
	resp := saleByCertResponse{Purchase: saleByCertPurchase{
		ID: p.ID, CampaignID: p.CampaignID, CardName: p.CardName,
		CertNumber: p.CertNumber, SetName: p.SetName, GradeValue: p.GradeValue,
	}}
	if s := res.Sale; s != nil {
		resp.Sale = &saleByCertSale{ID: s.ID, SaleChannel: string(s.SaleChannel), SaleDate: s.SaleDate, SalePriceCents: s.SalePriceCents}
	}
	writeJSON(w, http.StatusOK, resp)
}
