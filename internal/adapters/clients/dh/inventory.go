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
	if err := c.doEnterprise(ctx, "GET", fullURL, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// UpdateInventory updates an inventory item on DH (status and/or cost basis).
// When transitioning an item to "listed", DH performs a PSA cert lookup to
// resolve the card image. We inject X-PSA-API-Key so DH can use our key rather
// than its own; a 401 response means the key is bad or exhausted and callers
// can rotate via UpdateInventoryWithRotation.
func (c *Client) UpdateInventory(ctx context.Context, inventoryID int, update InventoryUpdate) (*InventoryResult, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/inventory/%d", c.baseURL, inventoryID)

	var extraHeaders map[string]string
	if update.Status == InventoryStatusListed {
		if key := c.currentPSAKey(); key != "" {
			extraHeaders = map[string]string{"X-PSA-API-Key": key}
		}
	}

	var resp InventoryResult
	if err := c.patchEnterprise(ctx, fullURL, update, &resp, extraHeaders); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SyncChannels pushes a listed inventory item to external sales channels.
func (c *Client) SyncChannels(ctx context.Context, inventoryID int, channels []string) (*ChannelSyncResponse, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/inventory/%d/sync", c.baseURL, inventoryID)
	body := ChannelSyncRequest{Channels: channels}

	var resp ChannelSyncResponse
	if err := c.postEnterprise(ctx, fullURL, body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DeleteInventory permanently removes an inventory item from DH, cancelling any
// active market orders and delisting from all channels in one transaction.
// Use this on unmatch; use DelistChannels for channel-only removal (e.g. card swap).
func (c *Client) DeleteInventory(ctx context.Context, inventoryID int) error {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/inventory/%d", c.baseURL, inventoryID)
	return c.deleteEnterprise(ctx, fullURL, nil, nil)
}

// DelistChannels removes a listed inventory item from specific external channels.
// If channels is empty, delists from all channels.
func (c *Client) DelistChannels(ctx context.Context, inventoryID int, channels []string) (*ChannelSyncResponse, error) {
	fullURL := fmt.Sprintf("%s/api/v1/enterprise/inventory/%d/sync", c.baseURL, inventoryID)

	var body *ChannelDelistRequest
	if len(channels) > 0 {
		body = &ChannelDelistRequest{Channels: channels}
	}

	var resp ChannelSyncResponse
	if err := c.deleteEnterprise(ctx, fullURL, body, &resp); err != nil {
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
	if err := c.doEnterprise(ctx, "GET", fullURL, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
