package psaportal

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
)

const embedTokenHeader = "Lightdash-Embed-Token"

// embedHeaders builds the JSON + embed-token header map for Lightdash embed calls.
func embedHeaders(embedJWT string) map[string]string {
	return map[string]string{
		"Content-Type":   "application/json",
		embedTokenHeader: embedJWT,
	}
}

type lightdashClient struct {
	client  *httpx.Client
	baseURL string
}

func newLightdashClient(baseURL string) *lightdashClient {
	cfg := httpx.DefaultConfig("Lightdash")
	cfg.DefaultTimeout = 30 * time.Second
	return &lightdashClient{
		client:  httpx.NewClient(cfg),
		baseURL: baseURL,
	}
}

// tileRows POSTs chart-and-results and returns each row flattened to fieldId -> raw-as-string.
func (lc *lightdashClient) tileRows(ctx context.Context, projectUUID, embedJWT, tileUUID string) ([]map[string]string, error) {
	url := fmt.Sprintf("%s/api/v1/embed/%s/chart-and-results", lc.baseURL, projectUUID)

	reqBody, err := json.Marshal(map[string]any{
		"tileUuid": tileUUID,
		"dashboardFilters": map[string]any{
			"dimensions":        []any{},
			"metrics":           []any{},
			"tableCalculations": []any{},
		},
		"dashboardSorts": []any{},
	})
	if err != nil {
		return nil, fmt.Errorf("lightdash: marshal request: %w", err)
	}

	headers := embedHeaders(embedJWT)

	resp, err := lc.client.Post(ctx, url, headers, reqBody, 0)
	if err != nil {
		return nil, fmt.Errorf("lightdash: POST chart-and-results: %w", err)
	}

	var envelope struct {
		Status  string          `json:"status"`
		Error   json.RawMessage `json:"error"`
		Results *struct {
			Rows []map[string]ldCell `json:"rows"`
		} `json:"results"`
	}
	if err := json.Unmarshal(resp.Body, &envelope); err != nil {
		return nil, fmt.Errorf("lightdash: decode response: %w", err)
	}
	// A rejected embed JWT or query error can come back with a 2xx status and an
	// error envelope (httpx only errors on >=400). Treat a present error or a
	// missing results object as a failure so a broken exchange doesn't decode to
	// an empty-but-successful row set.
	if len(envelope.Error) > 0 && string(envelope.Error) != "null" {
		return nil, fmt.Errorf("lightdash: chart-and-results error: %s", envelope.Error)
	}
	if envelope.Results == nil {
		return nil, fmt.Errorf("lightdash: chart-and-results returned no results object")
	}

	out := make([]map[string]string, len(envelope.Results.Rows))
	for i, row := range envelope.Results.Rows {
		flat := make(map[string]string, len(row))
		for fieldID, cell := range row {
			flat[fieldID] = stringifyRaw(cell.Value.Raw)
		}
		out[i] = flat
	}
	return out, nil
}

// findTileUUIDBySlug returns the dashboard tile uuid whose chartSlug matches slug.
func (lc *lightdashClient) findTileUUIDBySlug(ctx context.Context, projectUUID, embedJWT, slug string) (string, error) {
	url := fmt.Sprintf("%s/api/v1/embed/%s/dashboard", lc.baseURL, projectUUID)
	headers := embedHeaders(embedJWT)
	resp, err := lc.client.Post(ctx, url, headers, []byte("{}"), 0)
	if err != nil {
		return "", fmt.Errorf("lightdash: POST dashboard: %w", err)
	}

	var envelope struct {
		Results struct {
			Tiles []struct {
				UUID       string `json:"uuid"`
				Properties struct {
					ChartSlug string `json:"chartSlug"`
				} `json:"properties"`
			} `json:"tiles"`
		} `json:"results"`
	}
	if err := json.Unmarshal(resp.Body, &envelope); err != nil {
		return "", fmt.Errorf("lightdash: decode dashboard response: %w", err)
	}
	for _, tile := range envelope.Results.Tiles {
		if tile.Properties.ChartSlug == slug {
			return tile.UUID, nil
		}
	}
	return "", fmt.Errorf("lightdash: tile with chartSlug %q not found in dashboard", slug)
}

// ldCell is the per-field cell shape returned by the Lightdash chart-and-results endpoint.
type ldCell struct {
	Value struct {
		Raw json.RawMessage `json:"raw"`
	} `json:"value"`
}

// stringifyRaw turns a JSON raw value into a plain string: null -> "", quoted
// string -> unquoted, everything else -> the raw JSON text (numbers/bools as-is).
func stringifyRaw(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	if len(s) > 1 && s[0] == '"' {
		var out string
		if err := json.Unmarshal(raw, &out); err == nil {
			return out
		}
	}
	return s
}
