package handlers

import (
	"errors"
	"net/http"

	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// SalesCompsHandler serves sales comp data for purchases.
type SalesCompsHandler struct {
	salesStore   *sqlite.CLSalesStore
	mappingStore *sqlite.CardLadderStore
	campService  campaigns.Service
	logger       observability.Logger
}

// NewSalesCompsHandler creates a new sales comps handler.
func NewSalesCompsHandler(
	salesStore *sqlite.CLSalesStore,
	mappingStore *sqlite.CardLadderStore,
	campService campaigns.Service,
	logger observability.Logger,
) *SalesCompsHandler {
	return &SalesCompsHandler{
		salesStore:   salesStore,
		mappingStore: mappingStore,
		campService:  campService,
		logger:       logger,
	}
}

type saleCompResponse struct {
	Date        string  `json:"date"`
	Price       float64 `json:"price"`
	Platform    string  `json:"platform"`
	ListingType string  `json:"listingType"`
	Seller      string  `json:"seller"`
	URL         string  `json:"url"`
	SlabSerial  string  `json:"slabSerial,omitempty"`
}

// HandleGetSalesComps returns recent sales comps for a purchase.
func (h *SalesCompsHandler) HandleGetSalesComps(w http.ResponseWriter, r *http.Request) {
	purchaseID, ok := pathID(w, r, "id", "purchase ID")
	if !ok {
		return
	}

	// Look up the purchase to get its cert number.
	purchase, err := h.campService.GetPurchase(r.Context(), purchaseID)
	if err != nil {
		if errors.Is(err, campaigns.ErrPurchaseNotFound) {
			writeError(w, http.StatusNotFound, "purchase not found")
			return
		}
		h.logger.Error(r.Context(), "failed to get purchase", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Look up the CL mapping to get gemRateID.
	mapping, err := h.mappingStore.GetMapping(r.Context(), purchase.CertNumber)
	if err != nil {
		h.logger.Error(r.Context(), "failed to get CL mapping", observability.String("cert", purchase.CertNumber), observability.Err(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if mapping == nil || mapping.CLGemRateID == "" {
		writeJSON(w, http.StatusOK, []saleCompResponse{})
		return
	}

	comps, err := h.salesStore.GetSaleComps(r.Context(), mapping.CLGemRateID, mapping.CLCondition, 50)
	if err != nil {
		h.logger.Error(r.Context(), "failed to get sales comps", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	result := make([]saleCompResponse, 0, len(comps))
	for _, c := range comps {
		result = append(result, saleCompResponse{
			Date:        c.SaleDate,
			Price:       mathutil.ToDollars(int64(c.PriceCents)),
			Platform:    c.Platform,
			ListingType: c.ListingType,
			Seller:      c.Seller,
			URL:         c.ItemURL,
			SlabSerial:  c.SlabSerial,
		})
	}

	writeJSON(w, http.StatusOK, result)
}
