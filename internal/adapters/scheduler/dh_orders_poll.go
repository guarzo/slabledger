package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

const syncStateKeyDHOrdersPoll = "dh_orders_last_poll"

// maxOrderPagesPerPoll prevents unbounded pagination if the DH API misreports totals.
const maxOrderPagesPerPoll = 100

// DHOrdersClient is the subset of dh.Client used by the orders poll scheduler.
type DHOrdersClient interface {
	GetOrders(ctx context.Context, filters dh.OrderFilters) (*dh.OrdersResponse, error)
}

// DHOrdersPollConfig controls the orders poll scheduler.
type DHOrdersPollConfig struct {
	Enabled  bool
	Interval time.Duration
}

// DHOrdersPollScheduler polls DH for completed sales and auto-records them.
type DHOrdersPollScheduler struct {
	StopHandle
	client      DHOrdersClient
	syncState   SyncStateStore
	campaignSvc inventory.ImportService
	logger      observability.Logger
	config      DHOrdersPollConfig
}

// NewDHOrdersPollScheduler creates a new orders poll scheduler.
func NewDHOrdersPollScheduler(
	client DHOrdersClient,
	syncState SyncStateStore,
	campaignSvc inventory.ImportService,
	logger observability.Logger,
	config DHOrdersPollConfig,
) *DHOrdersPollScheduler {
	if config.Interval <= 0 {
		config.Interval = 30 * time.Minute
	}
	return &DHOrdersPollScheduler{
		StopHandle:  NewStopHandle(),
		client:      client,
		syncState:   syncState,
		campaignSvc: campaignSvc,
		logger:      logger.With(context.Background(), observability.String("component", "dh-orders-poll")),
		config:      config,
	}
}

// Start begins the orders poll loop.
func (s *DHOrdersPollScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.WG().Add(1)
		defer s.WG().Done()
		s.logger.Info(ctx, "dh orders poll scheduler disabled")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:     "dh-orders-poll",
		Interval: s.config.Interval,
		WG:       s.WG(),
		StopChan: s.Done(),
		Logger:   s.logger,
	}, s.poll)
}

// poll fetches new DH orders and records them as sales via ImportOrdersSales + ConfirmOrdersSales.
func (s *DHOrdersPollScheduler) poll(ctx context.Context) {
	since, err := s.syncState.Get(ctx, syncStateKeyDHOrdersPoll)
	if err != nil {
		s.logger.Warn(ctx, "failed to read dh orders sync state, defaulting to 24h ago",
			observability.Err(err))
	}
	if since == "" {
		since = time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	}

	allOrders, err := s.fetchAllPages(ctx, since)
	if err != nil {
		s.logger.Warn(ctx, "dh orders poll failed",
			observability.String("since", since),
			observability.Err(err))
		return
	}

	if len(allOrders) == 0 {
		s.logger.Debug(ctx, "dh orders poll: no new orders",
			observability.String("since", since))
		return
	}

	s.logger.Info(ctx, "dh orders poll received orders",
		observability.Int("count", len(allOrders)),
		observability.String("since", since))

	rows := make([]inventory.OrdersExportRow, 0, len(allOrders))
	certToOrderID := make(map[string]string, len(allOrders))

	for _, order := range allOrders {
		rows = append(rows, inventory.OrdersExportRow{
			OrderNumber:  order.OrderID,
			Date:         parseDHSoldAt(order.SoldAt),
			SalesChannel: mapDHChannel(ctx, order.Channel, s.logger),
			ProductTitle: order.CardName,
			Grader:       "PSA",
			CertNumber:   order.CertNumber,
			Grade:        order.Grade,
			UnitPrice:    float64(order.SalePriceCents) / 100.0,
		})
		certToOrderID[order.CertNumber] = order.OrderID
	}

	importResult, err := s.campaignSvc.ImportOrdersSales(ctx, rows)
	if err != nil {
		s.logger.Warn(ctx, "dh orders import failed",
			observability.Err(err))
		return
	}

	for _, skip := range importResult.AlreadySold {
		s.logger.Info(ctx, "dh orders poll: already sold",
			observability.String("cert", skip.CertNumber),
			observability.String("title", skip.ProductTitle))
	}
	for _, skip := range importResult.NotFound {
		s.logger.Warn(ctx, "dh orders poll: cert not found",
			observability.String("cert", skip.CertNumber),
			observability.String("title", skip.ProductTitle))
	}

	if len(importResult.Matched) == 0 {
		s.logger.Info(ctx, "dh orders poll: no new matches to confirm")
		s.updateDHOrdersCheckpoint(ctx, allOrders)
		return
	}

	confirmItems := make([]inventory.OrdersConfirmItem, 0, len(importResult.Matched))
	for _, m := range importResult.Matched {
		confirmItems = append(confirmItems, inventory.OrdersConfirmItem{
			PurchaseID:     m.PurchaseID,
			SaleChannel:    m.SaleChannel,
			SaleDate:       m.SaleDate,
			SalePriceCents: m.SalePriceCents,
			OrderID:        certToOrderID[m.CertNumber],
		})
	}

	bulkResult, err := s.campaignSvc.ConfirmOrdersSales(ctx, confirmItems)
	if err != nil {
		s.logger.Warn(ctx, "dh orders confirm failed",
			observability.Err(err))
		return
	}

	s.logger.Info(ctx, "dh orders poll completed",
		observability.Int("matched", len(importResult.Matched)),
		observability.Int("created", bulkResult.Created),
		observability.Int("failed", bulkResult.Failed),
		observability.Int("alreadySold", len(importResult.AlreadySold)),
		observability.Int("notFound", len(importResult.NotFound)))

	s.updateDHOrdersCheckpoint(ctx, allOrders)
}

// fetchAllPages retrieves all order pages from DH.
func (s *DHOrdersPollScheduler) fetchAllPages(ctx context.Context, since string) ([]dh.Order, error) {
	var allOrders []dh.Order
	page := 1
	for {
		if page > maxOrderPagesPerPoll {
			return nil, fmt.Errorf("fetchAllPages: exceeded max pages (%d), possible API total miscount", maxOrderPagesPerPoll)
		}
		resp, err := s.client.GetOrders(ctx, dh.OrderFilters{Since: since, Page: page, PerPage: 100})
		if err != nil {
			return nil, err
		}
		allOrders = append(allOrders, resp.Orders...)
		if len(allOrders) >= resp.Meta.TotalCount || len(resp.Orders) == 0 {
			break
		}
		page++
	}
	return allOrders, nil
}

// updateDHOrdersCheckpoint advances the sync state to the latest sold_at timestamp.
func (s *DHOrdersPollScheduler) updateDHOrdersCheckpoint(ctx context.Context, orders []dh.Order) {
	latest := findLatestSoldAt(orders)
	if latest == "" {
		return
	}
	if err := s.syncState.Set(ctx, syncStateKeyDHOrdersPoll, latest); err != nil {
		s.logger.Warn(ctx, "failed to update dh orders sync state",
			observability.String("timestamp", latest),
			observability.Err(err))
	}
}

// mapDHChannel converts a DH channel string to an inventory.SaleChannel.
// Unknown channels default to SaleChannelOther with a Warn log — never drop
// the order, but surface the miscategorization so it can be fixed.
func mapDHChannel(ctx context.Context, channel string, logger observability.Logger) inventory.SaleChannel {
	switch channel {
	case "dh":
		return inventory.SaleChannelDoubleHolo
	case "ebay":
		return inventory.SaleChannelEbay
	case "shopify":
		return inventory.SaleChannelWebsite
	default:
		logger.Warn(ctx, "dh orders poll: unknown channel, defaulting to 'other'",
			observability.String("channel", channel))
		return inventory.SaleChannelOther
	}
}

// parseDHSoldAt parses an RFC3339 timestamp to "2006-01-02" format.
func parseDHSoldAt(soldAt string) string {
	t, err := time.Parse(time.RFC3339, soldAt)
	if err != nil {
		return soldAt
	}
	return t.Format("2006-01-02")
}

// findLatestSoldAt returns the lexicographically greatest sold_at value from the orders.
func findLatestSoldAt(orders []dh.Order) string {
	if len(orders) == 0 {
		return ""
	}
	latest := orders[0].SoldAt
	for _, o := range orders[1:] {
		if o.SoldAt > latest {
			latest = o.SoldAt
		}
	}
	return latest
}

// Compile-time check that dh.Client satisfies DHOrdersClient.
var _ DHOrdersClient = (*dh.Client)(nil)
