package scheduler

import "context"

// SyncStateStore reads and writes sync state key-value pairs.
type SyncStateStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}

// CardIDMapping represents a cached external ID mapping for a card.
type CardIDMapping struct {
	CardName        string
	SetName         string
	CollectorNumber string
	ExternalID      string
}

// CardIDMappingLister lists all mapped cards for a given provider.
type CardIDMappingLister interface {
	ListByProvider(ctx context.Context, provider string) ([]CardIDMapping, error)
}

// CardIDMappingSaver persists card ID mappings discovered during batch discovery.
type CardIDMappingSaver interface {
	SaveExternalID(ctx context.Context, cardName, setName, collectorNumber, provider, externalID string) error
}

// UnsoldCard represents a card identity from unsold purchases.
type UnsoldCard struct {
	CardName   string
	SetName    string
	CardNumber string
}

// CampaignCardLister lists cards from unsold purchases in active campaigns.
type CampaignCardLister interface {
	ListUnsoldCards(ctx context.Context) ([]UnsoldCard, error)
}

// cardKey builds a composite key from card name, set name, and card number.
func cardKey(name, set, number string) string {
	return name + "|" + set + "|" + number
}
