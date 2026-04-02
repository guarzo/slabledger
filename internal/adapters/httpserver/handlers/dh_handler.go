package handlers

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/intelligence"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
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

// DHHandler handles DH bulk match, export, intelligence, and suggestions endpoints.
type DHHandler struct {
	matchClient     DHMatchClient
	cardIDSaver     DHCardIDSaver
	purchaseLister  DHPurchaseLister
	intelRepo       intelligence.Repository
	suggestionsRepo intelligence.SuggestionsRepository
	logger          observability.Logger
}

// NewDHHandler creates a new DHHandler with the given dependencies.
func NewDHHandler(
	matchClient DHMatchClient,
	cardIDSaver DHCardIDSaver,
	purchaseLister DHPurchaseLister,
	intelRepo intelligence.Repository,
	suggestionsRepo intelligence.SuggestionsRepository,
	logger observability.Logger,
) *DHHandler {
	return &DHHandler{
		matchClient:     matchClient,
		cardIDSaver:     cardIDSaver,
		purchaseLister:  purchaseLister,
		intelRepo:       intelRepo,
		suggestionsRepo: suggestionsRepo,
		logger:          logger,
	}
}

// bulkMatchResponse is the JSON shape returned by HandleBulkMatch.
type bulkMatchResponse struct {
	Total         int `json:"total"`
	Matched       int `json:"matched"`
	Skipped       int `json:"skipped"`
	LowConfidence int `json:"low_confidence"`
	Failed        int `json:"failed"`
}

// HandleBulkMatch matches all unmatched inventory cards against the DH catalog.
func (h *DHHandler) HandleBulkMatch(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	ctx := r.Context()

	identities, err := h.uniqueCardIdentities(ctx)
	if err != nil {
		h.logger.Error(ctx, "bulk match: list purchases", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to list purchases")
		return
	}

	// Pre-load all existing DH mappings in a single query.
	mappedSet, err := h.cardIDSaver.GetMappedSet(ctx, pricing.SourceDH)
	if err != nil {
		h.logger.Error(ctx, "bulk match: load mapped set", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to load mappings")
		return
	}

	result := bulkMatchResponse{Total: len(identities)}

	for _, ci := range identities {
		if ctx.Err() != nil {
			break
		}

		if mappedSet[dhCardKey(ci.CardName, ci.SetName, ci.CardNumber)] != "" {
			result.Skipped++
			continue
		}

		title := ci.PSAListingTitle
		if title == "" {
			title = buildMatchTitle(ci.CardName, ci.SetName, ci.CardNumber)
		}

		matchResp, err := h.matchClient.Match(ctx, title, "")
		if err != nil {
			h.logger.Warn(ctx, "bulk match: DH match failed",
				observability.String("card", ci.CardName), observability.Err(err))
			result.Failed++
			continue
		}

		if !matchResp.Success || matchResp.Confidence < 0.90 {
			result.LowConfidence++
			continue
		}

		externalID := strconv.Itoa(matchResp.CardID)
		if err := h.cardIDSaver.SaveExternalID(ctx, ci.CardName, ci.SetName, ci.CardNumber, pricing.SourceDH, externalID); err != nil {
			h.logger.Error(ctx, "bulk match: save external ID", observability.Err(err),
				observability.String("card", ci.CardName))
			result.Failed++
			continue
		}
		result.Matched++
	}

	writeJSON(w, http.StatusOK, result)
}

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

// HandleGetIntelligence returns market intelligence for a specific card.
func (h *DHHandler) HandleGetIntelligence(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	ctx := r.Context()

	cardName := r.URL.Query().Get("card_name")
	setName := r.URL.Query().Get("set_name")
	cardNumber := r.URL.Query().Get("card_number")
	if cardName == "" || setName == "" {
		writeError(w, http.StatusBadRequest, "card_name and set_name are required")
		return
	}

	intel, err := h.intelRepo.GetByCard(ctx, cardName, setName, cardNumber)
	if err != nil {
		h.logger.Error(ctx, "get intelligence", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to get intelligence")
		return
	}
	if intel == nil {
		writeError(w, http.StatusNotFound, "no intelligence data found")
		return
	}

	writeJSON(w, http.StatusOK, intel)
}

// HandleGetSuggestions returns the latest DH buy/sell suggestions.
func (h *DHHandler) HandleGetSuggestions(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	ctx := r.Context()

	suggestions, err := h.suggestionsRepo.GetLatest(ctx)
	if err != nil {
		h.logger.Error(ctx, "get suggestions", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to get suggestions")
		return
	}
	if suggestions == nil {
		suggestions = []intelligence.Suggestion{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"suggestions": suggestions, "count": len(suggestions)})
}

// HandleInventoryAlerts cross-references latest DH suggestions against current inventory.
func (h *DHHandler) HandleInventoryAlerts(w http.ResponseWriter, r *http.Request) {
	if requireUser(w, r) == nil {
		return
	}
	ctx := r.Context()

	suggestions, err := h.suggestionsRepo.GetLatest(ctx)
	if err != nil {
		h.logger.Error(ctx, "inventory alerts: get suggestions", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to get suggestions")
		return
	}

	purchases, err := h.purchaseLister.ListAllUnsoldPurchases(ctx)
	if err != nil {
		h.logger.Error(ctx, "inventory alerts: list purchases", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "failed to list purchases")
		return
	}

	// Build lookup set of inventory cards by name+set+number for efficient matching
	type inventoryKey struct{ name, set, cardNumber string }
	inventorySet := make(map[inventoryKey]bool, len(purchases))
	for _, p := range purchases {
		inventorySet[inventoryKey{
			name:       strings.ToLower(p.CardName),
			set:        strings.ToLower(p.SetName),
			cardNumber: strings.ToLower(p.CardNumber),
		}] = true
	}

	var alerts []intelligence.Suggestion
	for _, s := range suggestions {
		key := inventoryKey{
			name:       strings.ToLower(s.CardName),
			set:        strings.ToLower(s.SetName),
			cardNumber: strings.ToLower(s.CardNumber),
		}
		if inventorySet[key] {
			alerts = append(alerts, s)
		}
	}
	if alerts == nil {
		alerts = []intelligence.Suggestion{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"alerts": alerts, "count": len(alerts)})
}

// uniqueCardIdentities returns deduplicated card identities from all unsold purchases.
func (h *DHHandler) uniqueCardIdentities(ctx context.Context) ([]campaigns.CardIdentity, error) {
	purchases, err := h.purchaseLister.ListAllUnsoldPurchases(ctx)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool, len(purchases))
	var identities []campaigns.CardIdentity
	for _, p := range purchases {
		key := dhCardKey(p.CardName, p.SetName, p.CardNumber)
		if seen[key] {
			continue
		}
		seen[key] = true
		identities = append(identities, p.ToCardIdentity())
	}
	return identities, nil
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

// dhCardKey builds the pipe-delimited key used by GetMappedSet.
func dhCardKey(cardName, setName, cardNumber string) string {
	return cardName + "|" + setName + "|" + cardNumber
}

// centsToDollarStr formats cents as a dollar string (e.g. 12345 -> "123.45").
func centsToDollarStr(cents int) string {
	return fmt.Sprintf("%.2f", float64(cents)/100)
}
