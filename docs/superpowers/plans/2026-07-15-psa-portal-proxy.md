# PSA Portal Proxy Egress Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Route the PSA harvester's Playwright browser through an optional proxy (`PSA_PORTAL_PROXY_URL`) so psacard.com sees a clean egress IP instead of the Cloudflare-challenged Fly datacenter IP.

**Architecture:** Plumb one optional env var from Go config → the `psaportal` session boundary → the node harvest script, where it becomes a Playwright context-level `proxy`. No Go transport changes, no UA changes. Strictly additive: unset var = unchanged direct egress.

**Tech Stack:** Go 1.26 (config + exec), Node/Playwright (`@playwright/test`), Vitest (JS unit test).

## Global Constraints

- No Go transport / `Fetcher` changes — egress only.
- Do NOT touch the User-Agent (`web/scripts/harvest-psa-token.mjs:38`).
- Keep `cmd/psa-harvest/main.go` linear wiring — no new orchestration.
- Never commit the proxy URL/creds — Fly secret only; `.env.example` gets an empty placeholder + comment.
- Table-driven tests (Go) / cases array (JS). Do not fake Cloudflare-clearing green — assert the env→proxy mapping only.
- Proxy attached at **context level** (creds ride with the session).
- Monetary/unrelated code untouched.

---

### Task 1: Config plumbing (`ProxyURL`)

**Files:**
- Modify: `internal/platform/config/types.go:282-290` (add field)
- Modify: `internal/platform/config/loader.go:240-241` (read env)
- Test: `internal/platform/config/config_test.go` (new test func)

**Interfaces:**
- Produces: `PSAPortalConfig.ProxyURL string`, populated from `PSA_PORTAL_PROXY_URL`.

- [ ] **Step 1: Write the failing test**

Add to `internal/platform/config/config_test.go`:

```go
func TestPSAPortalProxyURL(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want string
	}{
		{name: "set from env", env: "http://u:p@host:10001", want: "http://u:p@host:10001"},
		{name: "empty when unset", env: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PSA_PORTAL_PROXY_URL", tt.env)
			cfg := FromEnv(Default())
			if cfg.PSAPortal.ProxyURL != tt.want {
				t.Errorf("PSAPortal.ProxyURL = %q, want %q", cfg.PSAPortal.ProxyURL, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/platform/config/ -run TestPSAPortalProxyURL`
Expected: FAIL — `cfg.PSAPortal.ProxyURL` undefined (compile error).

- [ ] **Step 3: Add the field**

In `internal/platform/config/types.go`, inside `PSAPortalConfig`, after `Password string`:

```go
	Password string
	// ProxyURL, when set, routes the harvester's Playwright browser through a
	// proxy so psacard.com sees a clean egress IP (Cloudflare challenges the Fly
	// datacenter IP). Optional; empty = direct egress. Format:
	// http://user:pass@host:port or socks5://host:port.
	ProxyURL string
```

- [ ] **Step 4: Read the env var**

In `internal/platform/config/loader.go`, after line 241 (`cfg.PSAPortal.Password = ...`):

```go
	cfg.PSAPortal.ProxyURL = os.Getenv("PSA_PORTAL_PROXY_URL")
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/platform/config/ -run 'TestPSAPortalProxyURL|TestPSAPortalEnabled'`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/platform/config/types.go internal/platform/config/loader.go internal/platform/config/config_test.go
git commit -m "feat(config): add optional PSA_PORTAL_PROXY_URL"
```

---

### Task 2: Pass proxy through the Go→node session boundary

**Files:**
- Modify: `internal/adapters/clients/psaportal/session.go:147-156` (signature + env)
- Modify: `cmd/psa-harvest/main.go:81` (caller)

**Interfaces:**
- Consumes: `cfg.PSAPortal.ProxyURL` (Task 1).
- Produces: `OpenBrowserSession(ctx, workDir, email, password, storedToken, proxyURL string, logger)` — injects `PSA_PORTAL_PROXY_URL` into the node subprocess env when non-empty.

- [ ] **Step 1: Add the parameter and env injection**

In `internal/adapters/clients/psaportal/session.go`, change the signature (line 147):

```go
func OpenBrowserSession(ctx context.Context, workDir, email, password, storedToken, proxyURL string, logger observability.Logger) (*browserSession, string, time.Time, error) {
```

After the `storedToken` env block (line 154-156), add:

```go
	if proxyURL != "" {
		cmd.Env = append(cmd.Env, "PSA_PORTAL_PROXY_URL="+proxyURL)
	}
```

- [ ] **Step 2: Update the caller**

In `cmd/psa-harvest/main.go` line 81, add `cfg.PSAPortal.ProxyURL` before `storedToken`... — match the new parameter order (proxyURL comes after storedToken):

```go
	session, token, expiresAt, err := psaportal.OpenBrowserSession(ctx, ".", cfg.PSAPortal.Email, cfg.PSAPortal.Password, storedToken, cfg.PSAPortal.ProxyURL, logger)
```

- [ ] **Step 3: Build to verify wiring compiles**

Run: `go build ./cmd/psa-harvest ./internal/adapters/clients/psaportal`
Expected: no output (success). Also run `grep -rn "OpenBrowserSession(" internal cmd | grep -v _test` and confirm the caller is the only one; if a test calls it, update its args to add `""` for proxyURL.

- [ ] **Step 4: Run package tests**

Run: `go test ./internal/adapters/clients/psaportal/... ./cmd/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/clients/psaportal/session.go cmd/psa-harvest/main.go
git commit -m "feat(psaportal): plumb proxy URL into harvester session env"
```

---

### Task 3: Playwright proxy in the harvest script (`proxyFromEnv` helper + wiring)

**Files:**
- Create: `web/scripts/proxy-from-env.mjs` (pure, testable helper)
- Create: `web/scripts/proxy-from-env.test.js` (Vitest)
- Modify: `web/scripts/harvest-psa-token.mjs:30` (import), `:156` (use helper)

**Interfaces:**
- Produces: `proxyFromEnv(url: string | undefined) => {server, username, password} | undefined`.
- Consumes (script side): `process.env.PSA_PORTAL_PROXY_URL` (Task 2).

- [ ] **Step 1: Write the failing test**

Create `web/scripts/proxy-from-env.test.js`:

```js
import { describe, it, expect } from 'vitest';
import { proxyFromEnv } from './proxy-from-env.mjs';

describe('proxyFromEnv', () => {
  const cases = [
    {
      name: 'http with credentials',
      input: 'http://user:pass@host:10001',
      want: { server: 'http://host:10001', username: 'user', password: 'pass' },
    },
    {
      name: 'socks5 without credentials',
      input: 'socks5://host:1080',
      want: { server: 'socks5://host:1080', username: undefined, password: undefined },
    },
    { name: 'empty string', input: '', want: undefined },
    { name: 'undefined', input: undefined, want: undefined },
  ];
  for (const c of cases) {
    it(c.name, () => {
      expect(proxyFromEnv(c.input)).toEqual(c.want);
    });
  }
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run scripts/proxy-from-env.test.js`
Expected: FAIL — cannot resolve `./proxy-from-env.mjs`.

- [ ] **Step 3: Write the helper**

Create `web/scripts/proxy-from-env.mjs`:

```js
// proxyFromEnv maps a PSA_PORTAL_PROXY_URL string to a Playwright context
// `proxy` option. Returns undefined when unset so egress stays direct.
// Format: http://user:pass@host:port or socks5://host:port.
export function proxyFromEnv(url) {
  if (!url) return undefined;
  const u = new URL(url);
  return {
    server: `${u.protocol}//${u.host}`,
    username: u.username || undefined,
    password: u.password ? decodeURIComponent(u.password) : undefined,
  };
}
```

Note: `u.password` is percent-decoded so creds containing reserved chars (e.g. `=`) survive round-trip. Test passwords contain no `%`, so `decodeURIComponent` is a no-op for them.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run scripts/proxy-from-env.test.js`
Expected: PASS (4 cases).

- [ ] **Step 5: Wire the helper into the harvest script**

In `web/scripts/harvest-psa-token.mjs`, add after line 30 (`import { chromium } ...`):

```js
import { proxyFromEnv } from './proxy-from-env.mjs';
```

Replace line 156 (`const context = await browser.newContext({ userAgent: UA });`) with:

```js
const contextOpts = { userAgent: UA };
const proxy = proxyFromEnv(process.env.PSA_PORTAL_PROXY_URL);
if (proxy) contextOpts.proxy = proxy;
const context = await browser.newContext(contextOpts);
```

- [ ] **Step 6: Lint/typecheck the web changes**

Run: `cd web && npm run lint`
Expected: PASS (no new errors in the two touched/created files).

- [ ] **Step 7: Commit**

```bash
git add web/scripts/proxy-from-env.mjs web/scripts/proxy-from-env.test.js web/scripts/harvest-psa-token.mjs
git commit -m "feat(harvest): route Playwright context through optional proxy"
```

---

### Task 4: Document the env var in `.env.example`

**Files:**
- Modify: `.env.example:240` (after `PSA_PORTAL_PASSWORD=`)

- [ ] **Step 1: Add the placeholder + comment**

In `.env.example`, after line 240 (`PSA_PORTAL_PASSWORD=`) and before the blank line at 241:

```bash
PSA_PORTAL_PASSWORD=

# Optional. Routes the harvester's Playwright browser through a proxy so
# psacard.com sees a clean egress IP (Cloudflare challenges the Fly datacenter
# IP). Leave empty for direct egress. Set as a Fly secret, never commit creds.
# Format: http://user:pass@host:port or socks5://host:port
PSA_PORTAL_PROXY_URL=
```

- [ ] **Step 2: Commit**

```bash
git add .env.example
git commit -m "docs(env): document PSA_PORTAL_PROXY_URL"
```

---

### Task 5: Full verification gate

- [ ] **Step 1: Go tests with race**

Run: `go test -race ./internal/platform/config/... ./internal/adapters/clients/psaportal/... ./cmd/...`
Expected: PASS.

- [ ] **Step 2: Full suite + quality checks**

Run: `go test ./... && make check`
Expected: PASS / green (lint + architecture import check + file size).

- [ ] **Step 3: JS tests + lint**

Run: `cd web && npx vitest run scripts/proxy-from-env.test.js && npm run lint`
Expected: PASS.

- [ ] **Step 4: golangci-lint**

Run: `golangci-lint run ./internal/platform/config/... ./internal/adapters/clients/psaportal/... ./cmd/psa-harvest/...`
Expected: no issues.

---

## Post-merge acceptance (ops — not part of the code PR)

Documented for the operator; run after merge/deploy:

1. `fly secrets set PSA_PORTAL_PROXY_URL='http://<user>:<pass>@us.decodo.com:10001' -a slabledger-psa-harvest` (Decodo US residential gateway; real creds live only in the Fly secret / password manager, validated via Chromium 2026-07-15).
2. Reset the pending create row (prod DB, `SUPABASE_DB_URL` in `/workspace/.env`):
   `UPDATE psa_campaign_push_queue SET status='approved', error=NULL WHERE id='3fa8cdb6-0439-4f82-ad52-bf8abd79d73d';`
3. `fly deploy` **then** `fly machine update <id> --image … --schedule hourly` (per `docs/psa-harvester.md` — plain deploy does not update the scheduled machine).
4. Confirm: live run drains the create; portal shows the new PAUSED campaign with `psaCampaignRequestId`; `__data.json` read returns 200; with var unset, behavior unchanged.

---

## Self-Review

- **Spec coverage:** types.go/loader.go (Task 1) ✓, session.go env inject (Task 2) ✓, main.go caller (Task 2) ✓, harvest-psa-token.mjs context proxy + `proxyFromEnv` (Task 3) ✓, `.env.example` (Task 4) ✓, Go loader test + JS mapping test (Tasks 1,3) ✓, acceptance/ops documented (post-merge) ✓.
- **Placeholder scan:** none — every code step shows full code.
- **Type consistency:** `proxyFromEnv` signature/return identical in helper, test, and script usage; `OpenBrowserSession` new param order (…storedToken, proxyURL, logger) consistent between definition and caller; `PSAPortalConfig.ProxyURL` consistent across field, loader, session caller.
