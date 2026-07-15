package psaportal

import (
	"context"
	"fmt"
	"regexp"
)

// buildHashRe matches the SvelteKit app build hash in the portal page's script
// imports (e.g. .../immutable/entry/app.<hash>.js). The hash changes on every
// PSA frontend deploy, so it must be scraped fresh, never hardcoded.
var buildHashRe = regexp.MustCompile(`immutable/entry/app\.([A-Za-z0-9_-]{6,})\.js`)

// fetchBuildHash scrapes the portal landing page for the current SvelteKit
// build hash via the browser session, needed to construct the
// /_app/remote/{hash}/updateCampaign URL.
func (c *Client) fetchBuildHash(ctx context.Context) (string, error) {
	resp, err := c.fetch.Do(ctx, FetchRequest{URL: c.baseURL() + "/buyercampaignmanager", Method: "GET"})
	if err != nil {
		return "", fmt.Errorf("psaportal: build-hash page: %w", err)
	}
	if resp.Status != 200 {
		return "", fmt.Errorf("psaportal: build-hash page status %d", resp.Status)
	}
	m := buildHashRe.FindStringSubmatch(resp.Body)
	if m == nil {
		return "", fmt.Errorf("psaportal: build hash not found on portal page")
	}
	return m[1], nil
}
