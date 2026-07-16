package psaportal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

// numericFormDataFields are formData keys that PSA's updateCampaign endpoint
// expects as JSON numbers, not strings. Grade bounds are the notable
// exception: gradeMinimum/gradeMaximum are strings ("1"/"10") on the wire.
var numericFormDataFields = map[string]bool{
	"bidPercentage":               true,
	"dailyBudget":                 true,
	"yearMinimum":                 true,
	"yearMaximum":                 true,
	"priceMinimum":                true,
	"priceMaximum":                true,
	"flatFee":                     true,
	"dailySpecLimit":              true,
	"cardLadderConfidenceMinimum": true,
}

// PushCampaign applies changes to campaign id by read-modify-writing the full
// edit-form record: it fetches the current formData, mutates only the
// changed fields, then re-encodes and POSTs the whole record to PSA's
// updateCampaign endpoint (mirroring the internal PUT-full-record rule).
func (c *Client) PushCampaign(ctx context.Context, id string, changes []psacampaign.FieldChange) error {
	editURL := c.baseURL() + fmt.Sprintf(campaignEditPathF, id)
	root, err := c.getRefPacked(ctx, editURL)
	if err != nil {
		return err
	}
	rootMap, ok := root.(map[string]any)
	if !ok {
		return fmt.Errorf("psaportal: edit root not an object")
	}
	fdRaw, ok := rootMap["formData"]
	if !ok {
		return fmt.Errorf("psaportal: edit response missing formData")
	}
	formData, ok := fdRaw.(map[string]any)
	if !ok {
		return fmt.Errorf("psaportal: formData not an object")
	}

	for _, ch := range changes {
		if _, exists := formData[ch.Field]; !exists {
			return fmt.Errorf("psaportal: unknown campaign field %q", ch.Field)
		}
		if numericFormDataFields[ch.Field] {
			n, err := strconv.ParseFloat(ch.New, 64)
			if err != nil {
				return fmt.Errorf("psaportal: field %q value %q is not numeric: %w", ch.Field, ch.New, err)
			}
			formData[ch.Field] = n
		} else {
			formData[ch.Field] = ch.New
		}
	}

	remoteHash, err := c.fetchRemoteHash(ctx, "updateCampaign")
	if err != nil {
		return err
	}

	packed, err := EncodeRefPacked([]any{map[string]any{"id": id, "formData": formData}})
	if err != nil {
		return fmt.Errorf("psaportal: encode update payload: %w", err)
	}
	arrJSON, err := json.Marshal(packed)
	if err != nil {
		return fmt.Errorf("psaportal: marshal update payload: %w", err)
	}
	payload := base64.StdEncoding.EncodeToString(arrJSON)

	body, err := json.Marshal(map[string]any{"payload": payload, "refreshes": []any{}})
	if err != nil {
		return fmt.Errorf("psaportal: marshal update request: %w", err)
	}

	updateURL := fmt.Sprintf("%s/buyercampaignmanager/_app/remote/%s/updateCampaign", c.baseURL(), remoteHash)
	resp, err := c.fetch.Do(ctx, FetchRequest{URL: updateURL, Method: "POST", Body: string(body)})
	if err != nil {
		return fmt.Errorf("psaportal: update campaign: %w", err)
	}
	if resp.Status != 200 {
		return fmt.Errorf("psaportal: update campaign status %d", resp.Status)
	}

	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &envelope); err != nil {
		return fmt.Errorf("psaportal: decode update campaign response: %w", err)
	}
	if envelope.Type != "result" {
		return fmt.Errorf("psaportal: update campaign response type %q, want \"result\"", envelope.Type)
	}
	return nil
}
