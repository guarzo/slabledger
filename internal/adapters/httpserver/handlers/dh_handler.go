package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/intelligence"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// DHMatchClient is the subset of the DH client needed for card matching.
type DHMatchClient interface {
	Match(ctx context.Context, title, sku string) (*dh.MatchResponse, error)
	Available() bool
}

// DHCardIDSaver reads and writes DH card ID mappings.
type DHCardIDSaver interface {
	GetExternalID(ctx context.Context, cardName, setName, collectorNumber, provider string) (string, error)
	SaveExternalID(ctx context.Context, cardName, setName, collectorNumber, provider, externalID string) error
	GetMappedSet(ctx context.Context, provider string) (map[string]string, error)
}

// DHPurchaseLister lists all unsold purchases for bulk match and export operations.
type DHPurchaseLister interface {
	ListAllUnsoldPurchases(ctx context.Context) ([]campaigns.Purchase, error)
}

// DHInventoryPusher pushes inventory items to DH.
type DHInventoryPusher interface {
	PushInventory(ctx context.Context, items []dh.InventoryItem) (*dh.InventoryPushResponse, error)
}

// DHFieldsUpdater persists DH tracking fields on local purchases.
type DHFieldsUpdater interface {
	UpdatePurchaseDHFields(ctx context.Context, id string, update campaigns.DHFieldsUpdate) error
}

// DHIntelligenceCounter returns aggregate stats for market intelligence.
type DHIntelligenceCounter interface {
	CountAll(ctx context.Context) (int, error)
	LatestFetchedAt(ctx context.Context) (string, error)
}

// DHSuggestionsCounter returns aggregate stats for DH suggestions.
type DHSuggestionsCounter interface {
	CountLatest(ctx context.Context) (int, error)
	LatestFetchedAt(ctx context.Context) (string, error)
}

// DHHandler handles DH bulk match, export, intelligence, and suggestions endpoints.
type DHHandler struct {
	matchClient     DHMatchClient
	cardIDSaver     DHCardIDSaver
	purchaseLister  DHPurchaseLister
	inventoryPusher DHInventoryPusher // optional: pushes matched cards to DH inventory
	dhFieldsUpdater DHFieldsUpdater   // optional: persists DH inventory IDs after push
	intelRepo       intelligence.Repository
	suggestionsRepo intelligence.SuggestionsRepository
	intelCounter    DHIntelligenceCounter
	suggestCounter  DHSuggestionsCounter
	logger          observability.Logger
	baseCtx         context.Context

	bgWG             sync.WaitGroup
	bulkMatchMu      sync.Mutex
	bulkMatchRunning atomic.Bool
}

// NewDHHandler creates a new DHHandler with the given dependencies.
// baseCtx is a server-lifecycle context; background goroutines derive from it.
func NewDHHandler(
	matchClient DHMatchClient,
	cardIDSaver DHCardIDSaver,
	purchaseLister DHPurchaseLister,
	inventoryPusher DHInventoryPusher,
	dhFieldsUpdater DHFieldsUpdater,
	intelRepo intelligence.Repository,
	suggestionsRepo intelligence.SuggestionsRepository,
	intelCounter DHIntelligenceCounter,
	suggestCounter DHSuggestionsCounter,
	logger observability.Logger,
	baseCtx context.Context,
) *DHHandler {
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	return &DHHandler{
		matchClient:     matchClient,
		cardIDSaver:     cardIDSaver,
		purchaseLister:  purchaseLister,
		inventoryPusher: inventoryPusher,
		dhFieldsUpdater: dhFieldsUpdater,
		intelRepo:       intelRepo,
		suggestionsRepo: suggestionsRepo,
		intelCounter:    intelCounter,
		suggestCounter:  suggestCounter,
		logger:          logger,
		baseCtx:         baseCtx,
	}
}

// Wait blocks until all background goroutines (e.g. bulk match) have completed.
// Call during graceful shutdown to avoid writing to a closed database.
func (h *DHHandler) Wait() { h.bgWG.Wait() }

// dhCardKey builds the pipe-delimited key used by GetMappedSet.
func dhCardKey(cardName, setName, cardNumber string) string {
	return cardName + "|" + setName + "|" + cardNumber
}

// buildMatchTitle constructs a search title from card metadata when PSAListingTitle is empty.
func buildMatchTitle(cardName, setName, cardNumber string) string {
	parts := []string{cardName}
	if setName != "" {
		parts = append(parts, setName)
	}
	if cardNumber != "" {
		parts = append(parts, cardNumber)
	}
	return strings.Join(parts, " ")
}

// marshalChannels serializes channel statuses to JSON, defaulting to "[]".
func marshalChannels(channels []dh.InventoryChannelStatus) string {
	if len(channels) == 0 {
		return "[]"
	}
	b, err := json.Marshal(channels)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// Compile-time checks.
var _ DHInventoryPusher = (*dh.Client)(nil)
