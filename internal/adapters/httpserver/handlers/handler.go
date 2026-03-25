package handlers

import (
	"github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

type Handler struct {
	cardProv      cards.CardProvider
	logger        observability.Logger
	searchService *cards.SearchService
	priceProv     pricing.PriceProvider
}

func NewHandler(
	cardProv cards.CardProvider,
	searchService *cards.SearchService,
	logger observability.Logger,
	opts ...HandlerOption,
) *Handler {
	h := &Handler{
		cardProv:      cardProv,
		searchService: searchService,
		logger:        logger,
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
