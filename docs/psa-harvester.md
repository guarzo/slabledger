# PSA token harvester

The PSA Buyer Campaign Manager portal (which replaced the old Google-Sheet feed)
authenticates with a confidential OAuth flow that can't be refreshed headlessly from
a token alone. Instead, a small **out-of-process job logs in with a real browser** and
writes a fresh ~24h access token to Postgres. The main app reads that token from the
`psa_portal_token` table and never runs a browser itself.

```
psa-harvest job (Chromium)  ──writes encrypted token──▶  psa_portal_token (Postgres)
                                                                │
                          main app  ──reads token──────────────┘  → PSA sync / import
```

## Why a separate job

Playwright/Chromium doesn't run on the app's alpine image (musl), and the app only
needs a browser for a once-a-day login. Keeping it separate lets the app image stay
lean; the DB is the only coupling.

## Build & run

```bash
docker build -f Dockerfile.harvest -t slabledger-psa-harvest .

docker run --rm \
  -e PSA_PORTAL_EMAIL="user@example.com" \
  -e PSA_PORTAL_PASSWORD="********" \
  -e ENCRYPTION_KEY="$ENCRYPTION_KEY" \
  -e DATABASE_URL="$DATABASE_URL" \
  slabledger-psa-harvest
```

On success it logs `psa-harvest: access token refreshed` and exits 0. On failure the
underlying Playwright script's stderr (and a debug screenshot/HTML) is surfaced in the
logs; exit is non-zero.

## Scheduling

Run every ~12h so the stored token (valid ~24h) never lapses. Any scheduler works —
pick what matches the deploy:

- **cron / systemd timer**: `0 */12 * * *  docker run --rm ... slabledger-psa-harvest`
- **docker-compose**: a one-shot service invoked by the host cron, or a sidecar with a
  `sleep 43200` loop.
- **Kubernetes**: a `CronJob` using the `slabledger-psa-harvest` image, schedule
  `0 */12 * * *`, with the four env vars from a Secret.

## Env

| Var | Used by | Notes |
|---|---|---|
| `PSA_PORTAL_EMAIL` / `PSA_PORTAL_PASSWORD` | harvester only | portal login (password-only, no MFA) |
| `ENCRYPTION_KEY` | harvester + app | AES key; token encrypted at rest |
| `DATABASE_URL` | harvester + app | shared Postgres |

The **main app** needs only `ENCRYPTION_KEY` + `DATABASE_URL` (to decrypt/read the
token) and `PSA_SYNC_ENABLED=true` to run the daily import — it does **not** need the
PSA credentials.

## Version coupling

`Dockerfile.harvest`'s `mcr.microsoft.com/playwright:vX-…` tag must match
`web/package.json`'s `@playwright/test` version so the npm-installed client matches the
browsers baked into the base image. Bump them together.
