package psaportal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

// FetchSubjects calls the portal's getSubjects remote function for categoryID
// (16 = POKEMON) and returns every subject the category offers — 492 for
// Pokemon, confirmed live 2026-08-06. It runs only in the harvester: the
// result is persisted via psacampaign.CatalogStore.SaveSubjects so the main
// HTTP server, which has no portal session, can resolve subject names without
// ever calling the portal itself.
//
// The returned ids are all current-generation (22xxx). Live campaigns hold a
// mix of 4xxx/8xxx/22xxx subject ids from older PSA subject generations, so
// this catalog is for resolving NEW subjects an operator adds by name — it
// must never be used to rewrite an id already pulled from a live campaign.
func (c *Client) FetchSubjects(ctx context.Context, categoryID int) ([]psacampaign.SubjectRef, error) {
	remoteHash, err := c.fetchRemoteHash(ctx, "getSubjects")
	if err != nil {
		return nil, err
	}

	// getSubjects takes one positional argument, the category id, packed as a
	// bare one-element array — `getSubjects([16])` — not the object shape
	// createCampaign/updateCampaign use.
	packed, err := EncodeRefPacked([]any{float64(categoryID)})
	if err != nil {
		return nil, fmt.Errorf("psaportal: encode getSubjects payload: %w", err)
	}
	arrJSON, err := json.Marshal(packed)
	if err != nil {
		return nil, fmt.Errorf("psaportal: marshal getSubjects payload: %w", err)
	}
	body, err := json.Marshal(map[string]any{
		"payload":   base64.StdEncoding.EncodeToString(arrJSON),
		"refreshes": []any{},
	})
	if err != nil {
		return nil, fmt.Errorf("psaportal: marshal getSubjects request: %w", err)
	}

	url := fmt.Sprintf("%s/buyercampaignmanager/_app/remote/%s/getSubjects", c.baseURL(), remoteHash)
	resp, err := c.fetch.Do(ctx, FetchRequest{URL: url, Method: "POST", Body: string(body)})
	if err != nil {
		return nil, fmt.Errorf("psaportal: get subjects: %w", err)
	}
	if resp.Status != 200 {
		return nil, fmt.Errorf("psaportal: get subjects status %d", resp.Status)
	}

	var envelope struct {
		Type   string `json:"type"`
		Result string `json:"result"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &envelope); err != nil {
		return nil, fmt.Errorf("psaportal: decode getSubjects response: %w", err)
	}
	if envelope.Type != "result" {
		return nil, fmt.Errorf("psaportal: getSubjects response type %q, want \"result\": %s", envelope.Type, truncateBody(resp.Body))
	}

	var resultPacked []json.RawMessage
	if err := json.Unmarshal([]byte(envelope.Result), &resultPacked); err != nil {
		return nil, fmt.Errorf("psaportal: getSubjects result undecodable: %w", err)
	}
	root, err := DecodeRefPacked(resultPacked)
	if err != nil {
		return nil, fmt.Errorf("psaportal: getSubjects result undecodable: %w", err)
	}
	return asSubjectRefs(root), nil
}
