package psaportal

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

// DrainPushQueue pushes all approved rows in q to the PSA portal via c,
// marking each row pushed or failed based on the outcome. It returns the
// count of successful and failed pushes.
func DrainPushQueue(ctx context.Context, c *Client, q psacampaign.PushQueueStore, logger observability.Logger) (pushed, failed int) {
	rows, err := q.ListByStatus(ctx, psacampaign.PushApproved)
	if err != nil {
		logger.Error(ctx, "psaportal: list approved push rows failed", observability.Err(err))
		return 0, 0
	}

	for _, row := range rows {
		if err := c.PushCampaign(ctx, row.PSACampaignID, row.Diff.Changes); err != nil {
			if markErr := q.MarkResult(ctx, row.ID, psacampaign.PushFailed, "", err.Error()); markErr != nil {
				logger.Error(ctx, "psaportal: mark push failed result failed",
					observability.String("row_id", row.ID), observability.Err(markErr))
			}
			logger.Error(ctx, "psaportal: push campaign failed",
				observability.String("row_id", row.ID),
				observability.String("psa_campaign_id", row.PSACampaignID),
				observability.Err(err))
			failed++
			continue
		}

		if markErr := q.MarkResult(ctx, row.ID, psacampaign.PushPushed, "", ""); markErr != nil {
			logger.Error(ctx, "psaportal: mark push pushed result failed",
				observability.String("row_id", row.ID), observability.Err(markErr))
		}
		pushed++
	}

	logger.Info(ctx, "psaportal: push queue drained",
		observability.Int("pushed", pushed), observability.Int("failed", failed))
	return pushed, failed
}
