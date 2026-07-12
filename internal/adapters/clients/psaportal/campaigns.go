package psaportal

import (
	"context"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

// FetchCampaigns returns all portal campaigns with buy-box + member lists,
// paginating the list endpoint and enriching each item with its edit-form
// subject/publisher filters.
func (c *Client) FetchCampaigns(ctx context.Context) ([]psacampaign.PortalCampaign, error) {
	token, err := c.tokens.AccessToken(ctx)
	if err != nil {
		return nil, err
	}

	var out []psacampaign.PortalCampaign
	page := 1
	for {
		root, err := c.getRefPacked(ctx, token, fmt.Sprintf("%s%s&page=%d", c.baseURL(), campaignsListPath, page))
		if err != nil {
			return nil, err
		}
		items, pageSize, totalCount, err := campaignItems(root)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			pc, err := mapListItem(it)
			if err != nil {
				c.logger.Warn(ctx, "psaportal: skipping malformed campaign", observability.Err(err))
				continue
			}
			fd, err := c.fetchCampaignFormData(ctx, token, pc.CampaignRequestID)
			if err != nil {
				c.logger.Warn(ctx, "psaportal: edit fetch failed",
					observability.String("campaign_id", pc.CampaignRequestID), observability.Err(err))
			} else {
				applyFormData(&pc, fd)
			}
			out = append(out, pc)
		}
		if len(items) == 0 || len(items) < pageSize || len(out) >= totalCount {
			break
		}
		page++
	}
	return out, nil
}

// fetchCampaignFormData fetches and decodes the edit-page formData for one campaign.
func (c *Client) fetchCampaignFormData(ctx context.Context, token, campaignID string) (psacampaign.CampaignFormData, error) {
	url := c.baseURL() + fmt.Sprintf(campaignEditPathF, campaignID)
	root, err := c.getRefPacked(ctx, token, url)
	if err != nil {
		return psacampaign.CampaignFormData{}, err
	}
	m, ok := root.(map[string]any)
	if !ok {
		return psacampaign.CampaignFormData{}, fmt.Errorf("psaportal: edit root not an object")
	}
	fdRaw, ok := m["formData"]
	if !ok {
		return psacampaign.CampaignFormData{}, fmt.Errorf("psaportal: edit response missing formData")
	}
	fd, ok := fdRaw.(map[string]any)
	if !ok {
		return psacampaign.CampaignFormData{}, fmt.Errorf("psaportal: formData not an object")
	}
	return decodeFormData(fd)
}

// getRefPacked fetches url and resolves the SvelteKit ref-packed envelope into a plain value.
func (c *Client) getRefPacked(ctx context.Context, token, url string) (any, error) {
	headers := map[string]string{
		"Cookie":     "accessToken=" + token,
		"User-Agent": browserUA,
		"Accept":     "application/json",
	}
	resp, err := c.http.Get(ctx, url, headers, 0)
	if err != nil {
		return nil, fmt.Errorf("psaportal: campaign fetch: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("psaportal: campaign fetch status %d", resp.StatusCode)
	}
	packed, err := packedFromEnvelope(resp.Body)
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
			GradeMin:          asString(buyBox["gradeMin"]),
			GradeMax:          asString(buyBox["gradeMax"]),
			YearMin:           asInt(buyBox["yearMin"]),
			YearMax:           asInt(buyBox["yearMax"]),
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

// applyFormData fills the subject/publisher filters on pc from the edit-form data.
func applyFormData(pc *psacampaign.PortalCampaign, fd psacampaign.CampaignFormData) {
	pc.SubjectFilter = psacampaign.CampaignFilter{
		Type:     fd.SubjectFilterType,
		Subjects: fd.SelectedSubjects,
	}
	pc.PublisherFilter = psacampaign.CampaignFilter{
		Type:     fd.PublisherFilterType,
		Subjects: fd.SelectedPublishers,
	}
}
