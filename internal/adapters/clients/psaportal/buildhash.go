package psaportal

import (
	"context"
	"fmt"
	"regexp"
)

// appEntryRe matches the SvelteKit app entry chunk in the portal landing page
// (e.g. .../immutable/entry/app.<hash>.js). This is the client bootstrap, not a
// route hash; it changes on every PSA frontend deploy so it is scraped fresh.
var appEntryRe = regexp.MustCompile(`immutable/entry/app\.[A-Za-z0-9_-]+\.js`)

// chunkPathRe matches the immutable chunk/node paths listed in app.js's
// __vite__mapDeps array (e.g. "../chunks/DtZbo4JC.js", "../nodes/6.B_6a_64F.js").
var chunkPathRe = regexp.MustCompile(`(?:chunks|nodes)/[A-Za-z0-9_.-]+\.js`)

// remoteHashReFor builds a regex that finds a SvelteKit remote-function id for
// fn in a compiled client chunk. SvelteKit addresses remote functions at
// /_app/remote/{hash}/{fn} where {hash} is a djb2 (base36) hash of the .remote
// source file's path — NOT the app build hash — and the client bundles the id
// as the string literal "{hash}/{fn}" (e.g. "1fvuqqe/createCampaign").
func remoteHashReFor(fn string) *regexp.Regexp {
	return regexp.MustCompile(`"([a-z0-9]{5,10})/` + regexp.QuoteMeta(fn) + `"`)
}

// fetchRemoteHash resolves the SvelteKit remote-function hash segment for fn
// (e.g. "createCampaign") by crawling the portal client bundle: it loads the
// landing page, follows the app entry chunk, then scans each referenced
// immutable chunk for the "{hash}/{fn}" literal. The hash only changes when PSA
// renames the backing .remote source file, so it is cached on the Client for
// the lifetime of the run (one harvester drain), shared across all queued
// pushes rather than re-crawled per campaign.
func (c *Client) fetchRemoteHash(ctx context.Context, fn string) (string, error) {
	if c.remoteHashCache == nil {
		c.remoteHashCache = map[string]string{}
	}
	if h, ok := c.remoteHashCache[fn]; ok {
		return h, nil
	}

	page, err := c.getText(ctx, c.baseURL()+"/buyercampaignmanager")
	if err != nil {
		return "", fmt.Errorf("psaportal: remote-hash landing page: %w", err)
	}
	entry := appEntryRe.FindString(page)
	if entry == "" {
		return "", fmt.Errorf("psaportal: app entry chunk not found on portal page")
	}

	appJS, err := c.getText(ctx, c.baseURL()+"/buyercampaignmanager/_app/"+entry)
	if err != nil {
		return "", fmt.Errorf("psaportal: remote-hash app entry: %w", err)
	}

	re := remoteHashReFor(fn)
	// Chunk that carries the remote ids appears in the app entry's dep list.
	// Deduplicate paths and scan each until the "{hash}/{fn}" literal is found.
	seen := map[string]bool{}
	for _, rel := range chunkPathRe.FindAllString(appJS, -1) {
		if seen[rel] {
			continue
		}
		seen[rel] = true
		chunk, err := c.getText(ctx, c.baseURL()+"/buyercampaignmanager/_app/immutable/"+rel)
		if err != nil {
			return "", fmt.Errorf("psaportal: remote-hash chunk %s: %w", rel, err)
		}
		if m := re.FindStringSubmatch(chunk); m != nil {
			c.remoteHashCache[fn] = m[1]
			return m[1], nil
		}
	}
	return "", fmt.Errorf("psaportal: remote-function hash for %q not found in client bundle", fn)
}

// getText GETs url via the browser session and returns the body, erroring on any
// non-200 status.
func (c *Client) getText(ctx context.Context, url string) (string, error) {
	resp, err := c.fetch.Do(ctx, FetchRequest{URL: url, Method: "GET"})
	if err != nil {
		return "", err
	}
	if resp.Status != 200 {
		return "", fmt.Errorf("status %d", resp.Status)
	}
	return resp.Body, nil
}
