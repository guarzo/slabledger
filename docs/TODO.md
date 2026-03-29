# Pricing Improvement TODO

## Monitor Phase 2 Impact

After deploying the CardHedger accuracy improvements (2026-03-29):

- [ ] Watch admin page fusion coverage: "Full Fusion" vs "Partial" vs "PC Only" — cert sweep should shift cards toward Full/Partial over the next few batch cycles
- [ ] Check if the 415 CardHedger discovery failures count drops (newly cert-resolved cards won't need CardMatch)
- [ ] After 1 week: re-run CL vs fusion deviation analysis to measure improvement (baseline: 78.6% avg deviation from reviewed prices)
- [ ] Check how often `cl_fallback` triggers as `EstimateSource` — high frequency means CH accuracy still needs work; low frequency means the post-match validation is catching most bad matches upstream

## Phase 4: Reconsider PriceCharting

Deferred until Card Ladder scraping is working and Phase 2 improvements are validated.

- PriceCharting currently provides `lastSoldCents` (actual recent eBay sale prices) for 45% of cards
- Only 59 API calls/day — low cost to keep
- Once CL scraping provides fresh automated valuations and CH accuracy improves, evaluate whether PC still adds value
- Decision criteria: if CL + CH cover >90% of inventory with <20% avg deviation, PC may be redundant

## Card Ladder: Future Phases

Now that Phase 1 (value refresh) and Phase 2 (sales comps) are implemented:

- [ ] **Phase 3: Fusion Engine Integration** — Wire CL sales comp data into the fusion engine as a `SecondaryPriceSource`. CL comps would contribute to fused price calculations alongside CardHedger. Deferred because CL values already dominate reviewed prices (74% set directly to CL value) — feeding them into fusion risks over-indexing. Revisit once Phase 2 is stable and the value of blending CL comps with CH is understood.
- [ ] **Sell sheet enrichment** — Surface "last 5 sold prices" from CL comps alongside the CL value in the sell sheet for fuller negotiation context.
- [ ] **gemRateID population** — Currently mappings store gemRateID as empty string since the collection endpoint doesn't return it. Investigate whether the card detail page or another endpoint provides gemRateID to enable Phase 2 sales comp fetching. Without gemRateID, sales comps can't be queried.
- [ ] **Card Ladder official API migration** — CL support confirmed they'll offer an API in the future. When available, migrate from the internal Cloud Run search endpoints to the official API.

## TCGPlayer for Raw Pricing

Future consideration for card acquisition decisions. The double-holo-api project uses TCGPlayer as primary source for Raw NM card prices via a separate PostgreSQL database with daily snapshots.

- Would help evaluate whether to acquire ungraded cards for grading
- See `tmp/PRICE_DATA_SOURCES.md` for implementation pattern
- See memory file `project_tcgplayer_raw_pricing.md` for context
