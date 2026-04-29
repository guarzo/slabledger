package handlers

import (
	"net/http"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/psaexchange"
)

// PSAExchangeHandler serves the read-only PSA-Exchange opportunity endpoint.
type PSAExchangeHandler struct {
	svc    psaexchange.Service
	logger observability.Logger
}

// NewPSAExchangeHandler constructs the handler. svc may be nil when the
// integration is disabled (e.g. token unset); in that case the handler
// returns 503.
func NewPSAExchangeHandler(svc psaexchange.Service, logger observability.Logger) *PSAExchangeHandler {
	return &PSAExchangeHandler{svc: svc, logger: logger}
}

// HandleGetOpportunities implements GET /api/psa-exchange/opportunities.
func (h *PSAExchangeHandler) HandleGetOpportunities(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		writeError(w, http.StatusServiceUnavailable, "PSA-Exchange integration is not configured")
		return
	}
	res, err := h.svc.Opportunities(r.Context())
	if err != nil {
		if h.logger != nil {
			h.logger.Error(r.Context(), "psa_exchange.opportunities_failed", observability.Err(err))
		}
		writeError(w, http.StatusBadGateway, "failed to fetch PSA-Exchange opportunities")
		return
	}
	writeJSON(w, http.StatusOK, toOpportunitiesResponse(res))
}

type psaExchangeRow struct {
	Cert            string    `json:"cert"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Grade           string    `json:"grade"`
	ListPrice       float64   `json:"listPrice"`
	TargetOffer     float64   `json:"targetOffer"`
	MaxOfferPct     float64   `json:"maxOfferPct"`
	Comp            float64   `json:"comp"`
	LastSalePrice   float64   `json:"lastSalePrice"`
	LastSaleDate    time.Time `json:"lastSaleDate"`
	VelocityMonth   int       `json:"velocityMonth"`
	VelocityQuarter int       `json:"velocityQuarter"`
	Confidence      int       `json:"confidence"`
	Population      int       `json:"population"`
	EdgeAtOffer     float64   `json:"edgeAtOffer"`
	Score           float64   `json:"score"`
	ListRunwayPct   float64   `json:"listRunwayPct"`
	MayTakeAtList   bool      `json:"mayTakeAtList"`
	FrontImage      string    `json:"frontImage"`
	BackImage       string    `json:"backImage"`
	IndexID         string    `json:"indexId"`
	Tier            string    `json:"tier"`
}

type psaExchangeResponse struct {
	Opportunities       []psaExchangeRow `json:"opportunities"`
	CategoryURL         string           `json:"categoryUrl"`
	FetchedAt           time.Time        `json:"fetchedAt"`
	TotalCatalogPokemon int              `json:"totalCatalogPokemon"`
	AfterFilter         int              `json:"afterFilter"`
	EnrichmentErrors    int              `json:"enrichmentErrors"`
}

func toOpportunitiesResponse(res psaexchange.OpportunitiesResult) psaExchangeResponse {
	rows := make([]psaExchangeRow, 0, len(res.Opportunities))
	for _, l := range res.Opportunities {
		rows = append(rows, psaExchangeRow{
			Cert:            l.Cert,
			Name:            l.Name,
			Description:     l.Description,
			Grade:           l.Grade,
			ListPrice:       centsToUSD(l.ListPriceCents),
			TargetOffer:     centsToUSD(l.TargetOfferCents),
			MaxOfferPct:     l.MaxOfferPct,
			Comp:            centsToUSD(l.CompCents),
			LastSalePrice:   centsToUSD(l.LastSalePriceCents),
			LastSaleDate:    l.LastSaleDate,
			VelocityMonth:   l.VelocityMonth,
			VelocityQuarter: l.VelocityQuarter,
			Confidence:      l.Confidence,
			Population:      l.Population,
			EdgeAtOffer:     l.EdgeAtOffer,
			Score:           l.Score,
			ListRunwayPct:   l.ListRunwayPct,
			MayTakeAtList:   l.MayTakeAtList,
			FrontImage:      l.FrontImage,
			BackImage:       l.BackImage,
			IndexID:         l.IndexID,
			Tier:            l.Tier,
		})
	}
	return psaExchangeResponse{
		Opportunities:       rows,
		CategoryURL:         res.CategoryURL,
		FetchedAt:           res.FetchedAt,
		TotalCatalogPokemon: res.TotalCatalog,
		AfterFilter:         res.AfterFilter,
		EnrichmentErrors:    res.EnrichmentErrors,
	}
}

// centsToUSD converts int64 cents to float64 dollars.
// The existing centsToDollars helper accepts int; this variant handles int64
// as used by the psaexchange domain types.
func centsToUSD(c int64) float64 { return float64(c) / 100.0 }
