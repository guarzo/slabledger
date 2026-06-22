// Package psaportal pulls per-cert purchase rows from the PSA Buyer Campaign
// Manager portal (Collectors OAuth -> SvelteKit __data.json -> Lightdash embed).
package psaportal

const (
	defaultPSABaseURL       = "https://www.psacard.com"
	defaultLightdashBaseURL = "https://collectors.lightdash.cloud"
	analyticsPath           = "/buyercampaignmanager/analytics/__data.json?x-sveltekit-invalidated=001"
	itemizedPurchasesSlug   = "embed-itemized-purchases"

	// browserUA mimics a real browser so Cloudflare on psacard.com lets the request through.
	browserUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
)
