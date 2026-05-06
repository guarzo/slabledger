package handlers

import (
	"encoding/json"
	"errors"
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
	writeJSON(w, http.StatusOK, toOpportunitiesResponse(res, h.svc.EffectivePolicy(r.Context())))
}

// HandleGetPolicy implements GET /api/psa-exchange/policy.
// Returns the active (effective) policy plus the seed/defaults so the UI can
// offer a "reset to defaults" affordance.
func (h *PSAExchangeHandler) HandleGetPolicy(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		writeError(w, http.StatusServiceUnavailable, "PSA-Exchange integration is not configured")
		return
	}
	active := h.svc.EffectivePolicy(r.Context())
	defaults := h.svc.Policy()
	writeJSON(w, http.StatusOK, psaExchangePolicyResponse{
		Active:   toPSAExchangePolicy(active),
		Defaults: toPSAExchangePolicy(defaults),
	})
}

// HandlePutPolicy implements PUT /api/psa-exchange/policy. Admin-only at the
// router level; this handler validates the payload and persists.
func (h *PSAExchangeHandler) HandlePutPolicy(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		writeError(w, http.StatusServiceUnavailable, "PSA-Exchange integration is not configured")
		return
	}
	var body psaExchangePolicy
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	p := psaexchange.Policy{
		HighLiquidityVelocity:   body.HighLiquidityVelocity,
		HighLiquidityConfidence: body.HighLiquidityConfidence,
		HighLiquidityOfferPct:   body.HighLiquidityOfferPct,
		DefaultOfferPct:         body.DefaultOfferPct,
		MinConfidence:           body.MinConfidence,
		MinQuarterVelocity:      body.MinQuarterVelocity,
	}
	if err := h.svc.SetPolicy(r.Context(), p); err != nil {
		switch {
		case errors.Is(err, psaexchange.ErrInvalidPolicy):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, psaexchange.ErrPolicyStoreUnavailable):
			writeError(w, http.StatusServiceUnavailable, "policy store is not configured")
		default:
			if h.logger != nil {
				h.logger.Error(r.Context(), "psa_exchange.set_policy_failed", observability.Err(err))
			}
			writeError(w, http.StatusInternalServerError, "failed to persist policy")
		}
		return
	}
	writeJSON(w, http.StatusOK, psaExchangePolicyResponse{
		Active:   toPSAExchangePolicy(p),
		Defaults: toPSAExchangePolicy(h.svc.Policy()),
	})
}

type psaExchangePolicyResponse struct {
	Active   psaExchangePolicy `json:"active"`
	Defaults psaExchangePolicy `json:"defaults"`
}

func toPSAExchangePolicy(p psaexchange.Policy) psaExchangePolicy {
	return psaExchangePolicy{
		HighLiquidityVelocity:   p.HighLiquidityVelocity,
		HighLiquidityConfidence: p.HighLiquidityConfidence,
		HighLiquidityOfferPct:   p.HighLiquidityOfferPct,
		DefaultOfferPct:         p.DefaultOfferPct,
		MinConfidence:           p.MinConfidence,
		MinQuarterVelocity:      p.MinQuarterVelocity,
	}
}

type psaExchangePolicy struct {
	HighLiquidityVelocity   int     `json:"highLiquidityVelocity"`
	HighLiquidityConfidence int     `json:"highLiquidityConfidence"`
	HighLiquidityOfferPct   float64 `json:"highLiquidityOfferPct"`
	DefaultOfferPct         float64 `json:"defaultOfferPct"`
	MinConfidence           int     `json:"minConfidence"`
	MinQuarterVelocity      int     `json:"minQuarterVelocity"`
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
	Opportunities       []psaExchangeRow  `json:"opportunities"`
	CategoryURL         string            `json:"categoryUrl"`
	FetchedAt           time.Time         `json:"fetchedAt"`
	TotalCatalogPokemon int               `json:"totalCatalogPokemon"`
	AfterFilter         int               `json:"afterFilter"`
	EnrichmentErrors    int               `json:"enrichmentErrors"`
	Policy              psaExchangePolicy `json:"policy"`
}

func toOpportunitiesResponse(res psaexchange.OpportunitiesResult, p psaexchange.Policy) psaExchangeResponse {
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
		Policy: psaExchangePolicy{
			HighLiquidityVelocity:   p.HighLiquidityVelocity,
			HighLiquidityConfidence: p.HighLiquidityConfidence,
			HighLiquidityOfferPct:   p.HighLiquidityOfferPct,
			DefaultOfferPct:         p.DefaultOfferPct,
			MinConfidence:           p.MinConfidence,
			MinQuarterVelocity:      p.MinQuarterVelocity,
		},
	}
}

// centsToUSD converts int64 cents to float64 dollars.
// The existing centsToDollars helper accepts int; this variant handles int64
// as used by the psaexchange domain types.
func centsToUSD(c int64) float64 { return float64(c) / 100.0 }
