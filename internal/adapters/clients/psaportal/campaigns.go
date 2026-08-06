package psaportal

import (
	"context"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

// FetchCampaigns returns all portal campaigns with buy-box + member lists,
// paginating the list endpoint and enriching each item with its edit-form
// subject/publisher/spec-list filters. It also returns the curated spec-list
// catalog embedded in the edit-form responses — the harvester persists it via
// psacampaign.CatalogStore without a second portal round trip.
func (c *Client) FetchCampaigns(ctx context.Context) ([]psacampaign.PortalCampaign, []psacampaign.SpecListRef, error) {
	var out []psacampaign.PortalCampaign
	var catalog []psacampaign.SpecListRef
	page := 1
	for {
		root, err := c.getRefPacked(ctx, fmt.Sprintf("%s%s&page=%d", c.baseURL(), campaignsListPath, page))
		if err != nil {
			return nil, nil, err
		}
		items, pageSize, totalCount, err := campaignItems(root)
		if err != nil {
			return nil, nil, err
		}
		for _, it := range items {
			pc, err := mapListItem(it)
			if err != nil {
				c.logger.Warn(ctx, "psaportal: skipping malformed campaign", observability.Err(err))
				continue
			}
			fd, specLists, err := c.fetchCampaignFormData(ctx, pc.CampaignRequestID)
			if err != nil {
				// TargetingComplete stays false: pc carries zero targeting,
				// and the baseline pull must refuse to write it into the
				// campaign row rather than overwrite real targeting with an
				// open net. The snapshot store's empty-save guard
				// (psa_campaign_snapshot_store.go:29-30) only checks
				// len(campaigns) == 0, so it does NOT catch this — a fleet
				// of zero-targeting campaigns would sail straight through it.
				c.logger.Warn(ctx, "psaportal: edit fetch failed",
					observability.String("campaign_id", pc.CampaignRequestID), observability.Err(err))
			} else {
				applyFormData(&pc, fd, specLists)
				if len(specLists) > 0 {
					catalog = specLists
				}
			}
			out = append(out, pc)
		}
		if len(items) == 0 || len(items) < pageSize || len(out) >= totalCount {
			break
		}
		page++
	}
	return out, catalog, nil
}

// fetchCampaignFormData fetches and decodes the edit-page formData for one
// campaign, along with the curated spec-list catalog embedded alongside it.
func (c *Client) fetchCampaignFormData(ctx context.Context, campaignID string) (psacampaign.CampaignFormData, []psacampaign.SpecListRef, error) {
	url := c.baseURL() + fmt.Sprintf(campaignEditPathF, campaignID)
	root, err := c.getRefPacked(ctx, url)
	if err != nil {
		return psacampaign.CampaignFormData{}, nil, err
	}
	m, ok := root.(map[string]any)
	if !ok {
		return psacampaign.CampaignFormData{}, nil, fmt.Errorf("psaportal: edit root not an object")
	}
	fdRaw, ok := m["formData"]
	if !ok {
		return psacampaign.CampaignFormData{}, nil, fmt.Errorf("psaportal: edit response missing formData")
	}
	fd, ok := fdRaw.(map[string]any)
	if !ok {
		return psacampaign.CampaignFormData{}, nil, fmt.Errorf("psaportal: formData not an object")
	}
	formData, err := decodeFormData(fd)
	if err != nil {
		return psacampaign.CampaignFormData{}, nil, err
	}
	return formData, decodeSpecLists(m["prepackagedSpecLists"]), nil
}

// getRefPacked fetches url via the browser session and resolves the SvelteKit
// ref-packed envelope into a plain value.
func (c *Client) getRefPacked(ctx context.Context, url string) (any, error) {
	resp, err := c.fetch.Do(ctx, FetchRequest{URL: url, Method: "GET"})
	if err != nil {
		return nil, fmt.Errorf("psaportal: campaign fetch: %w", err)
	}
	if resp.Status != 200 {
		return nil, fmt.Errorf("psaportal: campaign fetch status %d", resp.Status)
	}
	packed, err := packedFromEnvelope([]byte(resp.Body))
	if err != nil {
		return nil, err
	}
	return DecodeRefPacked(packed)
}

// campaignItems walks the list response to campaignsResponse.items[] plus paging info.
func campaignItems(root any) (items []any, pageSize, totalCount int, err error) {
	m, ok := root.(map[string]any)
	if !ok {
		return nil, 0, 0, fmt.Errorf("psaportal: list root not an object")
	}
	crRaw, ok := m["campaignsResponse"]
	if !ok {
		return nil, 0, 0, fmt.Errorf("psaportal: list response missing campaignsResponse")
	}
	cr, ok := crRaw.(map[string]any)
	if !ok {
		return nil, 0, 0, fmt.Errorf("psaportal: campaignsResponse not an object")
	}
	itemsRaw, _ := cr["items"].([]any)
	pageSize = asInt(cr["pageSize"])
	if pageSize <= 0 {
		return nil, 0, 0, fmt.Errorf("psaportal: invalid pageSize %d in campaign list response", pageSize)
	}
	return itemsRaw, pageSize, asInt(cr["totalCount"]), nil
}

// mapListItem maps one campaignsResponse.items[] entry into a PortalCampaign.
func mapListItem(itRaw any) (psacampaign.PortalCampaign, error) {
	it, ok := itRaw.(map[string]any)
	if !ok {
		return psacampaign.PortalCampaign{}, fmt.Errorf("psaportal: campaign item not an object")
	}
	buyBox, _ := it["buyBox"].(map[string]any)
	budget, _ := it["budget"].(map[string]any)

	pc := psacampaign.PortalCampaign{
		CampaignRequestID: asString(it["campaignRequestId"]),
		Name:              asString(it["campaignName"]),
		Type:              asString(it["campaignType"]),
		Status:            asString(it["status"]),
		Category:          asString(it["category"]),
		BuyPercentClv:     asInt(it["buyerPricePercentClv"]),
		BuyBox: psacampaign.CampaignBuyBox{
			GradeMin: asString(buyBox["gradeMin"]),
			GradeMax: asString(buyBox["gradeMax"]),
			YearMin:  asInt(buyBox["yearMin"]),
			YearMax:  asInt(buyBox["yearMax"]),
			// priceMin/priceMax/buyerFlatFee arrive as whole-USD integers on the
			// wire (confirmed via docs/psa-campaigns-raw.json, e.g. 500, 3000);
			// decimal-USD has not been observed. asIntCents converts to cents.
			PriceMinCents:     asIntCents(buyBox["priceMin"]),
			PriceMaxCents:     asIntCents(buyBox["priceMax"]),
			ClvConfidenceMin:  asInt(buyBox["clvConfidenceMin"]),
			BuyerFlatFeeCents: asIntCents(buyBox["buyerFlatFee"]),
		},
		DailyBudgetCents: asIntCents(budget["dailyBudget"]),
		DailySpecLimit:   asInt(budget["dailySpecQuantityLimit"]),
		CreatedAt:        asTime(it["createdAt"]),
		UpdatedAt:        asTime(it["updatedAt"]),
	}
	if pc.CampaignRequestID == "" {
		return psacampaign.PortalCampaign{}, fmt.Errorf("psaportal: campaign item missing campaignRequestId")
	}
	return pc, nil
}

// applyFormData fills the subject/publisher/spec-list filters on pc from the
// edit-form data and the sibling spec-list catalog, then marks the record
// complete. SpecListNames resolves SpecListIDs against catalog at decode time
// so the snapshot stays human-readable across a PSA re-key of the UUIDs.
func applyFormData(pc *psacampaign.PortalCampaign, fd psacampaign.CampaignFormData, catalog []psacampaign.SpecListRef) {
	pc.SubjectFilter = psacampaign.CampaignFilter{
		Type:     fd.SubjectFilterType,
		Subjects: fd.SelectedSubjects,
	}
	pc.PublisherFilter = psacampaign.CampaignFilter{
		Type:     fd.PublisherFilterType,
		Subjects: fd.SelectedPublishers,
	}
	pc.SpecListIDs = fd.PrepackagedSpecListIDs
	pc.SpecListNames = specListNames(fd.PrepackagedSpecListIDs, catalog)
	pc.DeniedSpecs = fd.DeniedSpecs
	pc.TargetingComplete = true
}

// specListNames resolves each id against catalog, skipping ids the catalog
// does not (yet) explain rather than failing the whole decode.
func specListNames(ids []string, catalog []psacampaign.SpecListRef) []string {
	byID := make(map[string]string, len(catalog))
	for _, ref := range catalog {
		byID[ref.ID] = ref.Name
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if name, ok := byID[id]; ok {
			out = append(out, name)
		}
	}
	return out
}
