package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// triggerDHListing runs in the background so it doesn't delay the HTTP response.
func (h *CampaignsHandler) triggerDHListing(certNumbers []string) {
	if h.dhListingSvc == nil || len(certNumbers) == 0 {
		return
	}

	h.bgWG.Add(1)
	go func() {
		defer h.bgWG.Done()
		defer func() {
			if r := recover(); r != nil {
				h.logger.Error(h.baseCtx, "panic in triggerDHListing",
					observability.String("panic", fmt.Sprintf("%v", r)))
			}
		}()
		ctx, cancel := context.WithTimeout(h.baseCtx, 5*time.Minute)
		defer cancel()

		result := h.dhListingSvc.ListPurchases(ctx, certNumbers)
		h.logger.Info(ctx, "dh listing goroutine completed",
			observability.Int("listed", result.Listed),
			observability.Int("synced", result.Synced),
			observability.Int("total", result.Total))
	}()
}
