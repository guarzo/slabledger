# PSA token harvester

The PSA Buyer Campaign Manager portal (which replaced the old Google-Sheet feed)
authenticates with a confidential OAuth flow that can't be refreshed headlessly from
a token alone, and its Lightdash-embedded analytics data is Cloudflare-gated — requests
from datacenter IPs get challenged, but a real browser passes. So a small **out-of-process
job drives a real browser** end to end: it (1) logs in only when the stored token is
stale (otherwise it injects the stored cookie and skips login), (2) captures the portal's
`analytics/__data.json` in-browser to get past the Cloudflare check, (3) immediately
exchanges the short-lived (~1h) embed JWT found there for the actual Lightdash rows, and
(4) writes both a fresh `psa_portal_token` (for the next run's cookie injection) and a
`psa_portal_snapshot` (the rows). The main app never runs a browser and never talks to
Cloudflare — it only reads the already-fetched rows from `psa_portal_snapshot`.

```
psa-harvest job (Chromium) ──token──▶ psa_portal_token ─┐   (login skipped while token valid)
                    └──rows snapshot──▶ psa_portal_snapshot ──▶ main app PSA sync / import
```

## Rollout order

Because the main app now reads `psa_portal_snapshot` (added by migration `000017`)
instead of talking to PSA itself, **deploy the main app first** so the migration runs
and the table exists, then rebuild the harvest image and point the scheduled machine at
it with `fly machine update <machine_id> --image ... --schedule hourly` (see
"Updating the harvester after a code change" below). Deploying the harvester before the
migration would leave it writing snapshot rows the app can't yet read.

## Why a separate job

Playwright/Chromium doesn't run on the app's alpine image (musl), and the app only
needs a browser for a once-a-day login. Keeping it separate lets the app image stay
lean; the DB is the only coupling.

## Build & run

```bash
docker build -f Dockerfile.harvest -t slabledger-psa-harvest .

docker run --rm \
  -e PSA_PORTAL_EMAIL="yeti@yeti.cards" \
  -e PSA_PORTAL_PASSWORD="********" \
  -e ENCRYPTION_KEY="$ENCRYPTION_KEY" \
  -e DATABASE_URL="$DATABASE_URL" \
  slabledger-psa-harvest
```

On success it logs `psa-harvest: token is fresh` and exits 0 (it logs
`harvesting PSA portal access token` first only when the stored token is near expiry and
an actual login is needed). On failure the
underlying Playwright script's stderr (and a debug screenshot/HTML) is surfaced in the
logs; exit is non-zero.

## Scheduling

The stored token is valid ~24h, so it must be refreshed well inside that window.
`cmd/psa-harvest` calls `EnsureFreshToken`: it only performs the browser login when the
stored token drops below 6h of validity, and is a cheap no-op otherwise. So the scheduler
can fire often (for retry margin) without paying for a real login every time.

### Production: Fly.io (current deploy)

Production runs on Fly. The harvester is a **separate Fly app** (`slabledger-psa-harvest`)
because it needs the Playwright/Chromium image, which is different from the lean app
image. It is run-to-completion, not a server.

One-time setup:

```bash
# 1) Create the app (no HTTP service, no machines yet).
fly apps create slabledger-psa-harvest

# 2) Secrets — all four required. ENCRYPTION_KEY and DATABASE_URL MUST be byte-identical
#    to the main `slabledger` app (the app decrypts what the harvester encrypts).
fly secrets set -a slabledger-psa-harvest \
  PSA_PORTAL_EMAIL='...' \
  PSA_PORTAL_PASSWORD='...' \
  ENCRYPTION_KEY='<same as slabledger>' \
  DATABASE_URL='<same Postgres URL as slabledger>'

# 3) Build & push the image (does NOT start a run on its own).
#    --image-label pins a stable tag (:harvest); without it fly deploy pushes a
#    deployment-<timestamp> tag and there is no :latest to reference below.
fly deploy -c fly.harvest.toml --build-only --push --image-label harvest -a slabledger-psa-harvest
```

Create the scheduled machine (fires the harvester every hour):

```bash
fly machine run \
  registry.fly.io/slabledger-psa-harvest:harvest \
  --schedule hourly \
  --region iad \
  --vm-memory 1024 \
  --vm-cpu-kind shared \
  --vm-cpus 1 \
  -a slabledger-psa-harvest
```

> **Pass the sizing flags explicitly.** `fly machine run` does not reliably inherit the
> `[vm]` / `primary_region` blocks from `fly.harvest.toml` — that file is consumed by
> `fly deploy --build-only` to *build* the image, not by `fly machine run` to *size* the
> machine. Omit the flags and the scheduled machine gets Fly's defaults, which may be too
> small for Chromium. The `1024`/`shared`/`1` values here mirror `fly.harvest.toml`.

> Fly's `--schedule` only accepts `hourly | daily | weekly | monthly` — there is no
> "every 12h". `hourly` is used for a wide safety margin against a failed login. Because
> `cmd/psa-harvest` uses `EnsureFreshToken` (login only when <6h validity remains), the
> hourly runs are almost all cheap no-ops — roughly one real login per day, plus a few
> hourly retries in the 6h window before expiry if a login fails.

> **Verify a one-off run before scheduling.** Run it once *without* `--schedule` and
> confirm the logs show `psa-harvest: token is fresh` (exit 0) first. A machine created by
> `fly machine run` **auto-restarts on failure**, so a crash-looping harvester retries
> forever — if a test run crash-loops, `fly machine destroy <id> --force` it before fixing
> and retrying.

Inspect / re-run manually:

```bash
fly machine list -a slabledger-psa-harvest          # see the scheduled machine (note its ID) + last exit
fly logs -a slabledger-psa-harvest                  # success: "psa-harvest: token is fresh"
fly machine run registry.fly.io/slabledger-psa-harvest:harvest --region iad --vm-memory 1024 --vm-cpu-kind shared --vm-cpus 1 -a slabledger-psa-harvest  # one-off run now
```

### Updating the harvester after a code change

The scheduled machine is **unmanaged** — `fly deploy --build-only --push` rebuilds and
pushes a new image, but it does **not** touch an already-running machine, so the hourly
schedule would keep executing the old image indefinitely. After any harvester code change,
rebuild the image and then point the existing machine at it:

```bash
# 1) Rebuild + push the new image under the same stable tag.
fly deploy -c fly.harvest.toml --build-only --push --image-label harvest -a slabledger-psa-harvest

# 2) Update the existing scheduled machine to the new image (keep the schedule).
#    Get <machine_id> from `fly machine list -a slabledger-psa-harvest`.
fly machine update <machine_id> \
  --image registry.fly.io/slabledger-psa-harvest:harvest \
  --schedule hourly \
  -a slabledger-psa-harvest
```

Re-pass `--schedule hourly` on update — it is set, not preserved implicitly. (Alternatively,
`fly machine destroy <machine_id>` then recreate with the `fly machine run` command above.)

### Other platforms

Any scheduler that can run the image every ~12h works — e.g. a `cron`/systemd timer or a
Kubernetes `CronJob` (`0 */12 * * *`) using the `slabledger-psa-harvest` image with the
four env vars from a Secret.

## Env

| Var | Used by | Notes |
|---|---|---|
| `PSA_PORTAL_EMAIL` / `PSA_PORTAL_PASSWORD` | harvester (login) + app (enable gate) | portal login (password-only, no MFA); the app never logs in but won't wire the sync without them |
| `ENCRYPTION_KEY` | harvester + app | AES key; token encrypted at rest |
| `DATABASE_URL` | harvester + app | shared Postgres |

The **main app** needs `ENCRYPTION_KEY` + `DATABASE_URL` (to decrypt/read the token),
`PSA_SYNC_ENABLED=true` to run the daily import, **and** `PSA_PORTAL_EMAIL` +
`PSA_PORTAL_PASSWORD`. The app never logs in, but it only *wires* the portal sync when
those credentials are present (`PSAPortal.Enabled = email != "" && password != ""` in
`config/loader.go`; the client is constructed in `cmd/slabledger/main.go`). Config also
rejects setting just one of the pair — set both or neither. So in practice all four vars
go on both apps, with `ENCRYPTION_KEY`/`DATABASE_URL` identical across them.

## Version coupling

`Dockerfile.harvest`'s `mcr.microsoft.com/playwright:vX-…` tag must match
`web/package.json`'s `@playwright/test` version so the npm-installed client matches the
browsers baked into the base image. Bump them together.
