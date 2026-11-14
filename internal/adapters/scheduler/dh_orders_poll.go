package scheduler

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/csvimport"
	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

const syncStateKeyDHOrdersPoll = "dh_orders_last_poll"

// DH releases held orders with their original sold_at, not their release time.
const ordersPollLookback = 7 * 24 * time.Hour

// maxOrderPagesPerPoll prevents unbounded pagination if the DH API misreports totals.
const maxOrderPagesPerPoll = 100

// DHOrdersPollSummary captures the outcome of one ingest pass, returned by
// both the scheduled poll loop and the manual /api/dh/ingest-orders handler.
type DHOrdersPollSummary struct {
	Since         string // the since timestamp that was used
	OrdersFetched int    // total orders returned by DH
	Matched       int    // newly created sales (from BulkSaleResult.Created)
	AlreadySold   int    // skipped: purchase already has a sale (len(importResult.AlreadySold))
	Returned      int    // retained confirmed-return orders skipped
	NotFound      int    // skipped: cert not found in local system (len(importResult.NotFound))
	Failed        int    // bulk sale failures (BulkSaleResult.Failed)
	LatestSoldAt  string // max sold_at observed across the batch (empty if no orders)
}

// DHOrdersClient is the subset of dh.Client used by the orders poll scheduler.
type DHOrdersClient interface {
	GetOrders(ctx context.Context, filters dh.OrderFilters) (*dh.OrdersResponse, error)
}

// DHOrdersImporter is the slice of CSV/orders intake this scheduler needs: match
// DH order rows against inventory, then record the confirmed ones as sales.
type DHOrdersImporter interface {
	ImportOrdersSales(ctx context.Context, rows []csvimport.OrdersExportRow) (*csvimport.OrdersImportResult, error)
	ConfirmOrdersSales(ctx context.Context, items []csvimport.OrdersConfirmItem) (*inventory.BulkSaleResult, error)
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
	onFirstRun  func()
	syncState   SyncStateStore
	campaignSvc DHOrdersImporter
	eventRec    dhevents.Recorder // may be nil
	logger      observability.Logger
	config      DHOrdersPollConfig
}

// NewDHOrdersPollScheduler creates a new orders poll scheduler.
func NewDHOrdersPollScheduler(
	client DHOrdersClient,
	syncState SyncStateStore,
	campaignSvc DHOrdersImporter,
	eventRec dhevents.Recorder, // may be nil for tests / unwired
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
		eventRec:    eventRec,
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
		Name:       "dh-orders-poll",
		OnFirstRun: s.onFirstRun,
		Interval:   s.config.Interval,
		WG:         s.WG(),
		StopChan:   s.Done(),
		Logger:     s.logger,
	}, s.poll)
}

// recordEvent emits an event to the recorder if present. Failures are logged but do not abort.
func (s *DHOrdersPollScheduler) recordEvent(ctx context.Context, e dhevents.Event) {
	if s.eventRec == nil {
		return
	}
	if err := s.eventRec.Record(ctx, e); err != nil {
		s.logger.Warn(ctx, "dh orders poll: record event failed",
			observability.String("type", string(e.Type)),
			observability.Err(err))
	}
}

// RunOnce executes one ingest pass against DH orders with a caller-supplied
// since timestamp. Returns a summary struct describing the outcome. Does NOT
// advance the sync-state checkpoint — callers that want persistence (e.g.
// the scheduled poll loop) are responsible for calling syncState.Set after
// inspecting the returned LatestSoldAt.
//
// Used by both the scheduled poll loop (via poll()) and the manual
// POST /api/dh/ingest-orders endpoint.
func (s *DHOrdersPollScheduler) RunOnce(ctx context.Context, since string) (*DHOrdersPollSummary, error) {
	return s.runOnce(ctx, since, time.Time{})
}

// notifyAfter quiets scheduled replays without suppressing newly created sales.
// A zero cutoff preserves the explicit RunOnce caller's per-order diagnostics.
func (s *DHOrdersPollScheduler) runOnce(ctx context.Context, since string, notifyAfter time.Time) (*DHOrdersPollSummary, error) {
	summary := &DHOrdersPollSummary{Since: since}

	allOrders, err := s.fetchAllPages(ctx, since)
	if err != nil {
		return summary, fmt.Errorf("fetch all pages: %w", err)
	}
	summary.OrdersFetched = len(allOrders)
	if len(allOrders) == 0 {
		return summary, nil
	}

	rows := make([]csvimport.OrdersExportRow, 0, len(allOrders))
	notify := make(map[string]bool, len(allOrders))
	for _, order := range allOrders {
		soldAt, _ := time.Parse(time.RFC3339, order.SoldAt)
		firstSighting := notifyAfter.IsZero() || soldAt.After(notifyAfter)
		notify[order.OrderID] = firstSighting
		grade, err := strconv.ParseFloat(order.Grade, 64)
		if err != nil && firstSighting {
			s.logger.Warn(ctx, "dh orders poll: could not parse grade, falling back to 0",
				observability.String("orderID", order.OrderID),
				observability.String("cert", order.CertNumber),
				observability.String("grade", order.Grade),
				observability.Err(err))
		}
		var rowLogger observability.Logger
		if firstSighting {
			rowLogger = s.logger
		}
		rows = append(rows, csvimport.OrdersExportRow{
			OrderNumber:  order.OrderID,
			Date:         parseDHSoldAt(order.SoldAt),
			SalesChannel: mapDHChannel(ctx, order.Channel, rowLogger),
			ProductTitle: order.CardName,
			Grader:       "PSA",
			CertNumber:   order.CertNumber,
			Grade:        grade,
			UnitPrice:    float64(order.SalePriceCents) / 100.0,
		})
	}

	importResult, err := s.campaignSvc.ImportOrdersSales(ctx, rows)
	if err != nil {
		return summary, fmt.Errorf("import orders: %w", err)
	}
	summary.AlreadySold = len(importResult.AlreadySold)
	summary.NotFound = len(importResult.NotFound)
	summary.LatestSoldAt = findLatestSoldAt(allOrders)
	for _, skipped := range importResult.Skipped {
		if skipped.Reason == "returned_order" {
			summary.Returned++
		}
	}

	// Skip events describe first sightings, not every lookback replay. Late
	// orphans at/before the cursor remain visible in the poll summary only.
	for _, nf := range importResult.NotFound {
		if !notify[nf.OrderID] {
			continue
		}
		s.recordEvent(ctx, dhevents.Event{
			CertNumber:     nf.CertNumber,
			Type:           dhevents.TypeOrphanSale,
			DHOrderID:      nf.OrderID,
			SalePriceCents: nf.SalePriceCents,
			Source:         dhevents.SourceDHOrdersPoll,
			Notes:          "no local purchase matched this cert",
		})
	}
	for _, as := range importResult.AlreadySold {
		if !notify[as.OrderID] {
			continue
		}
		s.recordEvent(ctx, dhevents.Event{
			CertNumber: as.CertNumber,
			Type:       dhevents.TypeAlreadySold,
			DHOrderID:  as.OrderID,
			Source:     dhevents.SourceDHOrdersPoll,
		})
	}

	if len(importResult.Matched) == 0 {
		return summary, nil
	}

	confirmItems := make([]csvimport.OrdersConfirmItem, 0, len(importResult.Matched))
	eventCandidates := make([]csvimport.OrdersImportMatch, 0, len(importResult.Matched))
	seenPurchases := map[string]bool{}
	for _, m := range importResult.Matched {
		// The bulk result identifies failures by purchase, not order. Confirm
		// one original row per purchase so a later duplicate cannot hide the
		// successful first row's event; keep the existing duplicate failure count.
		if seenPurchases[m.PurchaseID] {
			summary.Failed++
			continue
		}
		seenPurchases[m.PurchaseID] = true
		eventCandidates = append(eventCandidates, m)
		confirmItems = append(confirmItems, csvimport.OrdersConfirmItem{
			PurchaseID:     m.PurchaseID,
			SaleChannel:    m.SaleChannel,
			SaleDate:       m.SaleDate,
			SalePriceCents: m.SalePriceCents,
			OrderID:        m.OrderID,
		})
	}

	bulkResult, err := s.campaignSvc.ConfirmOrdersSales(ctx, confirmItems)
	if err != nil {
		return summary, fmt.Errorf("confirm orders: %w", err)
	}
	summary.Matched = bulkResult.Created
	summary.Failed += bulkResult.Failed

	// Emit sold events only for items where sale creation succeeded. Items that
	// failed during ConfirmOrdersSales must NOT emit a sold event.
	failedIDs := make(map[string]bool, len(bulkResult.Errors))
	for _, e := range bulkResult.Errors {
		failedIDs[e.PurchaseID] = true
	}
	for _, m := range eventCandidates {
		if failedIDs[m.PurchaseID] {
			continue
		}
		s.recordEvent(ctx, dhevents.Event{
			PurchaseID:     m.PurchaseID,
			CertNumber:     m.CertNumber,
			Type:           dhevents.TypeSold,
			NewDHStatus:    string(inventory.DHStatusSold),
			DHOrderID:      m.OrderID,
			SalePriceCents: m.SalePriceCents,
			Source:         dhevents.SourceDHOrdersPoll,
		})
	}

	return summary, nil
}

// poll replays a bounded window before the high-water mark to recover late
// orders. Successful passes may advance the checkpoint, never move it backward.
func (s *DHOrdersPollScheduler) poll(ctx context.Context) {
	since, err := s.syncState.Get(ctx, syncStateKeyDHOrdersPoll)
	if err != nil {
		s.logger.Warn(ctx, "dh orders poll: cursor read failed")
		return
	}
	firstRun := since == ""
	if firstRun {
		since = time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
		// Persist the replay bound before ingest, including when the first pass fails.
		if err := s.syncState.Set(ctx, syncStateKeyDHOrdersPoll, since); err != nil {
			s.logger.Warn(ctx, "dh orders poll: cursor seed failed")
			return
		}
	}

	cursor, err := time.Parse(time.RFC3339, since)
	if err != nil {
		s.logger.Warn(ctx, "dh orders poll: invalid cursor")
		return
	}
	if !firstRun {
		since = cursor.Add(-ordersPollLookback).UTC().Format(time.RFC3339Nano)
	}

	summary, runErr := s.runOnce(ctx, since, cursor)
	if runErr != nil {
		s.logger.Warn(ctx, "dh orders poll failed",
			observability.String("since", since),
			observability.Err(runErr))
		return
	}

	s.logger.Info(ctx, "dh orders poll completed",
		observability.Int("orders", summary.OrdersFetched),
		observability.Int("matched", summary.Matched),
		observability.Int("already_sold", summary.AlreadySold),
		observability.Int("not_found", summary.NotFound),
		observability.Int("returned_order", summary.Returned),
		observability.Int("failed", summary.Failed))

	if summary.Failed > 0 || ctx.Err() != nil {
		return
	}
	latest, _ := time.Parse(time.RFC3339, summary.LatestSoldAt)
	if latest.After(cursor) {
		if setErr := s.syncState.Set(ctx, syncStateKeyDHOrdersPoll, summary.LatestSoldAt); setErr != nil {
			s.logger.Warn(ctx, "dh orders poll: cursor write failed")
			return
		}
	}
	if err := s.syncState.Set(ctx, syncStateKeyDHOrdersClean, time.Now().UTC().Format(time.RFC3339)); err != nil {
		s.logger.Warn(ctx, "dh orders poll: clean marker write failed")
	}
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
		if logger != nil {
			logger.Warn(ctx, "dh orders poll: unknown channel, defaulting to 'other'",
				observability.String("channel", channel))
		}
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

// findLatestSoldAt returns the chronologically newest valid sold_at. Invalid
// timestamps cannot become a checkpoint that stalls the next poll.
func findLatestSoldAt(orders []dh.Order) string {
	var latest time.Time
	var value string
	for _, o := range orders {
		at, err := time.Parse(time.RFC3339, o.SoldAt)
		if err == nil && at.After(latest) {
			latest, value = at, o.SoldAt
		}
	}
	return value
}

// Compile-time check that dh.Client satisfies DHOrdersClient.
var _ DHOrdersClient = (*dh.Client)(nil)
