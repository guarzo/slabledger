package psaportal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/guarzo/slabledger/internal/domain/observability"
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

	// Compare-and-swap against the live record before mutating anything. The
	// approval signature proves the operator approved *this diff*; it cannot
	// prove the portal still holds what they were shown. Between approval and
	// drain the campaign may have moved — someone edited it in the PSA UI, or a
	// database writer re-queued a stale approved row (SLA-44's threat model).
	// FieldChange.Old is what the approver saw as the current value, so if the
	// live record no longer renders to it, this push would silently clobber a
	// change nobody approved. Refuse instead, before the first mutation.
	live, err := decodeFormData(formData)
	if err != nil {
		return fmt.Errorf("psaportal: decode live formData for staleness check: %w", err)
	}
	for _, ch := range changes {
		liveValue, ok := psacampaign.RenderFieldValue(ch.Field, live)
		if !ok {
			return fmt.Errorf("%w: %q", psacampaign.ErrNoRenderer, ch.Field)
		}
		if liveValue != ch.Old {
			c.logger.Error(ctx, "psaportal: refusing stale update",
				observability.String("campaign_id", id),
				observability.String("field", ch.Field),
				observability.String("approved_old", truncateValue(ch.Old)),
				observability.String("live", truncateValue(liveValue)))
			return fmt.Errorf("%w: field %q was %q at approval, portal now holds %q",
				psacampaign.ErrFieldStale, ch.Field, ch.Old, liveValue)
		}
	}

	for _, ch := range changes {
		if _, exists := formData[ch.Field]; !exists {
			return fmt.Errorf("psaportal: unknown campaign field %q", ch.Field)
		}
		switch {
		case numericFormDataFields[ch.Field]:
			n, err := strconv.ParseFloat(ch.New, 64)
			if err != nil {
				return fmt.Errorf("psaportal: field %q value %q is not numeric: %w", ch.Field, ch.New, err)
			}
			formData[ch.Field] = n
		case ch.Value != nil:
			// List-valued changes (selectedSubjects, deniedSpecs,
			// prepackagedSpecListIds) carry a typed Go value, not the display
			// string in ch.New. Round-trip it through JSON so EncodeRefPacked
			// sees the same plain map[string]any/[]any/scalar types the rest of
			// formData already holds (it came from DecodeRefPacked), exactly as
			// CreateCampaign round-trips its formData struct (create.go:24-34).
			valueJSON, err := json.Marshal(ch.Value)
			if err != nil {
				return fmt.Errorf("psaportal: marshal field %q value: %w", ch.Field, err)
			}
			var plain any
			if err := json.Unmarshal(valueJSON, &plain); err != nil {
				return fmt.Errorf("psaportal: remarshal field %q value: %w", ch.Field, err)
			}
			formData[ch.Field] = plain
		default:
			formData[ch.Field] = ch.New
		}
	}

	remoteHash, err := c.fetchRemoteHash(ctx, "updateCampaign")
	if err != nil {
		return err
	}

	// PSA's compiled edit page calls the remote function with a bare object:
	// `updateCampaign({id, formData})` (nodes/11.*.js), exactly as the create
	// page calls `createCampaign({...formData})`. The argument must NOT be
	// wrapped in an array — SvelteKit passes the decoded payload straight
	// through as the function's single argument, so an array makes the
	// server-side destructure of `id`/`formData` yield undefined and the
	// endpoint fails with an opaque 500 "Internal Error" + errorId.
	packed, err := EncodeRefPacked(map[string]any{"id": id, "formData": formData})
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

	updateURL := fmt.Sprintf("%s/_app/remote/%s/updateCampaign", c.baseURL(), remoteHash)
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
		// PSA returns an opaque 500 ("Internal Error" + errorId) when it rejects
		// the read-modify-written record, with no field-level detail. Dump the
		// exact formData we sent (keys, Go types, values) so a campaign-specific
		// encode/round-trip corruption can be spotted without re-running blind.
		c.logger.Error(ctx, "psaportal: update campaign rejected — dumping sent formData",
			observability.String("campaign_id", id),
			observability.String("response_body", truncateBody(resp.Body)),
			observability.String("form_data", dumpFormData(formData)))
		return fmt.Errorf("psaportal: update campaign response type %q, want \"result\": %s", envelope.Type, truncateBody(resp.Body))
	}
	return nil
}

// dumpFormData renders a formData map as a stable, bounded "key(type)=value"
// string for diagnostic logging. Keys are sorted so successive dumps diff
// cleanly; each value is length-capped and the whole output is capped by a
// total budget so a large record cannot flood the log.
func dumpFormData(fd map[string]any) string {
	const totalBudget = 2000 // cap the whole diagnostic line, not just per-value
	keys := make([]string, 0, len(fd))
	for k := range fd {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if b.Len() >= totalBudget {
			fmt.Fprintf(&b, ", …(%d more fields)", len(keys)-i)
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		v := fd[k]
		fmt.Fprintf(&b, "%s(%T)=%s", truncateRunes(k, 64), v, truncateValue(v))
	}
	return b.String()
}

// truncateValue formats a single formData value, capping its length.
func truncateValue(v any) string {
	return truncateRunes(fmt.Sprintf("%v", v), 80)
}

// truncateRunes caps s to n runes (rune-safe, never splitting a multi-byte
// character) and appends an ellipsis when it trims.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
