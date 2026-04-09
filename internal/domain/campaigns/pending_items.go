package campaigns

import (
	"context"
	"time"
)

// PendingItem represents an ambiguous or unmatched PSA import item
// that needs manual review.
type PendingItem struct {
	ID                 string     `json:"id"`
	CertNumber         string     `json:"certNumber"`
	CardName           string     `json:"cardName"`
	SetName            string     `json:"setName"`
	CardNumber         string     `json:"cardNumber"`
	Grade              float64    `json:"grade"`
	BuyCostCents       int        `json:"buyCostCents"`
	PurchaseDate       string     `json:"purchaseDate"`
	Status             string     `json:"status"` // "ambiguous" or "unmatched"
	Candidates         []string   `json:"candidates"`
	Source             string     `json:"source"` // "scheduler" or "manual"
	CreatedAt          time.Time  `json:"createdAt"`
	ResolvedAt         *time.Time `json:"resolvedAt,omitempty"`
	ResolvedCampaignID string     `json:"resolvedCampaignId,omitempty"`
}

// PendingItemRepository manages persistent storage of pending PSA import items.
type PendingItemRepository interface {
	// SavePendingItems upserts pending items by cert_number. Resolved items are skipped.
	SavePendingItems(ctx context.Context, items []PendingItem) error
	// ListPendingItems returns all unresolved pending items, ordered by created_at DESC.
	ListPendingItems(ctx context.Context) ([]PendingItem, error)
	// ResolvePendingItem marks a pending item as resolved with the given campaign ID.
	ResolvePendingItem(ctx context.Context, id string, campaignID string) error
	// DismissPendingItem marks a pending item as resolved with an empty campaign ID.
	DismissPendingItem(ctx context.Context, id string) error
	// CountPendingItems returns the number of unresolved pending items.
	CountPendingItems(ctx context.Context) (int, error)
}
