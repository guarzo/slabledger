package dh

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
)

// LivePriceResult is the original accepted target and price, not current state.
type LivePriceResult struct {
	DHInventoryID     int  `json:"dh_inventory_id"`
	ListingPriceCents int  `json:"listing_price_cents"`
	Replayed          bool `json:"replayed"`
}

// UnmarshalJSON distinguishes a missing replayed field from a legitimate false.
func (r *LivePriceResult) UnmarshalJSON(data []byte) error {
	var wire struct {
		DHInventoryID     *int  `json:"dh_inventory_id"`
		ListingPriceCents *int  `json:"listing_price_cents"`
		Replayed          *bool `json:"replayed"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.DHInventoryID == nil || *wire.DHInventoryID <= 0 ||
		wire.ListingPriceCents == nil || *wire.ListingPriceCents <= 0 || *wire.ListingPriceCents > 9999999999 ||
		wire.Replayed == nil {
		return fmt.Errorf("DH live-price receipt requires a positive target, bounded positive cents and a replayed boolean")
	}
	*r = LivePriceResult{DHInventoryID: *wire.DHInventoryID, ListingPriceCents: *wire.ListingPriceCents, Replayed: *wire.Replayed}
	return nil
}

// UpdateLivePrice sends only the persisted price intent, once per invocation.
// Scheduler recovery, not a nested transport loop, replays the same key/input.
func (c *Client) UpdateLivePrice(ctx context.Context, inventoryID int, key string, cents int) (*LivePriceResult, error) {
	if inventoryID <= 0 || cents <= 0 || cents > 9999999999 || !validLivePriceKey(key) {
		return nil, apperrors.ProviderInvalidRequest(providerName,
			fmt.Errorf("DH live price requires a positive target, cents in 1..9999999999 and an unchanged UTF-8 key of 1..255 characters without NUL"))
	}
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/inventory/%d/live-price", c.baseURL, inventoryID)
	body := struct {
		ListingPriceCents int `json:"listing_price_cents"`
	}{ListingPriceCents: cents}
	var receipt LivePriceResult
	if err := c.doEnterprise(httpx.WithNoRetry(ctx), "POST", fullURL, body, &receipt, map[string]string{"Idempotency-Key": key}); err != nil {
		return nil, err
	}
	if receipt.DHInventoryID != inventoryID || receipt.ListingPriceCents != cents {
		return nil, &httpx.RequestError{Phase: httpx.PhaseResponse, Uncertain: true,
			Err: apperrors.ProviderInvalidResponse(providerName,
				fmt.Errorf("DH live-price receipt does not match requested target and cents"))}
	}
	return &receipt, nil
}

func validLivePriceKey(key string) bool {
	return utf8.ValidString(key) && !strings.ContainsRune(key, '\x00') &&
		strings.TrimSpace(key) == key && utf8.RuneCountInString(key) >= 1 && utf8.RuneCountInString(key) <= 255
}
