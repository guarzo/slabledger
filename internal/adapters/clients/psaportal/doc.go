// Package psaportal pulls per-cert purchase rows from the PSA Buyer Campaign
// Manager portal (Collectors OAuth -> SvelteKit __data.json -> Lightdash embed).
package psaportal

const (
	defaultLightdashBaseURL = "https://collectors.lightdash.cloud"
	itemizedPurchasesSlug   = "embed-itemized-purchases"
)
