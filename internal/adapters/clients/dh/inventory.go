package dh

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// InventoryFilters are query parameters for GET /inventory.
type InventoryFilters struct {
	Status       string
	CertNumber   string
	UpdatedSince string
	Page         int
	PerPage      int
}

// OrderFilters are query parameters for GET /orders.
type OrderFilters struct {
	Since   string // ISO 8601, required
	Channel string
	Page    int
	PerPage int
}

// PushInventory creates or updates inventory items on DH (upsert semantics).
func (c *Client) PushInventory(ctx context.Context, items []InventoryItem) (*InventoryPushResponse, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/inventory", c.baseURL)
	body := InventoryPushRequest{Items: items}

	var resp InventoryPushResponse
	if err := c.postEnterprise(ctx, fullURL, body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListInventory retrieves current inventory with optional filters.
func (c *Client) ListInventory(ctx context.Context, filters InventoryFilters) (*InventoryListResponse, error) {
	params := url.Values{}
	if filters.Status != "" {
		params.Set("status", filters.Status)
	}
	if filters.CertNumber != "" {
		params.Set("cert_number", filters.CertNumber)
	}
	if filters.UpdatedSince != "" {
		params.Set("updated_since", filters.UpdatedSince)
	}
	if filters.Page > 0 {
		params.Set("page", fmt.Sprintf("%d", filters.Page))
	}
	if filters.PerPage > 0 {
		params.Set("per_page", fmt.Sprintf("%d", filters.PerPage))
	}

	fullURL := fmt.Sprintf("%s/api/v1/enterprise/inventory?%s", c.baseURL, params.Encode())

	var resp InventoryListResponse
	if err := c.getEnterprise(ctx, fullURL, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetOrders retrieves completed sales from DH.
func (c *Client) GetOrders(ctx context.Context, filters OrderFilters) (*OrdersResponse, error) {
	since := strings.TrimSpace(filters.Since)
	if since == "" {
		return nil, fmt.Errorf("OrderFilters.Since is required")
	}

	params := url.Values{}
	params.Set("since", since)
	if filters.Channel != "" {
		params.Set("channel", filters.Channel)
	}
	if filters.Page > 0 {
		params.Set("page", fmt.Sprintf("%d", filters.Page))
	}
	if filters.PerPage > 0 {
		params.Set("per_page", fmt.Sprintf("%d", filters.PerPage))
	}

	fullURL := fmt.Sprintf("%s/api/v1/enterprise/orders?%s", c.baseURL, params.Encode())

	var resp OrdersResponse
	if err := c.getEnterprise(ctx, fullURL, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
