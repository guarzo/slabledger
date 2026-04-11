package scheduler

import (
	"context"
	"strconv"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// CL failure reason tags. Short, stable strings used by the /failures admin endpoint.
const (
	CLReasonNoImageMatch = "no_image_match"
	CLReasonNoCertMatch  = "no_cert_match"
	CLReasonNoValue      = "no_value"
	CLReasonAPIError     = "api_error"
)

// recordCLError persists a failure reason (or clears it when reason=="") on a
// purchase. Logs warnings but never fails the refresh loop — diagnostics are
// best-effort.
func (s *CardLadderRefreshScheduler) recordCLError(ctx context.Context, purchaseID, reason string) {
	var reasonAt string
	if reason != "" {
		reasonAt = time.Now().UTC().Format(time.RFC3339)
	}
	if err := s.valueUpdater.UpdatePurchaseCLError(ctx, purchaseID, reason, reasonAt); err != nil {
		s.logger.Debug(ctx, "CL refresh: failed to persist error reason",
			observability.String("purchaseId", purchaseID),
			observability.String("reason", reason),
			observability.Err(err))
	}
}

// extractGradeValue parses "PSA 9", "PSA 9.5", or "g9" → numeric grade value.
func extractGradeValue(condition string) float64 {
	if m := gradeDigitsRe.FindString(condition); m != "" {
		v, _ := strconv.ParseFloat(m, 64)
		return v
	}
	return 0
}
