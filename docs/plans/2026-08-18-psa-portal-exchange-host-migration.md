# PSA portal migration: `www.psacard.com/buyercampaignmanager` → `exchange.psacard.com`

**Status:** implemented on branch `fix/psa-exchange-host-migration` (commit
`269e65db`, stacked on the SLA-110 CF-cookie fix). Local gates green; end-to-end
live verification on Fly still pending (see Verification). Paths below are
measured, not guessed.
**Context:** follow-up to the 2026-08-17 harvester 403 outage (fixed separately by
the flagged-CF-cookie strip, PR #683 / SLA-110). Once the 403 cleared, two
endpoints surfaced as broken because the portal has genuinely moved hosts.

## Symptom (measured 2026-08-18 on Fly, authenticated session)

- Campaigns-list GET `www.psacard.com/buyercampaignmanager/__data.json?...&page=1`
  → 307 → `exchange.psacard.com/__data.json` → **404** (the app root moved).
- Subjects/create/update POSTs → CORS **"Failed to fetch"**: a credentialed
  in-page `fetch()` cannot follow the cross-origin www→exchange redirect.
- Analytics GET and the rows snapshot keep working (navigation follows the
  redirect), so the 26h staleness gate is NOT at risk — this is degraded, not down.

## Root shape

Exchange serves the *same* SvelteKit app, rooted at `/` instead of
`/buyercampaignmanager`, with `_app` at the root. The uniform rule is:
**host `www.psacard.com` → `exchange.psacard.com`, drop the `/buyercampaignmanager`
prefix**, with the one wrinkle that the old app-root list endpoint becomes the
explicit `/campaigns` route.

Authentication is not host-bound to www: a direct `page.goto` to
`exchange.psacard.com/campaigns/__data.json` from the logged-in context returned
200 with the real campaign payload. So re-anchoring data requests to exchange
stays authenticated, and because any GET leaves the page ON exchange, the
same-origin POSTs then succeed (fixing the CORS failure).

## Measured path map

| Endpoint | Old (www) | New (exchange) | Verified |
|---|---|---|---|
| analytics data (harvester.go:83) | `/buyercampaignmanager/analytics/__data.json?x-sveltekit-invalidated=001` | `/analytics/__data.json?x-sveltekit-invalidated=001` | ✅ 200, direct |
| campaigns list (doc.go:10 `campaignsListPath`, used campaigns.go:27,92) | `/buyercampaignmanager/__data.json?x-sveltekit-trailing-slash=1&x-sveltekit-invalidated=001` (+`&page=N`) | `/campaigns/__data.json?x-sveltekit-invalidated=001` (+`&page=N`) | ✅ 200, direct (trailing-slash param inert) |
| campaign edit (doc.go:11 `campaignEditPathF`, used campaigns.go:121, push.go:37) | `/buyercampaignmanager/campaigns/%s/edit/__data.json?x-sveltekit-invalidated=0001` | `/campaigns/%s/edit/__data.json?x-sveltekit-invalidated=0001` | ⚠️ follows the rule; not directly probed (needs a live campaign id) |
| buildhash landing (buildhash.go:48) | `/buyercampaignmanager` | `/campaigns` | ✅ 200, serves `_app/immutable/entry/app.ZnWE7Kd9.js` |
| _app entry (buildhash.go:57) | `/buyercampaignmanager/_app/<entry>` | `/_app/<entry>` | ✅ `_app` refs at root in the exchange SPA |
| _app immutable (buildhash.go:71) | `/buyercampaignmanager/_app/immutable/<rel>` | `/_app/immutable/<rel>` | ✅ same |
| createCampaign POST (create.go:52) | `/buyercampaignmanager/_app/remote/%s/createCampaign` | `/_app/remote/%s/createCampaign` | ⚠️ follows the rule; not directly probed |
| updateCampaign POST (push.go:142) | `/buyercampaignmanager/_app/remote/%s/updateCampaign` | `/_app/remote/%s/updateCampaign` | ⚠️ follows the rule; not directly probed |
| getSubjects POST (subjects.go:48) | `/buyercampaignmanager/_app/remote/%s/getSubjects` | `/_app/remote/%s/getSubjects` | ⚠️ follows the rule; not directly probed |

Out of scope (different host, unaffected): the Lightdash client
(`collectors.lightdash.cloud`, lightdash.go). The analytics `embedUrl` still
points at lightdash unchanged.

## Config wiring today (from the code map)

- `baseURL()` returns `c.psaBaseURL`, set in `New` from `Config.PSABaseURL`,
  falling back to the const `defaultPSABaseURL = "https://www.psacard.com"`
  (client.go:97-113, doc.go:7).
- The sole prod construction passes an **empty** `Config{}`
  (`cmd/psa-harvest/main.go:140`), so the default const is always used. There is
  **no** env var / config field / flag for the host; `PSAPortalConfig`
  (`internal/platform/config/types.go:272-296`) has no base-URL field.
- The `/buyercampaignmanager` prefix is baked into all nine URLs: the two
  `doc.go` consts plus six inline `"/buyercampaignmanager/..."` literals in
  buildhash.go, create.go, push.go, subjects.go.
- Harvest script `START_URL` = `PSA_PORTAL_START_URL || 'https://www.psacard.com/buyercampaignmanager/'`
  (harvest-psa-token.mjs:36). `OpenBrowserSession` does not pass
  `PSA_PORTAL_START_URL`, so the www default runs in prod. Login can stay on www
  (it authenticates exchange fine), so this file need not change for correctness.

## Proposed change (smallest complete)

1. `defaultPSABaseURL` → `https://exchange.psacard.com` (doc.go:7).
2. Drop `/buyercampaignmanager` from all nine paths; set
   `campaignsListPath = "/campaigns/__data.json?x-sveltekit-invalidated=001"` and
   `campaignEditPathF = "/campaigns/%s/edit/__data.json?x-sveltekit-invalidated=0001"`
   (doc.go), and edit the inline literals in buildhash.go, create.go, push.go,
   subjects.go.
3. **Analytics request must become absolute, not just prefix-stripped**
   (harvester.go:82-85). It is sent as a *relative* URL, which the browser
   script resolves against `START_URL` (harvest-psa-token.mjs:36-37, 240-242) —
   still the www host. Prefix-stripping it to `/analytics/__data.json` would
   resolve to `www.psacard.com/analytics/__data.json`, which has no
   `/buyercampaignmanager/*` redirect rule and will not reach exchange. Change it
   to the absolute `https://exchange.psacard.com/analytics/__data.json?x-sveltekit-invalidated=001`
   (or thread the exchange origin through so the relative form resolves there).
   `Harvester.Run` currently hard-codes the relative literal; the absolute form
   is the least-coupled fix.
4. **Login / `START_URL` and the injected-cookie host need an explicit
   decision** — do NOT assume "leave on www, it just works." The measured
   exchange auth was observed from a *fresh, fully logged-in* SSO context only.
   Production also has a stored-token fast path (main.go:107 passes `storedToken`
   into `OpenBrowserSession`), and the script injects that token as a cookie
   scoped to `START_URL`'s hostname (harvest-psa-token.mjs:50-55, 165-173) —
   i.e. `www.psacard.com`, which is NOT sent to `exchange.psacard.com`. The clean
   fix-build verification run (03:35 UTC) used a valid stored token and still
   fetched exchange analytics, which suggests exchange auth rides a
   parent-scoped or CF/session cookie rather than the www-scoped `accessToken` —
   but that is inference, and it was never verified for the campaigns or subjects
   endpoints on the stored-token path. Decide and document the cookie strategy
   (keep login on www vs. move `START_URL`/`COOKIE_DOMAIN` to exchange), and
   verify BOTH the stored-token and the fresh-SSO paths against exchange
   (see Verification).

5. **Update the Go test fixtures that hard-code the old paths** — omitting this
   leaves the suite red. Fixtures pinning `/buyercampaignmanager/*` (and one
   absolute www URL) live in: `harvester_test.go:48`, `create_test.go:82,92,104,157,160`,
   `push_test.go` (updateCampaign routes throughout), `subjects_test.go:12,30,43`,
   `drain_test.go:51-52,289-294,443-451,508`, and `live_test.go:31`. Migrate each
   route key and asserted URL to the new host/path scheme in lockstep with the
   production edits.

6. **Migrate the user-facing PSA link** (lower urgency — separable). The
   Campaigns page button opens `https://www.psacard.com/buyercampaignmanager/`
   (CampaignsPage.tsx:377) with a test pinning it
   (CampaignsPage.psaLink.test.tsx:55). This is a top-level browser navigation,
   so it still redirects to exchange for a human (no CORS) — stale, not broken.
   Point it at `https://exchange.psacard.com/campaigns` and update the test.

**Optional hardening (separate, not required):** thread a `PSABaseURL` from a new
`PSA_PORTAL_BASE_URL` env var into `Config{}` at `cmd/psa-harvest/main.go:140`
so the next host move is a secret change, not a code change. Also document
`PSA_PORTAL_PROXY_URL` in `.env.example` and `docs/psa-harvester.md` — it exists
only as a Fly secret today.

## Verification

**Local gate first (must pass before any deploy):**
- `go test ./internal/adapters/clients/psaportal/...` — green after the step-5
  fixture updates. This is what catches a path that was changed in production
  code but missed in a fixture (or vice versa).
- `make check` — lint + import + size + doc-path checks.
- `cd web && npm test` — covers the step-6 `CampaignsPage.psaLink` test.

**End-to-end on Fly (must be end-to-end, not exit-0):** deploy to
`slabledger-psa-harvest`, run once, and confirm in the logs / DB:
- `saved PSA portal rows snapshot` — no regression (this is the 26h-gated path).
- Analytics read returns 200 from **exchange** (confirm the request went to the
  absolute exchange URL from step 3, not www).
- campaigns fetch returns 200 and the `psa-harvest: skipping attribution
  reconcile` WARN disappears (attribution reconcile runs).
- subjects POST returns 200 (no "Failed to fetch"); `fetch subjects failed` gone.
- create/update POSTs exercised via a real push-queue item if one is available.

**Both auth paths (from step 4):** run once with a **valid stored token** present
(`psa_portal_token` current) and once forcing a **fresh SSO** (clear/expire the
stored token). Confirm campaigns + subjects succeed on exchange in BOTH — the
stored-token path is the one the measurements never isolated, so a green here is
the finding-2 sign-off.

## Residual risk

The three POST `_app/remote/*` paths and the campaign-edit path follow the
uniform prefix-strip rule but were not directly probed (they need a live
campaign id / remote hash). The first real harvest run after the change is the
verification; if a remote path differs, the buildhash-scraped hash and the
`_app/remote/<hash>/<name>` shape are the two things to re-check on exchange.
