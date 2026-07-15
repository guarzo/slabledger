// Package psaportal pulls per-cert purchase rows and campaign offer-program
// config from the PSA Buyer Campaign Manager portal (Collectors OAuth ->
// SvelteKit __data.json -> Lightdash embed).
package psaportal

const (
	defaultPSABaseURL       = "https://www.psacard.com"
	defaultLightdashBaseURL = "https://collectors.lightdash.cloud"
	itemizedPurchasesSlug   = "embed-itemized-purchases"
	campaignsListPath       = "/buyercampaignmanager/__data.json?x-sveltekit-trailing-slash=1&x-sveltekit-invalidated=001"
	campaignEditPathF       = "/buyercampaignmanager/campaigns/%s/edit/__data.json?x-sveltekit-invalidated=0001"
)
