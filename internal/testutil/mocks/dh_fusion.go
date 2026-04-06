package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
)

// MockDHMarketDataClient is a test double for dhprice.MarketDataClient.
type MockDHMarketDataClient struct {
	RecentSalesFn func(ctx context.Context, cardID int) ([]dh.RecentSale, error)
}

func (m *MockDHMarketDataClient) RecentSales(ctx context.Context, cardID int) ([]dh.RecentSale, error) {
	if m.RecentSalesFn != nil {
		return m.RecentSalesFn(ctx, cardID)
	}
	return nil, nil
}

// MockDHCardIDLookup is a test double for dhprice.CardIDLookup.
type MockDHCardIDLookup struct {
	GetExternalIDFn func(ctx context.Context, cardName, setName, collectorNumber, provider string) (string, error)
}

func (m *MockDHCardIDLookup) GetExternalID(ctx context.Context, cardName, setName, collectorNumber, provider string) (string, error) {
	if m.GetExternalIDFn != nil {
		return m.GetExternalIDFn(ctx, cardName, setName, collectorNumber, provider)
	}
	return "", nil
}
