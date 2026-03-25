package psa

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// PurchaseLister can list purchases missing images.
type PurchaseLister interface {
	ListPurchasesMissingImages(ctx context.Context, limit int) ([]PurchaseImageRow, error)
}

// PurchaseImageRow holds the minimal fields needed for image backfill.
type PurchaseImageRow struct {
	ID         string
	CertNumber string
}

// ImageUpdater can update purchase image URLs.
type ImageUpdater interface {
	UpdatePurchaseImageURLs(ctx context.Context, id, frontURL, backURL string) error
}

// ImageBackfiller fetches PSA slab images for purchases missing front_image_url.
type ImageBackfiller struct {
	client  *Client
	lister  PurchaseLister
	updater ImageUpdater
	logger  observability.Logger
}

// NewImageBackfiller creates a new image backfiller.
func NewImageBackfiller(client *Client, lister PurchaseLister, updater ImageUpdater, logger observability.Logger) *ImageBackfiller {
	return &ImageBackfiller{
		client:  client,
		lister:  lister,
		updater: updater,
		logger:  logger,
	}
}

// BackfillImages fetches images for purchases that are missing images,
// processing at most 80 per call to respect PSA API rate limits.
func (b *ImageBackfiller) BackfillImages(ctx context.Context) (updated int, errors int, err error) {
	purchases, err := b.lister.ListPurchasesMissingImages(ctx, 80)
	if err != nil {
		return 0, 0, err
	}

	if len(purchases) == 0 {
		b.logger.Info(ctx, "image backfill: no purchases missing images")
		return 0, 0, nil
	}

	b.logger.Info(ctx, "image backfill: starting",
		observability.Int("count", len(purchases)))

	for i, p := range purchases {
		if ctx.Err() != nil {
			return updated, errors, ctx.Err()
		}

		// Throttle between iterations (skip the first to avoid initial delay)
		if i > 0 {
			select {
			case <-ctx.Done():
				return updated, errors, ctx.Err()
			case <-time.After(600 * time.Millisecond):
			}
		}

		images, fetchErr := b.client.GetImages(ctx, p.CertNumber)
		if fetchErr != nil {
			b.logger.Error(ctx, "image backfill: fetch failed",
				observability.String("cert", p.CertNumber),
				observability.Err(fetchErr))
			errors++
			continue
		}

		var frontURL, backURL string
		for _, img := range images {
			if img.IsFrontImage && frontURL == "" {
				frontURL = img.ImageURL
			} else if !img.IsFrontImage && backURL == "" {
				backURL = img.ImageURL
			}
		}

		if frontURL == "" {
			b.logger.Info(ctx, "image backfill: no front image found",
				observability.String("cert", p.CertNumber),
				observability.Int("imagesReturned", len(images)))
			errors++
			continue
		}

		if updateErr := b.updater.UpdatePurchaseImageURLs(ctx, p.ID, frontURL, backURL); updateErr != nil {
			b.logger.Error(ctx, "image backfill: update failed",
				observability.String("id", p.ID),
				observability.Err(updateErr))
			errors++
			continue
		}

		updated++
	}

	b.logger.Info(ctx, "image backfill: completed",
		observability.Int("updated", updated),
		observability.Int("errors", errors))

	return updated, errors, nil
}
