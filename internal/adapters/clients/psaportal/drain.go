package psaportal

import (
	"context"
	"encoding/json"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

// DrainPushQueue pushes all approved rows in q to the PSA portal via c,
// marking each row pushed or failed based on the outcome. It returns the
// count of successful and failed pushes.
func DrainPushQueue(ctx context.Context, c *Client, q psacampaign.PushQueueStore, linker psacampaign.CampaignLinker, logger observability.Logger) (pushed, failed int) {
	rows, err := q.ListByStatus(ctx, psacampaign.PushApproved)
	if err != nil {
		logger.Error(ctx, "psaportal: list approved push rows failed", observability.Err(err))
		return 0, 0
	}

	for _, row := range rows {
		claimed, err := q.Claim(ctx, row.ID)
		if err != nil {
			logger.Error(ctx, "psaportal: claim push row failed",
				observability.String("row_id", row.ID), observability.Err(err))
			continue
		}
		if !claimed {
			logger.Info(ctx, "psaportal: push row already claimed, skipping",
				observability.String("row_id", row.ID))
			continue
		}

		if row.Operation == psacampaign.OpCreate {
			if drainCreate(ctx, c, q, linker, row, logger) {
				pushed++
			} else {
				failed++
			}
			continue
		}

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

// drainCreate executes one approved create row: creates the portal campaign,
// links the new id back onto the internal campaign, and records the outcome.
// Returns true on success.
func drainCreate(ctx context.Context, c *Client, q psacampaign.PushQueueStore, linker psacampaign.CampaignLinker, row psacampaign.PushRow, logger observability.Logger) bool {
	if row.Diff.Create == nil {
		if err := q.MarkResult(ctx, row.ID, psacampaign.PushFailed, "", "create row missing formData"); err != nil {
			logger.Error(ctx, "psaportal: mark create-missing-formData failed", observability.String("row_id", row.ID), observability.Err(err))
		}
		return false
	}

	newID, err := c.CreateCampaign(ctx, *row.Diff.Create)
	if err != nil {
		if markErr := q.MarkResult(ctx, row.ID, psacampaign.PushFailed, "", err.Error()); markErr != nil {
			logger.Error(ctx, "psaportal: mark create failed result failed", observability.String("row_id", row.ID), observability.Err(markErr))
		}
		logger.Error(ctx, "psaportal: create campaign failed", observability.String("row_id", row.ID), observability.Err(err))
		return false
	}

	if linker != nil && row.InternalCampaignID != "" {
		if err := linker.LinkPSACampaign(ctx, row.InternalCampaignID, newID); err != nil {
			// The portal campaign exists; surface but don't fail the row — the
			// operator can link manually via psa-link.
			logger.Error(ctx, "psaportal: link created campaign failed",
				observability.String("row_id", row.ID),
				observability.String("internal_campaign_id", row.InternalCampaignID),
				observability.String("psa_campaign_id", newID),
				observability.Err(err))
		}
	}

	resultJSON, _ := json.Marshal(map[string]string{"campaignRequestId": newID})
	if err := q.MarkResult(ctx, row.ID, psacampaign.PushPushed, string(resultJSON), ""); err != nil {
		logger.Error(ctx, "psaportal: mark create pushed result failed", observability.String("row_id", row.ID), observability.Err(err))
	}
	logger.Info(ctx, "psaportal: portal campaign created",
		observability.String("row_id", row.ID),
		observability.String("psa_campaign_id", newID))
	return true
}
