package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

type HealthHandler struct {
	healthChecker pricing.HealthChecker
	cardProv      cards.CardProvider
	priceProv     pricing.PriceProvider
	logger        observability.Logger
}

func NewHealthHandler(
	healthChecker pricing.HealthChecker,
	cardProv cards.CardProvider,
	priceProv pricing.PriceProvider,
	logger observability.Logger,
) *HealthHandler {
	return &HealthHandler{
		healthChecker: healthChecker,
		cardProv:      cardProv,
		priceProv:     priceProv,
		logger:        logger,
	}
}

func (h *HealthHandler) HandleHealthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	dbStatus := "not configured"
	overallStatus := "healthy"
	if h.healthChecker != nil {
		if err := h.healthChecker.Ping(r.Context()); err != nil {
			dbStatus = fmt.Sprintf("unhealthy: %v", err)
			overallStatus = "degraded"
		} else {
			dbStatus = "healthy"
		}
	}

	health := map[string]any{
		"status":    overallStatus,
		"timestamp": time.Now(),
		"providers": map[string]bool{
			"cards":  h.cardProv != nil,
			"prices": h.priceProv != nil && h.priceProv.Available(),
		},
		"database": dbStatus,
	}

	statusCode := http.StatusOK
	if overallStatus != "healthy" {
		statusCode = http.StatusServiceUnavailable
	}

	writeJSON(w, statusCode, health)
}
