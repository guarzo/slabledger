package handlers

import (
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

type Handler struct {
	logger    observability.Logger
	priceProv pricing.PriceProvider
}

func NewHandler(
	logger observability.Logger,
	opts ...HandlerOption,
) *Handler {
	h := &Handler{
		logger: logger,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// HandlerOption is a functional option for Handler.
type HandlerOption func(*Handler)

// WithPriceProvider sets the price provider on the handler.
func WithPriceProvider(p pricing.PriceProvider) HandlerOption {
	return func(h *Handler) {
		h.priceProv = p
	}
}
