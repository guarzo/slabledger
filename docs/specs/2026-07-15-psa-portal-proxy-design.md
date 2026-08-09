# PSA portal harvester: clean egress via proxy

## Problem

PSA portal writes and campaign-config reads fail in prod with Cloudflare
`HTTP 403: Just a moment...`. Root cause is the **egress IP** — Cloudflare's WAF
scores the Fly datacenter IP as a bot on the stricter `/buyercampaignmanager`
routes. Confirmed 2026-07-15: same URL + same UA returns 200 from the dev
workspace IP (8/8) and 403 from the Fly harvester IP (8/8). See
`docs/private/psa-portal-residential-proxy-prompt.md`.

## Fix

Route the harvester's Playwright **browser** through a proxy with a clean egress
IP. Browser-config + infra only — **no Go transport changes, no UA changes**.
Strictly additive: with the env var unset, egress is unchanged (direct).

### Proxy validated (2026-07-15)

Decodo US residential gateway (`us.decodo.com:10001`) was gated **through a real
Chromium browser** (the harvester's actual transport) against
`https://www.psacard.com/buyercampaignmanager/`:

| Transport | Result |
|---|---|
| Direct (Fly IP, prod) | 403 |
| curl through Decodo US proxy | 403 (misleading — curl TLS fingerprint is challenged regardless of IP) |
| **Chromium through Decodo US proxy** | **200 "Sign In to PSA"** (4 exits: ports 10001/10002/10005/10007) |

Key lesson: the curl gate from the diagnosis is unreliable for **proxied**
requests — Cloudflare challenges curl's fingerprint independent of IP. The real
gate is Playwright-through-proxy, which passes.

## Changes (all plumbing, 5 files)

1. **`internal/platform/config/types.go`** — add `ProxyURL string` to
   `PSAPortalConfig`.
2. **`internal/platform/config/loader.go`** — read
   `cfg.PSAPortal.ProxyURL = os.Getenv("PSA_PORTAL_PROXY_URL")`. No validation
   rule (optional, boundary-trusted secret).
3. **`internal/adapters/clients/psaportal/session.go`** — add a `proxyURL string`
   param to `OpenBrowserSession`; append `"PSA_PORTAL_PROXY_URL="+proxyURL` to
   `cmd.Env` only when non-empty.
4. **`cmd/psa-harvest/main.go`** — pass `cfg.PSAPortal.ProxyURL` to
   `OpenBrowserSession` (linear wiring only, no new orchestration — repo rule).
5. **`web/scripts/harvest-psa-token.mjs`** — extract a pure
   `proxyFromEnv(url)` helper returning a Playwright `proxy` object (or
   `undefined`); build `contextOpts` and attach `proxy` at **context level**
   (creds ride with the session). `newContext(contextOpts)`.
6. **`.env.example`** — empty `PSA_PORTAL_PROXY_URL=` placeholder + comment next
   to the other `PSA_PORTAL_*` vars.

## Testing (table-driven, per constraints)

- **Config loader test** (`config_test.go`): `PSA_PORTAL_PROXY_URL` env →
  `cfg.PSAPortal.ProxyURL`; set and unset cases.
- **JS unit test** (Vitest under `web/`): `proxyFromEnv()` mapping —
  - `http://user:pass@host:port` → `{server:'http://host:port', username, password}`
  - `socks5://host:port` (no creds) → `{server:'socks5://host:port', username:undefined, password:undefined}`
  - `''` / undefined → `undefined`

  The genuine Cloudflare-clearing behavior is left to the live E2E (below) — not
  faked green.

## Acceptance (ops, post-merge)

- `fly secrets set PSA_PORTAL_PROXY_URL='http://…@us.decodo.com:10001' -a slabledger-psa-harvest`
- Reset the pending create row to `approved`:
  `UPDATE psa_campaign_push_queue SET status='approved', error=NULL WHERE id='3fa8cdb6-0439-4f82-ad52-bf8abd79d73d';`
- Deploy + update the scheduled machine (`fly deploy` **then**
  `fly machine update <id> --image … --schedule hourly`, per
  `docs/psa-harvester.md`).
- Live run drains the create; portal shows the new PAUSED campaign with
  `psaCampaignRequestId` linked; `__data.json` read returns 200.
- With the var unset, behavior unchanged.
- `go test ./...`, `make check` green.

## Out of scope

- Proxy scoping to psacard.com-only (Lightdash proxying is harmless).
- Option (C) PSA IP-allowlist conversation (durable follow-up).
- Go transport / `Fetcher` / UA — all correct, untouched.
