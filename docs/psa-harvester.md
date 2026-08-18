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

**"Still has validity" is enforced, not assumed.** Injecting an *expired* token
is worse than injecting nothing: it does not authenticate, but it does stop the
portal from bouncing us to `/signin`, so the script reads its own dead cookie
back and reports it as a fresh harvest while every data fetch 403s. That is
exactly how the 2026-08-17 outage stayed invisible for eight hours — the token
row was rewritten hourly with the same already-expired value. Three checks now
close it: `ReusableToken` (Go) withholds a token inside a 5-minute expiry margin,
the script skips injection unless the JWT `exp` is still ahead, and both the
script and `readHandshake` hard-fail on an already-expired handshake token
(`ErrTokenExpired`) rather than persisting it.

### Production: Fly.io (current deploy)

Production runs on Fly. The harvester is a **separate Fly app** (`slabledger-psa-harvest`)
because it needs the Playwright/Chromium image, which is different from the lean app
image. It is run-to-completion, not a server.

One-time setup:

```bash
# 1) Create the app (no HTTP service, no machines yet).
fly apps create slabledger-psa-harvest

# 2) Secrets. ENCRYPTION_KEY, DATABASE_URL and PSA_PUSH_SIGNING_KEY MUST be byte-identical
#    to the main `slabledger` app (the app decrypts what the harvester encrypts, and
#    signs the approvals the harvester verifies).
fly secrets set -a slabledger-psa-harvest \
  PSA_PORTAL_EMAIL='...' \
  PSA_PORTAL_PASSWORD='...' \
  ENCRYPTION_KEY='<same as slabledger>' \
  DATABASE_URL='<same Postgres URL as slabledger>' \
  PSA_PUSH_SIGNING_KEY='<same as slabledger>'

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

**The primary cadence is a GitHub Actions cron** (`.github/workflows/psa-harvest-cron.yml`):
every hour it runs `fly machine start` on the app's machine, which works regardless of the
machine's schedule state — so a deploy recreating the machine can no longer silently kill
the pipeline (which Fly's own scheduler did on 2026-07-16 and again on 2026-07-19). It
needs a `FLY_API_TOKEN` repo secret (`fly tokens create deploy -a slabledger-psa-harvest`).
A red run in the Actions tab is the alert surface; "machine already started" is treated as
success. Fly's on-machine schedule (below) stays as a second layer — GitHub cron is
best-effort and either layer alone keeps the pipeline alive; a double fire is a harmless
duplicate harvest.

The Fly-side cadence lives on the **machine**, not in `fly.harvest.toml` or the deploy — it
is set with `fly machine update --schedule` and is what makes Fly fire the machine on its own:

```bash
fly machine update <machine_id> --schedule hourly -a slabledger-psa-harvest
# verify:
fly machine status <machine_id> -a slabledger-psa-harvest --display-config | grep -i schedule  # -> "schedule": "hourly"
```

Accepted values are `hourly | daily | weekly | monthly` (there is no "every 12h"); `hourly`
gives a wide safety margin — a missed run still leaves the reader inside its 26h staleness
ceiling. Between runs the machine sits `stopped`; Fly starts it on schedule, it harvests,
and it exits 0.

### Alerting: the Healthchecks.io dead-man switch

The cron's staleness gate is the *detector*; a Healthchecks.io check is the *delivery*.
The workflow pings `$HEALTHCHECK_URL/start` at the top, the bare URL on success, and
`$HEALTHCHECK_URL/fail` (with the failing reason in the body) on any failure.

This exists because detection alone proved insufficient. On **2026-08-08** SLA-44 made
`PSA_PUSH_SIGNING_KEY` mandatory in `cmd/psa-harvest/main.go`, but the secret was never
set on either Fly app. The harvester exited 1 before launching Chromium, and the staleness
gate correctly went red within ~3 hours — then stayed red **every hour for eight days**
while the snapshot froze at `2026-08-08T18:41:27Z`, because a failing scheduled workflow
only produces a GitHub notification email. Nobody read it.

The dead-man half matters independently of the `/fail` pings: it catches the failures the
workflow cannot report about itself — GitHub disabling the schedule after 60 days of repo
inactivity, an expired `FLY_API_TOKEN` killing the job before any step runs, an Actions
outage, or the workflow file being deleted. A workflow that never runs cannot alert you.

```bash
# Create a check (period 1h, grace 45m) at https://healthchecks.io, then:
gh secret set HEALTHCHECK_URL --body 'https://hc-ping.com/<uuid>'
```

Point the check's notification at a channel you actually read. `HEALTHCHECK_URL` being
unset is a deliberate soft skip — no pings are sent at all, so the check goes down and the
misconfiguration surfaces through the alerting itself rather than turning the harvest red.

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

> **A displayed schedule is not proof the scheduler is firing.** After the 2026-07-16
> deploy recreated the machine, its config still showed `"schedule": "hourly"` but Fly
> never started it again — zero start events for two days. After any deploy, also check
> the event log (`fly machine status <machine_id>`) or the harvester's DB writes
> (`psa_portal_token.updated_at`) for a run *after* the deploy time. If the machine
> isn't firing, re-assert with `fly machine update <machine_id> --schedule hourly` and
> kick one run with `fly machine start <machine_id>`.

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
| `PSA_PUSH_SIGNING_KEY` | harvester + app | HMAC key authenticating push approvals; **must be byte-identical on both** or the harvester rejects every approval the app signs. ≥32 chars (`openssl rand -hex 32`). **Mandatory since SLA-44: unset on the harvester and it exits 1 before launching Chromium — no harvest at all, not merely no pushes.** Unset on the app ⇒ `psa-publish` returns 503 |
| `PSA_PUSH_SIGNING_KEY_ID` | harvester + app | opaque label for the key above, for rotation; not a secret. Defaults to `default` |

The **main app** needs `ENCRYPTION_KEY` + `DATABASE_URL` (to decrypt/read the token),
`PSA_SYNC_ENABLED=true` to run the daily import, **and** `PSA_PORTAL_EMAIL` +
`PSA_PORTAL_PASSWORD`. The app never logs in, but it only *wires* the portal sync when
those credentials are present (`PSAPortal.Enabled = email != "" && password != ""` in
`config/loader.go`; the client is constructed in `cmd/slabledger/main.go`). Config also
rejects setting just one of the pair — set both or neither. So in practice the four
credential vars go on both apps, with `ENCRYPTION_KEY`/`DATABASE_URL` identical across
them — plus `PSA_PUSH_SIGNING_KEY`, which must also match on both for the push path to
work at all.

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
   `DrainPushQueue`, **verifies the approval signature**, and only then calls
   `updateCampaign` against psacard.com, marking the row `pushed` or `failed`.

So there is always at least one human click, plus one harvester run, between a proposed
change and it reaching PSA.

#### Why the approval is signed (SLA-44)

The `approved` status is a column, and the threat model is an adversary who can write
columns — a leaked `service_role` key, a compromised PostgREST path, or direct database
access. Flipping `status` to `approved` would otherwise be enough to make the harvester
push arbitrary campaign changes to psacard.com on the next run. RLS (migrations 000027–
000029) closes the anon/authenticated path but not the `service_role` one.

So the approval carries an HMAC-SHA256 signature over a canonical encoding of the row's
identity **and a digest of the exact payload the operator was shown**
(`internal/domain/psacampaign/approval.go`, signed by `internal/platform/crypto`). The
key lives only in `PSA_PUSH_SIGNING_KEY` in the environment — never in the database — so
a writer who can set `status` cannot forge the signature that makes it count.
`DrainPushQueue` verifies before any portal call and marks an unverifiable row `failed`
rather than pushing it.

Two further guards close the ways a *valid* signature could still be replayed against the
wrong target:

- **Creates** are checked against the portal itself before pushing, so a re-approved
  create cannot duplicate a campaign that already exists.
- **Updates** compare-and-swap against the live portal record: if the campaign changed
  since the diff was computed, the push is refused rather than clobbering the newer
  state. Both sides render through one shared canonical renderer, pinned by
  `TestPortalListAndEditFormRenderIdentically`, so an unordered portal response never
  reads as drift.

**Operationally this means an approval is bound to one payload.** If a queued push is
edited, superseded, or fails and is retried, it must be re-proposed and re-approved — the
old signature will not verify against the new payload, and the UI will refuse the publish
with a 409 rather than pushing something the operator did not review.

**Key rotation:** `PSA_PUSH_SIGNING_KEY_ID` labels the key in each row it signs, so a
rotation does not strand approvals already queued. Rotate by setting a new key **and** a
new key ID on both the app and the harvester together. Any row approved under the old key
must be re-approved — a rotation invalidates in-flight approvals by design.

**If the key is unset,** `psa-publish` refuses with `503 PSA push approval signing is not
configured` — nothing can be approved at all, and the drain independently fails any row
it cannot verify. Set the key on both apps (see Env below) before relying on the push
path.

## Version coupling

`Dockerfile.harvest`'s `mcr.microsoft.com/playwright:vX-…` tag must match
`web/package.json`'s `@playwright/test` version so the npm-installed client matches the
browsers baked into the base image. Bump them together. A mismatch crashes every harvest
at `chromium.launch()` (`Executable doesn't exist … Please update docker image as well`),
which is upstream of login and took the pipeline down for days after a Dependabot bump on
2026-07-29. `scripts/check-playwright-version.sh` (wired into `make check` and CI) now
fails loudly on any mismatch, so a future client bump can't ship without the image bump.

## Baseline pull (one-time targeting migration)

`cmd/psa-harvest -baseline-pull` performs the one-time copy of live portal targeting
(languages, subject list, denied specs) into `campaigns.target_languages` /
`subject_filter_mode` / `subjects` / `denied_specs`. A campaign may carry more than one
curated language list — all six live campaigns carry both "Pokemon - English Language
Only" and "Pokemon - Japanese Language Only" — and every recognized list is copied, not
collapsed to one. It makes **zero portal writes** — the flag returns before
`DrainPushQueue` runs. Run it once, review the report, and only then resume the normal
(non-baseline) scheduled harvest.

```bash
docker run --rm \
  -e PSA_PORTAL_EMAIL="user@example.com" \
  -e PSA_PORTAL_PASSWORD="********" \
  -e ENCRYPTION_KEY="$ENCRYPTION_KEY" \
  -e DATABASE_URL="$DATABASE_URL" \
  slabledger-psa-harvest -baseline-pull
```

### Manual operator checklist

- [ ] Run the baseline pull once and confirm it **exits zero**. A non-zero exit most often
      means `runBaselinePull` (`cmd/psa-harvest/baseline.go`) skipped at least one linked
      campaign: its edit-form fetch was incomplete (`TargetingComplete == false`); it
      carries a curated spec-list name SlabLedger does not model, which is **refused, never
      silently dropped**, and is named in the log line (add the token in
      `internal/domain/inventory/validation.go`, `internal/domain/psacampaign/resolver.go`,
      `cmd/psa-harvest/baseline.go`, and `web/src/react/utils/campaignConstants.ts`
      together — the closed set is duplicated across all four); it names no recognized
      curated list at all (the CATEGORY-era shape, converted by hand in the portal); its
      portal targeting failed validation (e.g. a `subjectFilterType` that is neither
      `Target` nor `Exclude`); or it never appeared in the portal fetch. Any of these
      leaves that campaign's row unwritten (its pre-baseline targeting is left in place,
      never blanked out) — re-run until clean before trusting the copy. A non-zero exit can
      also mean the whole run aborted on an ordinary database failure (`ListCampaigns` or
      the campaign write returning an error) rather than a per-campaign skip; check the log
      line immediately before the exit to tell which case you're in.
- [ ] For at least one named, currently-linked campaign, open its edit page in the PSA
      portal UI directly and confirm the pulled `target_languages` / `subject_filter_mode` /
      `subjects` / `denied_specs` match what the portal UI shows — in particular that a
      campaign showing **both** "Pokemon - English Language Only" and "Pokemon - Japanese
      Language Only" landed with both tokens, not one. This is the check for a silently
      wrong translation, not just a successful fetch.
- [ ] Re-run the baseline a second time against campaigns whose portal targeting you know
      is unchanged, and confirm the copy is idempotent by diffing a direct snapshot of the
      affected columns from before and after. `runBaselinePull` has no diff or dedup logic
      of its own — it unconditionally rewrites `target_languages` / `subject_filter_mode` /
      `subjects` / `denied_specs` on every linked, complete campaign — so this is the only
      way to catch a real regression before trusting it for six active, money-spending
      campaigns:
      ```sql
      -- Before the second run: target_languages/subjects/denied_specs are all unordered
      -- sets, so each is sorted before comparison and the diff is order-insensitive,
      -- mirroring psacampaign/mapper.go's renderSubjectRefs (which exists precisely because
      -- "an unordered portal response never produces a spurious diff" — the edit-form fetch
      -- this baseline reads is not guaranteed order-stable across calls either).
      SELECT
        id,
        COALESCE((SELECT jsonb_agg(elem ORDER BY elem #>> '{}')
                  FROM jsonb_array_elements(target_languages) elem), '[]'::jsonb)
          AS target_languages_sorted,
        subject_filter_mode,
        COALESCE((SELECT jsonb_agg(elem ORDER BY (elem->>'id')::int)
                  FROM jsonb_array_elements(subjects) elem), '[]'::jsonb) AS subjects_sorted,
        COALESCE((SELECT jsonb_agg(elem ORDER BY (elem->>'id')::int)
                  FROM jsonb_array_elements(denied_specs) elem), '[]'::jsonb) AS denied_specs_sorted
      FROM campaigns
      WHERE psa_campaign_request_id IS NOT NULL AND psa_campaign_request_id <> ''
      ORDER BY id;
      -- (save this output, e.g. psql ... > before.txt)
      ```
      `elem #>> '{}'` extracts each language element as text, since `target_languages`
      holds bare JSON strings rather than objects with an `id`. Run `-baseline-pull` again,
      then run the identical query into `after.txt` and `diff before.txt after.txt`.
      Because all three arrays are sorted in both snapshots, a plain portal-side reorder of
      the same set collapses to identical output and won't show up as a diff — the *set* is
      the real signal, not raw array order. Any remaining difference on a campaign whose
      portal targeting genuinely did not change (added/removed/changed id, a gained or lost
      language token, or a different `subject_filter_mode`) is a real bug and must be fixed
      before trusting this baseline.
- [ ] Confirm no portal writes occurred during the baseline: check that every campaign's
      `updatedAt` in the PSA portal UI is unchanged from before the run, and that
      `psa_campaign_push_queue` gained no new rows (`runBaselinePull` never touches that
      table — only `HandlePSAPropose` on the main server enqueues rows).

### Deferred: spec discovery

`deniedSpecs` round-trips (pulled, decoded, diffed, and pushed) but there is no UI in
SlabLedger to *discover* a new card to deny — the modal that searches PSA's spec catalog
was never opened during HAR capture, so its request/response shape is unknown. Until a
capture with that modal open is taken, adding a new denial is done by hand in the PSA
portal and picked up on the next pull; this is the one intentional exception to "no direct
data entry in the portal."
