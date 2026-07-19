package psaportal

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

// transientPushError reports whether a portal push failed on an edge/transport
// condition that a later run with a fresh browser session routinely clears —
// Cloudflare block/challenge (403), rate limiting (429), or a portal outage
// (503). Observed 2026-07-18: one run's every GET drew an instant Cloudflare
// 403 while runs minutes later were clean; marking such rows terminally failed
// is what left approved pushes permanently unsent. All portal client errors
// carry the HTTP code as a "status <code>" suffix (push.go, create.go,
// buildhash.go), so match on that. App-level rejections (400/422/…) stay
// terminal.
func transientPushError(err error) bool {
	msg := err.Error()
	for _, s := range []string{"status 403", "status 429", "status 503"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// pushOutcome returns the queue status a failed push should land in: back to
// approved for transient transport errors (retried by the next hourly drain;
// creates are safe to retry via the linker idempotency guard), failed
// otherwise.
func pushOutcome(err error) psacampaign.PushStatus {
	if transientPushError(err) {
		return psacampaign.PushApproved
	}
	return psacampaign.PushFailed
}

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
			outcome := pushOutcome(err)
			if markErr := q.MarkResult(ctx, row.ID, outcome, "", err.Error()); markErr != nil {
				logger.Error(ctx, "psaportal: mark push failed result failed",
					observability.String("row_id", row.ID), observability.Err(markErr))
			}
			logger.Error(ctx, "psaportal: push campaign failed",
				observability.String("row_id", row.ID),
				observability.String("psa_campaign_id", row.PSACampaignID),
				observability.String("outcome", string(outcome)),
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

	// Idempotency guard: if a prior attempt on this row already created and
	// linked a portal campaign (but failed to record its result), the internal
	// campaign carries the portal id. Re-recording the existing id avoids
	// creating a duplicate portal campaign on retry.
	if linker != nil && row.InternalCampaignID != "" {
		existingID, err := linker.LinkedPSACampaignID(ctx, row.InternalCampaignID)
		if err != nil {
			logger.Error(ctx, "psaportal: idempotency lookup failed, aborting create to avoid duplicate",
				observability.String("row_id", row.ID),
				observability.String("internal_campaign_id", row.InternalCampaignID),
				observability.Err(err))
			if markErr := q.MarkResult(ctx, row.ID, psacampaign.PushFailed, "", "idempotency lookup failed: "+err.Error()); markErr != nil {
				logger.Error(ctx, "psaportal: mark create idempotency-failed result failed", observability.String("row_id", row.ID), observability.Err(markErr))
			}
			return false
		}
		if existingID != "" {
			logger.Info(ctx, "psaportal: create row already linked, recording existing id without re-creating",
				observability.String("row_id", row.ID),
				observability.String("internal_campaign_id", row.InternalCampaignID),
				observability.String("psa_campaign_id", existingID))
			resultJSON, _ := json.Marshal(map[string]string{"campaignRequestId": existingID})
			if err := q.MarkResult(ctx, row.ID, psacampaign.PushPushed, string(resultJSON), ""); err != nil {
				logger.Error(ctx, "psaportal: mark create pushed (idempotent) result failed", observability.String("row_id", row.ID), observability.Err(err))
			}
			return true
		}
	}

	newID, err := c.CreateCampaign(ctx, *row.Diff.Create)
	if err != nil {
		outcome := pushOutcome(err)
		if markErr := q.MarkResult(ctx, row.ID, outcome, "", err.Error()); markErr != nil {
			logger.Error(ctx, "psaportal: mark create failed result failed", observability.String("row_id", row.ID), observability.Err(markErr))
		}
		logger.Error(ctx, "psaportal: create campaign failed",
			observability.String("row_id", row.ID),
			observability.String("outcome", string(outcome)),
			observability.Err(err))
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
	} else if linker != nil {
		// A create row should always carry an internal campaign id; missing one
		// means an upstream bug and leaves the new portal campaign unlinked.
		logger.Error(ctx, "psaportal: create row missing internal_campaign_id, cannot link",
			observability.String("row_id", row.ID),
			observability.String("psa_campaign_id", newID))
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
