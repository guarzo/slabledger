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
needs a browser to harvest the portal rows. Keeping it separate lets the app image stay
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

On success it logs `psa-harvest: token and rows snapshot refreshed` and exits 0.
On failure the underlying Playwright script's stderr (and a debug screenshot/HTML)
is surfaced in the logs; exit is non-zero.

## Scheduling

Every run does the full cycle: it launches Chromium, captures the analytics
`__data.json`, and exchanges the embed JWT found there for the rows. The embed
JWT is minted fresh per request with a ~1h TTL, so it must be exchanged on every
run — there is no "cheap no-op" run. What *is* skipped when the stored token
still has validity is the interactive SSO **login**: the script injects the
stored token as a cookie and, if the session is still accepted, never touches the
password form. So the scheduler should fire hourly for retry margin against a
failed login, well inside the snapshot's 26h staleness ceiling.

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
> "every 12h". `hourly` is used for a wide safety margin against a failed login. Every
> hourly run launches Chromium and re-exchanges the ~1h embed JWT for a fresh rows
> snapshot; the stored-token cookie injection only skips the interactive SSO login, not
> the run itself.

> **Verify a one-off run before scheduling.** Run it once *without* `--schedule` and
> confirm the logs show `psa-harvest: token and rows snapshot refreshed` (exit 0) first.
> A machine created by `fly machine run` **auto-restarts on failure**, so a crash-looping
> harvester retries forever — if a test run crash-loops, `fly machine destroy <id> --force`
> it before fixing and retrying.

Inspect / re-run manually:

```bash
fly machine list -a slabledger-psa-harvest          # see the scheduled machine (note its ID) + last exit
fly logs -a slabledger-psa-harvest                  # success: "psa-harvest: token and rows snapshot refreshed"
fly machine run registry.fly.io/slabledger-psa-harvest:harvest --region iad --vm-memory 1024 --vm-cpu-kind shared --vm-cpus 1 -a slabledger-psa-harvest  # one-off run now```

### Schedule

The cadence lives on the **machine**, not in `fly.harvest.toml` or the deploy — it is set
with `fly machine update --schedule` and is what makes Fly fire the machine on its own:

```bash
fly machine update <machine_id> --schedule hourly -a slabledger-psa-harvest
# verify:
fly machine status <machine_id> -a slabledger-psa-harvest --display-config | grep -i schedule  # -> "schedule": "hourly"
```

Accepted values are `hourly | daily | weekly | monthly` (there is no "every 12h"); `hourly`
gives a wide safety margin — a missed run still leaves the reader inside its 26h staleness
ceiling. Between runs the machine sits `stopped`; Fly starts it on schedule, it harvests,
and it exits 0.

**Keep exactly one scheduled machine.** Two machines with the same schedule both fire every
hour and double-harvest (harmless — the snapshot is an idempotent singleton upsert — but
wasteful). List and prune extras:

```bash
fly machine list -a slabledger-psa-harvest                 # expect ONE machine
fly machine destroy <extra_machine_id> --force -a slabledger-psa-harvest
```

### Updating the harvester after a code change

`fly deploy` (used to ship the main app from the same repo) **also rebuilds and rolls
the harvester machine to the new image automatically** — the machine is managed as part
of the app, so a merge + deploy is enough to get new harvester code running; you do not
need to hand-roll the image onto the machine. Confirm the roll landed and the schedule
survived it:

```bash
# The machine's LAST UPDATED should be the deploy time, on the new image.
fly machine list -a slabledger-psa-harvest

# Confirm the schedule is still set (see "Schedule" below — it must be re-asserted
# if a machine was recreated rather than updated in place).
fly machine status <machine_id> -a slabledger-psa-harvest --display-config | grep -i schedule
```

If you ever need to force a specific image onto the machine manually (e.g. rolling back):

```bash
fly machine update <machine_id> \
  --image registry.fly.io/slabledger-psa-harvest:<tag> \
  --schedule hourly \
  -a slabledger-psa-harvest
```

Re-pass `--schedule hourly` on any `fly machine update` — it is set, not preserved implicitly. (Alternatively,
`fly machine destroy <machine_id>` then recreate with the `fly machine run` command above.)

### Other platforms

Any scheduler that can run the image hourly works — e.g. a `cron`/systemd timer or a
Kubernetes `CronJob` (`0 * * * *`) using the `slabledger-psa-harvest` image with the
four env vars from a Secret.

## Env

| Var | Used by | Notes |
|---|---|---|
| `PSA_PORTAL_EMAIL` / `PSA_PORTAL_PASSWORD` | harvester (login) + app (enable gate) | portal login (password-only, no MFA); the app never logs in but won't wire the sync without them |
| `ENCRYPTION_KEY` | harvester + app | AES key; token encrypted at rest |
| `DATABASE_URL` | harvester + app | shared Postgres |
| `PSA_CAMPAIGN_SYNC_ENABLED` | harvester only | gates the campaign snapshot fetch + push-queue drain described above; the app reads/writes the tables regardless but never contacts PSA itself |

The **main app** needs `ENCRYPTION_KEY` + `DATABASE_URL` (to decrypt/read the token),
`PSA_SYNC_ENABLED=true` to run the daily import, **and** `PSA_PORTAL_EMAIL` +
`PSA_PORTAL_PASSWORD`. The app never logs in, but it only *wires* the portal sync when
those credentials are present (`PSAPortal.Enabled = email != "" && password != ""` in
`config/loader.go`; the client is constructed in `cmd/slabledger/main.go`). Config also
rejects setting just one of the pair — set both or neither. So in practice all four vars
go on both apps, with `ENCRYPTION_KEY`/`DATABASE_URL` identical across them.

## Campaign sync

Separate from the per-cert token flow above, the harvester also syncs PSA **campaign
configuration** (buy boxes, budgets, subject/publisher filters) — gated by
`PSA_CAMPAIGN_SYNC_ENABLED`. When that flag is set, each `cmd/psa-harvest` run does two
things after refreshing the token:

1. **Read: snapshot the portal's campaign list.** `portal.FetchCampaigns` (in
   `internal/adapters/clients/psaportal/campaigns.go`) pages the portal's campaign list,
   enriches each entry with its edit-form subject/publisher filters, and the harvester
   writes the result via `snap.SaveSnapshot` into the singleton `psa_campaign_snapshot`
   table.
2. **Write: drain the approved push queue.** `psaportal.DrainPushQueue` reads all
   `psa_campaign_push_queue` rows with `status = 'approved'` and calls
   `portal.PushCampaign` (via `updateCampaign`, see below) for each one, marking the row
   `pushed` or `failed` based on the outcome.

**The main app never contacts psacard.com directly** for campaign sync — Cloudflare
IP-blocks the app server the same way it would block any non-browser-UA request from a
Fly app IP. The app only:
- Reads the latest snapshot (`GET /api/psa-campaigns`) to show portal campaign data.
- Writes rows into `psa_campaign_push_queue` (`psa-propose`) and flips their status to
  `approved` (`psa-publish`).

The harvester (which already has a real Playwright login flow and the right egress
profile) is the only process that talks to `psacard.com` for both the snapshot fetch and
the push.

### The three PSA portal endpoints used

All three are called with the harvester's browser-mimicking `User-Agent` and the
encrypted `accessToken` cookie, and are defined in
`internal/adapters/clients/psaportal/`:

- **List:** `GET /buyercampaignmanager/__data.json?x-sveltekit-trailing-slash=1&x-sveltekit-invalidated=001`
  (`campaignsListPath` in `sveltekit.go`) — paginated (`&page=N`); the SvelteKit
  ref-packed response is decoded (`DecodeRefPacked`) down to a
  `campaignsResponse.items[]` array plus `pageSize`/`totalCount`, and each item is mapped
  into a `PortalCampaign` (`campaigns.go`).
- **Edit (per campaign):** `GET /buyercampaignmanager/campaigns/{campaignRequestId}/edit/__data.json?x-sveltekit-invalidated=0001`
  (`campaignEditPathF`) — used both to enrich the list snapshot with subject/publisher
  filters (`fetchCampaignFormData`) and, in `PushCampaign`, to fetch the current
  `formData` object that gets read-modify-written before pushing changes back.
- **Update:** `POST /buyercampaignmanager/_app/remote/{buildHash}/updateCampaign`
  (`push.go`) — the mutated `formData` (only the changed fields are overwritten; numeric
  fields listed in `numericFormDataFields` are coerced to JSON numbers) is re-encoded
  with `EncodeRefPacked`, base64'd into a `payload` field, and POSTed as
  `{"payload": ..., "refreshes": []}`. `{buildHash}` is resolved per-request via
  `fetchBuildHash`, since PSA's SvelteKit build hash changes on portal deploys.

### The human-approval gate

Campaign edits are never pushed automatically. The flow is:

1. The app computes a diff between an internal campaign and its linked PSA portal
   campaign (`POST /api/campaigns/{id}/psa-propose`), and writes a `pending` row to
   `psa_campaign_push_queue`.
2. A human reviews the proposed diff in the UI and clicks **Publish**
   (`POST /api/campaigns/{id}/psa-publish`), which flips the row to `approved` — this is
   the only state transition the app can perform on a queue row.
3. The next `cmd/psa-harvest` run (or a manual invocation) finds the `approved` row via
   `DrainPushQueue` and actually calls `updateCampaign` against psacard.com, marking the
   row `pushed` or `failed`.

So there is always at least one human click, plus one harvester run, between a proposed
change and it reaching PSA.

## Version coupling

`Dockerfile.harvest`'s `mcr.microsoft.com/playwright:vX-…` tag must match
`web/package.json`'s `@playwright/test` version so the npm-installed client matches the
browsers baked into the base image. Bump them together.
