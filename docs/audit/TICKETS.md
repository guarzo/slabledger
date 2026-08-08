# Audit Tickets — draft bodies

**Baseline revision:** `740976ec`  
**Destination:** Linear team **SlabLedger** (no project — the workspace has none)  
**Count:** 35 tickets, one per fix unit in `REPORT.md`

Nothing here is filed until the user signs off. The `Linear ID` line on each ticket is filled in after creation so a partial failure mid-run is recoverable.

---

## FU-01 — Enable RLS on psa_campaign_push_queue and stop trusting its status column

**Linear:** [SLA-9](https://linear.app/slabledger/issue/SLA-9/enable-rls-on-psa-campaign-push-queue-and-stop-trusting-its-status)  
**Label:** Bug · **Priority:** 2

<details><summary>Ticket body as posted</summary>

**Category:** Security & data integrity · **Severity:** medium · **Confidence:** strong · **Effort:** S
**Baseline revision:** `740976ec` · **Audit findings:** DBSCHEMA-002

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> Highest-risk finding in the audit. An unauthenticated PostgREST write of a forged `approved` row is pushed live to the PSA portal. Fix the RLS gap and the trust assumption in DrainPushQueue together — either alone leaves the path exploitable.

## Claim

psa_campaign_push_queue has no RLS, and the harvester's DrainPushQueue trusts the DB's status column alone — an unauthenticated PostgREST write of a forged 'approved' row would be pushed live to the PSA portal

**Subject:** `{'kind': 'table', 'identity': 'psa_campaign_push_queue'}`

**Verifier correction (already applied to this ticket):** The finding names the wrong control and therefore prescribes a fix that would not prevent the attack it describes. Adding RLS 'matching 000003's pattern' means `USING (true) WITH CHECK (true)` with no TO clause, i.e. TO PUBLIC — a policy that passes every role, as 000003's own comment at lines 111-112 concedes. A forged approved row would be equally insertable after that migration lands. The absence of RLS is consequently not the differentiator here: an attacker with the same PostgREST access could just as well UPDATE public.campaigns or public.user_tokens, both of which DO have RLS from 000003. Severity 'high' also assumes the first hop, which is the one segment I could not verify and which the finding's own exclusions concede is unverifiable. What survives, and survives cleanly, is a real defense-in-depth defect that is independent of RLS entirely: a live, irreversible external side effect against a third-party portal is authorized by a single mutable database column, with the recorded approver identity never checked at execution time. That is worth a ticket, retitled to say that, at medium. Recommended reframing: 'DrainPushQueue treats the status column as its sole authorization gate for a live PSA portal write; approved_by is recorded but never verified.' The RLS component should fold into the DBSCHEMA-001/-003 remediation rather than carry this finding's severity.

### Evidence

- psa_campaign_push_queue is created in 000018 with no ENABLE ROW LEVEL SECURITY / CREATE POLICY anywhere in any of the 25 migrations.
  - Reproduce: `grep -n 'ENABLE ROW LEVEL SECURITY\|CREATE POLICY' internal/adapters/storage/postgres/migrations/*.up.sql | grep -w psa_campaign_push_queue`
- DrainPushQueue's only gate before pushing a row live to PSA is the DB row's own status column ('approved'); there is no additional application-level signature or re-authorization check at drain time.
  - Reproduce: `grep -n "status = 'approved'" internal/adapters/storage/postgres/psa_campaign_push_queue_store.go`
- The table is heavily live (21 references) and holds a human-approval workflow (requested_by/approved_by/proposed_diff/status), so an RLS gap here is a write-path integrity issue, not just a read-confidentiality one.
  - Reproduce: `jq '.records[] | select(.identity=="table:psa_campaign_push_queue") | .external_refs' docs/audit/maps/db-map.json`

### Proposed fix

Add RLS (ENABLE ROW LEVEL SECURITY + service-role-bypass policy, matching 000003's pattern) for psa_campaign_push_queue in a new migration. Treat this one ahead of the others in DBSCHEMA-003 given the write-integrity consequence (forged approval rows reaching the live PSA portal).

### Blast radius

- `internal/adapters/storage/postgres/migrations/ (new migration file)`

### Acceptance criteria

- [ ] A new migration's .up.sql enables RLS and adds a policy for psa_campaign_push_queue; .down.sql reverses it.
- [ ] grep -n 'ENABLE ROW LEVEL SECURITY\\|CREATE POLICY' internal/adapters/storage/postgres/migrations/*.up.sql | grep -w psa_campaign_push_queue now returns two lines.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-02 — Enable RLS on the six remaining tables created after the 000003 blanket grant

**Linear:** [SLA-10](https://linear.app/slabledger/issue/SLA-10/enable-rls-on-the-six-remaining-tables-created-after-the-000003)  
**Label:** Bug · **Priority:** 2

<details><summary>Ticket body as posted</summary>

**Category:** Security & data integrity · **Severity:** low · **Confidence:** strong · **Effort:** S
**Baseline revision:** `740976ec` · **Audit findings:** DBSCHEMA-001, DBSCHEMA-003

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> Seven tables in total lack RLS; one of them (psa_campaign_push_queue) is ticketed separately above because its fix is not mechanical. The controller's original seed said eight — that was wrong, `mm_sales_comps` was dropped in 000021. Any ticket citing eight is citing the controller's mistake. psa_portal_token stores an encrypted PSA portal access token.

## DBSCHEMA-001 — psa_portal_token — a table storing an (encrypted) PSA portal access token — was created after the 000003 blanket RLS grant and has never had RLS enabled

**Subject:** `{'kind': 'table', 'identity': 'psa_portal_token'}`

**Verifier correction (already applied to this ticket):** Two things, one of them decisive. (1) The stated severity 'high' does not follow from the consequence the finding itself correctly scoped. Read exposure is AES-256-GCM ciphertext plus expires_at. Write exposure is smaller than the finding implies: cmd/psa-harvest/main.go:97 reads the row as `storedToken, _, _ := store.CurrentToken(ctx)` with an explicit best-effort comment ('"" just means full SSO'), so corrupting or deleting the singleton row degrades the next harvest to a full SSO login rather than breaking it, and the CHECK (id = 1) singleton constraint bounds insertion to that one row. An attacker cannot forge a usable token without ENCRYPTION_KEY, which lives outside the database. Real severity is low. (2) More seriously, the proposed_fix and acceptance_criteria are a security no-op as written. They instruct the fixer to follow 000003's pattern, but every policy in 000003 is `CREATE POLICY "service role bypass" ... USING (true) WITH CHECK (true)` with no TO clause, which defaults to TO PUBLIC and therefore passes every role. 000003's own header comment at lines 111-112 says so: 'the USING (true) policy allows all operations for any role.' Enabling RLS that way leaves the table exactly as reachable as it is now; the only control that actually gates a PostgREST caller is the table GRANT, and no migration in this repo issues any GRANT or REVOKE (git grep -nE 'GRANT|REVOKE|ALTER DEFAULT PRIVILEGES' over migrations/*.sql is empty). The ticket must require a restrictive policy (TO service_role, or a REVOKE from anon/authenticated), or the fixer will ship a migration that satisfies every acceptance criterion and changes nothing. Note also that this makes the finding's implicit premise — that the 37 tables which did get RLS in 000003 are protected and these are not — false; they are equally reachable.

### Evidence

- psa_portal_token is created in 000016 with no ENABLE ROW LEVEL SECURITY / CREATE POLICY anywhere in any of the 25 migrations.
  - Reproduce: `cat internal/adapters/storage/postgres/migrations/000016_psa_portal_token.up.sql`
- No RLS statement for this table exists in any migration (grep across all 25 .up.sql files, filtered to this table name, is empty).
  - Reproduce: `grep -n 'ENABLE ROW LEVEL SECURITY\|CREATE POLICY' internal/adapters/storage/postgres/migrations/*.up.sql | grep -w psa_portal_token`
- db-map.json's own scout-computed rls_status for this table agrees: none, and dates its creation as after the 000003 blanket pass.
  - Reproduce: `jq -r '.records[] | select(.identity=="rls:psa_portal_token") | .command' docs/audit/maps/db-map.json`
- The stored value is application-layer encrypted before insert and decrypted only in Go (PSAPortalTokenStore uses crypto.Encryptor), so a PostgREST reader without the app's ENCRYPTION_KEY gets ciphertext, not the plaintext PSA token — this narrows but does not eliminate the exposure (ciphertext + ability to overwrite/delete the singleton row and observe expires_at are still exposed).
  - Reproduce: `git grep -n 'Encrypt\|Decrypt' internal/adapters/storage/postgres/psa_portal_token_store.go`
- The table is live (7 references), so this is a live-table gap, not a stale record about a dropped object.
  - Reproduce: `jq '.records[] | select(.identity=="table:psa_portal_token") | {external_refs, ref_sites}' docs/audit/maps/db-map.json`

### Proposed fix

Add a new migration enabling RLS and a service-role-bypass policy for psa_portal_token, following the exact pattern used in 000003 (ALTER TABLE ... ENABLE ROW LEVEL SECURITY; CREATE POLICY "service role bypass" ON ... USING (true) WITH CHECK (true);). Consider also auditing whether this table needs to exist in the public Supabase project at all, given it holds token material even if encrypted.

### Blast radius

- `internal/adapters/storage/postgres/migrations/ (new migration file)`
- `internal/adapters/storage/postgres/psa_portal_token_store.go (no code change expected)`

### Acceptance criteria

- [ ] A new migration's .up.sql contains 'ALTER TABLE public.psa_portal_token ENABLE ROW LEVEL SECURITY;' and a CREATE POLICY statement for it; its .down.sql reverses both.
- [ ] grep -n 'ENABLE ROW LEVEL SECURITY\\|CREATE POLICY' internal/adapters/storage/postgres/migrations/*.up.sql | grep -w psa_portal_token now returns two lines.
- [ ] go test ./... passes (migrations apply cleanly against a local Postgres).

## DBSCHEMA-003 — Five more live tables (dh_comp_cache, dh_card_tombstones, psa_portal_snapshot, psa_campaign_snapshot, psa_portal_catalog) were created after the 000003 blanket RLS grant and never received RLS — grouped because the fix is identical for each

**Subject:** `{'kind': 'table', 'identity': 'dh_comp_cache'}`

**Verifier correction (already applied to this ticket):** Same two defects as DBSCHEMA-001, plus one nuance on the stated test. (1) Severity 'high' does not follow. Four of the five are caches or singleton snapshots of PSA/DH reference data (dh_comp_cache, dh_card_tombstones, psa_portal_snapshot, psa_campaign_snapshot, psa_portal_catalog); none holds credentials or user data, and corruption degrades a scheduled harvest rather than compromising anything. Real severity is low: this is convention drift against the project's own 000003 pass, and it is what a Supabase rls_disabled_in_public linter run would flag. (2) The fix is a no-op as specified. 'Enabling RLS + service-role-bypass policy (000003's pattern)' produces `USING (true) WITH CHECK (true)` with no TO clause — TO PUBLIC — which grants no protection; 000003's header at lines 111-112 states this outright. The acceptance criterion 'grep -c ENABLE ROW LEVEL SECURITY returns 5' would pass on a migration that changes nothing, so the ticket needs a criterion requiring a restrictive policy or an explicit REVOKE from anon/authenticated. This also means the finding's framing — that these tables are the gap and the 37 RLS'd tables are fine — is not right; on the current policy shape all 44 are equally reachable, and the actual gate is the table GRANT, which no migration in this repo sets. (3) Minor: 'created after the 000003 blanket grant' is close but not an exact test. 000008:18 created psa_exchange_policy after 000003 and did enable RLS on it, so the convention was honored at least once post-000003 — the accurate statement is 'created after 000003 and never retrofitted', which is what the evidence actually establishes.

### Evidence

- None of these five tables has an ENABLE ROW LEVEL SECURITY or CREATE POLICY statement anywhere in the 25 migrations.
  - Reproduce: `for t in dh_comp_cache dh_card_tombstones psa_portal_snapshot psa_campaign_snapshot psa_portal_catalog; do grep -n 'ENABLE ROW LEVEL SECURITY\|CREATE POLICY' internal/adapters/storage/postgres/migrations/*.up.sql | grep -w "$t"; done`
- All five are live (non-zero Go external_refs, i.e. not dead/dropped tables) per db-map.json.
  - Reproduce: `jq -r '.records[] | select(.kind=="table" and (.identity|test("dh_comp_cache|dh_card_tombstones|psa_portal_snapshot|psa_campaign_snapshot|psa_portal_catalog"))) | "\(.identity) refs=\(.external_refs)"' docs/audit/maps/db-map.json`
- None of the five is dropped by any later migration (confirmed by scanning every migration for DROP TABLE of each name).
  - Reproduce: `for t in dh_comp_cache dh_card_tombstones psa_portal_snapshot psa_campaign_snapshot psa_portal_catalog; do grep -l "DROP TABLE.*$t\b" internal/adapters/storage/postgres/migrations/*.up.sql; done`
- The eighth table this seed originally flagged, mm_sales_comps, is NOT part of this gap: it was dropped by 000021 (DROP TABLE IF EXISTS mm_sales_comps;) and no longer exists at HEAD, so there is nothing to retrofit RLS onto. Reported here to correct the seed's count from 8 down to 7 live tables (this finding's 5 + DBSCHEMA-001's psa_portal_token + DBSCHEMA-002's psa_campaign_push_queue).
  - Reproduce: `grep -n 'mm_sales_comps' internal/adapters/storage/postgres/migrations/000021_drop_market_movers.up.sql`

### Proposed fix

Add a single new migration enabling RLS + service-role-bypass policy (000003's pattern) for all five tables at once, alongside psa_portal_token (DBSCHEMA-001) and psa_campaign_push_queue (DBSCHEMA-002) if the team prefers one consolidated remediation migration instead of three.

### Blast radius

- `internal/adapters/storage/postgres/migrations/ (new migration file)`

### Acceptance criteria

- [ ] A new migration's .up.sql enables RLS and adds a policy for each of dh_comp_cache, dh_card_tombstones, psa_portal_snapshot, psa_campaign_snapshot, and psa_portal_catalog; .down.sql reverses all five.
- [ ] grep -c 'ENABLE ROW LEVEL SECURITY' against the new migration file returns 5, one per table.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-03 — Wire CleanupExpiredOAuthStates into the session-cleanup scheduler and restore its index

**Linear:** [SLA-11](https://linear.app/slabledger/issue/SLA-11/wire-cleanupexpiredoauthstates-into-the-session-cleanup-scheduler-and)  
**Label:** Bug · **Priority:** 2

<details><summary>Ticket body as posted</summary>

**Category:** Security & data integrity · **Severity:** low · **Confidence:** mechanical · **Effort:** S
**Baseline revision:** `740976ec` · **Audit findings:** DEADGO-004

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> ADJUDICATED — this is the bug half of a SPLIT finding. DEADGO-004 was filed as six dead methods. Five are dead; this one is an unwired implementation of NEEDED behavior. `oauth_states` has no FK cascade and no TTL, the only consuming DELETE fires on successful callback only, and migration 000003 line 254 DROPped the index supporting the cleanup query. Every abandoned login leaves a permanent row. Deleting this method — which the finding as filed proposed — would have removed the fix for a live leak.

## Claim

Six of auth.Repository's 24 methods are never called through the interface anywhere — five form a dead token-lifecycle path, the sixth is a separately-dead OAuth-state cleanup

**Subject:** `{'kind': 'symbol', 'identity': 'internal/domain/auth.Repository: GetTokens, GetTokensByUserID, UpdateTokens, DeleteTokens, DeleteAllUserTokens, CleanupExpiredOAuthStates'}`

**Verifier correction (already applied to this ticket):** Three defects, none of which overturn the central claim. (1) The holder count is wrong. Evidence #5 asserts "the only other two holders of an auth.Repository-typed variable are cmd/slabledger/main.go's authRepo and google.OAuthService's repo field," and runtime_checks.interface_satisfaction repeats it. There is a third: internal/domain/auth/service_impl.go:24 `repo Repository` (plus the New() parameter at :44), which is unqualified in-package and therefore invisible to the `auth\.Repository` grep the finding used. The finding's own evidence #4 enumerates that holder's call sites separately, so its conclusion survives -- but a verifier trusting the stated count and the stated command would have missed a holder entirely. The count should be three. (2) Evidence #1's output claims each name "resolves to exactly 4 lines"; each actually resolves to 5 lines across 4 files (the doc comment above each implementation matches too). Cosmetic -- the file set is right and the zero-call-site conclusion is right. (3) Substantive: the finding bundles five genuinely-removable methods with one that should not be removed, and its acceptance_criteria do not force the fixer to tell them apart. Criterion 2 reads "git grep for each of the six method names returns no results (or, for whichever is wired instead of removed, shows exactly one new call site...)" -- a fixer can satisfy every criterion by deleting all six, which would make the oauth_states leak permanent and erase the only evidence that the cleanup path was ever intended. The proposed_fix mentions wiring as an option but leaves the choice to the fixer; a mechanical-tier finding must not do that.

### Evidence

- For each of the six candidate methods, the only occurrences anywhere in the tree are the interface declaration, the Postgres implementation, and the two test mocks — no call site through any variable of interface type auth.Repository or auth.Service exists, in production or in tests.
  - Reproduce: `for fn in GetTokens GetTokensByUserID UpdateTokens DeleteTokens DeleteAllUserTokens CleanupExpiredOAuthStates; do git grep -n "\b$fn\b" -- '*.go'; done`
- Sanity check: the same grep technique correctly surfaces live call chains for sibling methods on the same interface (StoreTokens, GetSession, DeleteSession), which show call sites in google/oauth.go, service_impl.go, and httpserver handlers — confirming the absence of such call sites for the six candidates is a real signal, not a blind spot in the search.
  - Reproduce: `for fn in StoreTokens GetSession DeleteSession; do git grep -n "\b$fn\b" -- '*.go' | grep -v auth_token_store.go; done`
- auth.Service — the interface every consumer above the repository actually holds and calls through (handlers, middleware, schedulers) — does not expose GetTokens, GetTokensByUserID, UpdateTokens, DeleteTokens, DeleteAllUserTokens, or CleanupExpiredOAuthStates at all; it only exposes StoreTokens from the token group, and CleanupExpiredSessions (a different method, for session records) instead of CleanupExpiredOAuthStates.
  - Reproduce: `sed -n '1,48p' internal/domain/auth/service.go`
- authService.s.repo (the concrete implementation backing auth.Service) never calls any of the six methods on its embedded repo field — every s.repo.X() call in service_impl.go is enumerated and none of the six appear.
  - Reproduce: `git grep -n 's\.repo\.' internal/domain/auth/service_impl.go`
- The only other two holders of an auth.Repository-typed variable are cmd/slabledger/main.go's authRepo (assigned then only passed into a constructor, never called directly) and google.OAuthService's embedded repo field (whose own methods — StoreTokens/GetSession/DeleteSession/etc. — were individually grepped and never call the six candidates either).
  - Reproduce: `git grep -n 'auth\.Repository\b' -- '*.go' | grep -v '_test.go\|mocks'; git grep -n 'authRepo\.' cmd/slabledger/*.go`
- No admin/session-management handler references token revocation, per-session deletion, or OAuth-state cleanup by name, ruling out an unnamed indirect path.
  - Reproduce: `git grep -niE 'revoke|logout.?all|delete.*token' internal/adapters/httpserver/handlers/*.go | grep -v _test.go`

### Proposed fix

Two logically distinct removals, since the six methods split into a coherent token-lifecycle group and one unrelated method: (1) remove GetTokens, GetTokensByUserID, UpdateTokens, DeleteTokens, DeleteAllUserTokens from the auth.Repository interface, postgres.AuthRepository (auth_token_store.go), and both mocks — the per-session multi-device token read/update/delete path is entirely unused; only StoreTokens (create) is live. (2) separately remove CleanupExpiredOAuthStates from the interface, implementation (auth_oauth_state_store.go), and both mocks, or wire it into the existing session-cleanup scheduler (internal/adapters/scheduler/session_cleanup.go) if unbounded growth of the oauth_states table is an operational concern — that scheduler currently only calls CleanupExpiredSessions, not this method. Keep the auth.Repository interface itself; only these six methods and their implementations are in scope.

### Blast radius

- `internal/domain/auth/repository.go`
- `internal/adapters/storage/postgres/auth_token_store.go`
- `internal/adapters/storage/postgres/auth_oauth_state_store.go`
- `internal/testutil/mocks/auth_inmemory.go`
- `internal/testutil/mocks/auth_repository.go`

### Acceptance criteria

- [ ] go build ./... and go test ./... pass with the six methods removed from the interface, the Postgres implementation, and both mocks (or, for CleanupExpiredOAuthStates, with it wired into session_cleanup.go and covered by a scheduler test).
- [ ] git grep for each of the six method names returns no results (or, for whichever is wired instead of removed, shows exactly one new call site in the scheduler plus its test).
- [ ] The underlying Postgres tables/queries these methods issued are reviewed separately by the db lens before any migration is considered — this finding covers only the Go-level dead methods, not schema changes.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-04 — Fix the flat-sibling import checker: it skips 19 of 25 domain packages

**Linear:** [SLA-12](https://linear.app/slabledger/issue/SLA-12/fix-the-flat-sibling-import-checker-it-skips-19-of-25-domain-packages)  
**Label:** Bug · **Priority:** 2

<details><summary>Ticket body as posted</summary>

**Category:** Security & data integrity · **Severity:** high · **Confidence:** strong · **Effort:** S
**Baseline revision:** `740976ec` · **Audit findings:** NB-001

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> ADJUDICATED — confirmed and widened by the controller. check-imports.sh:39 hardcodes six package names; `git ls-files 'internal/domain/*'` yields 25. The checker iterates the name list rather than the directory, so 19 packages are never opened. Confirmed uncaught violation: dhpricing imports dhlisting from three files. Consequence: every 'make check passes, so the hexagonal invariant holds' claim rests on a check with a 19-of-25 blind spot. The ticket must ASK whether a sub-package may import the inventory core rather than presume it — check-imports.sh:7-8 documents only sibling-to-sibling.

## Claim

The flat-sibling rule enforces a hardcoded six-package list that has not kept pace with the ten domain siblings that now exist; dhpricing already imports dhlisting uncaught

**Subject:** `{'kind': 'config', 'identity': 'scripts/check-imports.sh:39 SUB_PACKAGES'}`

**Verifier correction (already applied to this ticket):** Two roster errors in evidence claim 2, neither touching the conclusion. (a) The claim prose says 'internal/domain/ actually contains 24 packages'; its own pasted output lists 25, and I counted 25. (b) It names dhevents, liquidation and intelligence as 'inventory-siblings absent from the enforced list', but `git grep -rn 'domain/inventory' -- internal/domain/{dhevents,liquidation,intelligence}/` returns nothing for all three — they do not import inventory and are not siblings in the sense the rule governs. Conversely it omits internal/domain/pricing/lookup, whose adapter.go:12 does import inventory. The set of genuinely uncovered inventory-importing packages is demand, dhpricing, psacampaign (+ pricing/lookup as a sub-package), not the six it lists. A fixer deriving SUB_PACKAGES from the filesystem, as the proposed_fix directs, gets the right set regardless, so the acceptance criteria remain provable.

### Evidence

- The flat-sibling rule is enforced against a hardcoded list of exactly six packages.
  - `scripts/check-imports.sh:39`
  - Reproduce: `grep -n 'SUB_PACKAGES=' scripts/check-imports.sh`
- internal/domain/ actually contains 24 packages, of which demand, dhevents, dhpricing, liquidation, psacampaign and intelligence are inventory-siblings absent from the enforced list.
  - Reproduce: `git ls-files 'internal/domain' | awk -F/ 'NF>3{print $3}' | sort -u | tr '\n' ' '`
- internal/domain/dhpricing imports sibling internal/domain/dhlisting in two production files. The check passes only because dhpricing is not in SUB_PACKAGES.
  - `internal/domain/dhpricing/service.go:7`
  - Reproduce: `git grep -n 'domain/dhlisting' -- 'internal/domain/dhpricing/*.go'`
- demand, dhpricing and psacampaign import internal/domain/inventory in production files, i.e. they occupy the same structural position as the six enforced siblings.
  - Reproduce: `for p in demand dhpricing psacampaign; do printf '%s %s\n' "$p" "$(git grep -l 'domain/inventory' -- "internal/domain/$p/*.go" | grep -cv _test)"; done`

### Proposed fix

Derive SUB_PACKAGES from the filesystem (every directory under internal/domain/ that imports internal/domain/inventory) rather than hardcoding six names, so the rule cannot silently stop covering new packages. Then decide explicitly whether dhpricing -> dhlisting is an intended exception (and record it as one) or a violation to be broken by moving the shared types into inventory or a leaf package.

### Blast radius

- `scripts/check-imports.sh`
- `internal/domain/dhpricing/service.go`
- `internal/domain/dhpricing/types.go`
- `make check / CI`

### Acceptance criteria

- [ ] scripts/check-imports.sh derives its sibling list from the repository layout, and adding a new directory under internal/domain/ that imports inventory brings it under the rule with no edit to the script.
- [ ] bash scripts/check-imports.sh exits 0, and the dhpricing -> dhlisting edge is either removed or listed in an explicit, commented allowlist in the script.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-05 — Strip denominators in the DH cert-disambiguation card-number normalizer

**Linear:** [SLA-13](https://linear.app/slabledger/issue/SLA-13/strip-denominators-in-the-dh-cert-disambiguation-card-number)  
**Label:** Bug · **Priority:** 2

<details><summary>Ticket body as posted</summary>

**Category:** Security & data integrity · **Severity:** medium · **Confidence:** strong · **Effort:** S
**Baseline revision:** `740976ec` · **Audit findings:** DUP-003

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> A live defect, not a style issue: the local normalizer diverges from cardutil's shared version, causing silent disambiguation misses.

## Claim

DH cert-disambiguation's local card-number normalizer doesn't strip denominators, unlike cardutil's shared version, causing silent disambiguation misses

**Subject:** `{'kind': 'symbol', 'identity': 'dh.normalizeCardNumber vs cardutil.NormalizeCardNumber'}`

### Evidence

- dh.normalizeCardNumber only strips leading zeros; it does not strip a '/denominator' suffix the way cardutil.NormalizeCardNumber does.
  - `internal/adapters/clients/dh/disambiguate.go:34-40`
  - Reproduce: `sed -n '34,40p' internal/adapters/clients/dh/disambiguate.go`
- cardutil.NormalizeCardNumber explicitly strips a denominator ('006/165' -> '6') before stripping leading zeros -- the two functions solve the same 'normalize a collector number for comparison' problem but cardutil's is a superset of dh's.
  - `internal/platform/cardutil/normalize_cards.go:186-196`
  - Reproduce: `sed -n '186,213p' internal/platform/cardutil/normalize_cards.go`
- The purchase card number fed into dh.Disambiguate (via dh.ResolveAmbiguous) genuinely can carry the denominator format -- ParsePSAListingTitle's own inline comment cites '199/165' as a real collector-number token it extracts and returns as the parsed card number, which becomes inventory.Purchase.CardNumber.
  - `internal/domain/inventory/import_parsing.go:100-101`
  - Reproduce: `grep -n '199/165' internal/domain/inventory/import_parsing.go`
- The call chain confirms p.CardNumber (which can be '199/165'-shaped) flows directly into dh.ResolveAmbiguous -> dh.Disambiguate -> dh.normalizeCardNumber, while DH's own candidate.CardNumber values are bare integers (per the existing unit test fixtures for CertResolutionCandidate, e.g. '199', '234', '96', '0', '5' -- never a denominator form).
  - `internal/adapters/httpserver/handlers/dh_match_handler.go:251`
  - Reproduce: `grep -n 'dh.ResolveAmbiguous(resp.Candidates, p.CardNumber' internal/adapters/httpserver/handlers/dh_match_handler.go; grep -n 'CardNumber:' internal/adapters/clients/dh/disambiguate_test.go`
- Concretely demonstrated divergence: for a purchase card number '199/165' against a DH candidate with bare card number '199' (the shapes both sides actually produce per the evidence above), dh.normalizeCardNumber leaves the two unequal ('199/165' != '199', match fails) while cardutil.NormalizeCardNumber reduces both to '199' (match succeeds). Reproduced by transcribing each function's exact logic (both are simple, fully deterministic string operations with no external dependencies).
  - `internal/adapters/clients/dh/disambiguate.go:36`
  - Reproduce: 
```
python3 -c "
def normalize_dh(s):
    n = s.lstrip('0')
    return '0' if n=='' and len(s)>0 else n
def normalize_cardutil(number):
    if '/' in number:
        number = number[:number.index('/')]
    i = next((i for i,c in enumerate(number) if c.isdigit()), None)
    if i is None: return number
    prefix, num = number[:i], number[i:].lstrip('0') or '0'
    return prefix+num
purchase, candidate = '199/165', '199'
print('dh match:', normalize_dh(purchase) == normalize_dh(candidate))
print('cardutil match:', normalize_cardutil(purchase) == normalize_cardutil(candidate))
"
```

### Proposed fix

Replace the body of internal/adapters/clients/dh/disambiguate.go's normalizeCardNumber with a call to cardutil.NormalizeCardNumber (or otherwise delegate to it), so denominator-bearing purchase card numbers (e.g. from ParsePSAListingTitle) can still be matched against bare-integer DH candidate numbers during ambiguous-cert resolution.

### Blast radius

- `internal/adapters/clients/dh/disambiguate.go`
- `internal/adapters/httpserver/handlers/dh_match_handler.go`

### Acceptance criteria

- [ ] go build ./... succeeds with dh.normalizeCardNumber delegating to cardutil.NormalizeCardNumber
- [ ] go test ./internal/adapters/clients/dh/... passes, including the existing disambiguate_test.go table plus a new case asserting Disambiguate matches a '199/165'-shaped input against a bare '199' candidate
- [ ] go test ./internal/adapters/httpserver/handlers/... passes unchanged

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-06 — Move the fill-rate business rule out of the HTTP handler into the domain

**Linear:** [SLA-14](https://linear.app/slabledger/issue/SLA-14/move-the-fill-rate-business-rule-out-of-the-http-handler-into-the)  
**Label:** Bug · **Priority:** 2

<details><summary>Ticket body as posted</summary>

**Category:** Correctness · **Severity:** medium · **Confidence:** strong · **Effort:** M
**Baseline revision:** `740976ec` · **Audit findings:** ARCH-001

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> The domain fields designed to carry this rule (DailySpend.CapCents, FillRatePct) are never populated by the only repository, so the handler recomputes it — substituting the campaign's CURRENT cap for every historical day. The same rule is implemented a second time in the tuning rule.

## Claim

Fill-rate business rule lives in an HTTP handler; the domain fields designed to carry it are never populated by the only repository

**Subject:** `{'kind': 'symbol', 'identity': 'internal/domain/inventory.DailySpend (CapCents, FillRatePct) + fill-rate rule in internal/adapters/httpserver/handlers/campaigns_analytics.go'}`

**Verifier correction (already applied to this ticket):** Nothing material. One presentational note: evidence item 2 quotes its command output with an elision ('...') inside the SQL string; the full sed range does print the quoted lines, so this is abridgement rather than misquotation. Also worth recording for the fixer, though the finding does not overstate it: the handler's substitution of the campaign's current DailySpendCapCents for every historical day means the fill-rate chart is retroactively rewritten whenever a user edits the cap. The finding treats this as a design smell and does not claim it as a bug, which is the correct posture, since no per-day historical cap is persisted anywhere for the fix to use.

### Evidence

- The domain type DailySpend declares CapCents and FillRatePct, with a doc comment defining the rule as 'spend / cap'.
  - `internal/domain/inventory/analytics_types.go:29-35`
  - Reproduce: `sed -n '28,36p' internal/domain/inventory/analytics_types.go`
- The sole non-test implementation of AnalyticsRepository.GetDailySpend selects only three columns and scans only Date, SpendCents, PurchaseCount. CapCents and FillRatePct are therefore always zero on every value that crosses the repository boundary.
  - `internal/adapters/storage/postgres/analytics_store.go:81-99`
  - Reproduce: `sed -n '81,100p' internal/adapters/storage/postgres/analytics_store.go`
- The HTTP handler discards the domain field and recomputes fill rate itself, substituting the campaign's CURRENT DailySpendCapCents for every historical day.
  - `internal/adapters/httpserver/handlers/campaigns_analytics.go:86-93`
  - Reproduce: `sed -n '83,98p' internal/adapters/httpserver/handlers/campaigns_analytics.go`
- The same rule is implemented a second time, independently, inside the domain tuning rule — also ignoring ds.CapCents and using the campaign's current cap.
  - `internal/domain/inventory/tuning.go:232-238`
  - Reproduce: `sed -n '232,240p' internal/domain/inventory/tuning.go`
- GetDailySpend has exactly two non-test consumers: the HTTP handler and the tuning service. There is no third implementation of the repository method that might populate the missing fields.
  - Reproduce: `git grep -nE 'GetDailySpend' -- 'internal/**/*.go' 'cmd/**/*.go' | grep -v _test | grep -v testutil`

### Proposed fix

Move the fill-rate computation into internal/domain/inventory as a single exported function over (DailySpend, Campaign) and call it from both internal/domain/tuning and the handler, or have the service populate DailySpend.CapCents/FillRatePct before returning. Then delete the handler-local arithmetic. If the historical per-day cap is genuinely unavailable, delete DailySpend.CapCents and its 'spend / cap' comment so the type stops advertising a field nothing fills.

### Blast radius

- `internal/domain/inventory/analytics_types.go`
- `internal/domain/inventory/tuning.go`
- `internal/domain/inventory/service_analytics.go`
- `internal/adapters/httpserver/handlers/campaigns_analytics.go`
- `internal/adapters/storage/postgres/analytics_store.go`
- `web/src (the fillRatePct JSON field value is unchanged only if the same cap is used)`

### Acceptance criteria

- [ ] Exactly one expression computing spend/cap remains in the repository: `git grep -nE 'SpendCents\) / float64' -- internal/ | grep -v _test` returns a single line, inside internal/domain/.
- [ ] A domain-level table-driven unit test covers the fill-rate rule (including capCents == 0) without constructing an http.Request.
- [ ] GET /api/campaigns/{id}/fill-rate returns the same fillRatePct values as before the change for a campaign whose daily cap has never been edited.
- [ ] `go build ./...`, `go test -race ./...`, and `make check` pass.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-07 — Add the missing forcedLiquidation field to the TypeScript Sale type

**Linear:** [SLA-15](https://linear.app/slabledger/issue/SLA-15/add-the-missing-forcedliquidation-field-to-the-typescript-sale-type)  
**Label:** Bug · **Priority:** 2

<details><summary>Ticket body as posted</summary>

**Category:** Correctness · **Severity:** low · **Confidence:** strong · **Effort:** S
**Baseline revision:** `740976ec` · **Audit findings:** FE-007

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

## Claim

Sale.forcedLiquidation is sent unconditionally by the Go API but has no field in the TS Sale type

**Subject:** `{'kind': 'type', 'identity': 'web/src/types/campaigns/core.ts Sale interface — missing field forcedLiquidation'}`

**Verifier correction (already applied to this ticket):** The stated severity of 'medium' overstates the runtime consequence: a field present in the JSON response but absent from the TS interface is dropped by nothing — it simply isn't typed, so no runtime error, crash, or data loss occurs. TypeScript's structural typing means extra wire fields are inert until something tries to read them; this is a type-completeness/documentation gap, not a defect with an active symptom. The real consequence is that any future code wanting to read forcedLiquidation has no compiler-checked field to use, and a reviewer scanning the Sale type would not know the field exists.

### Evidence

- Go's Sale struct (internal/domain/inventory/types_core.go) declares ForcedLiquidation as a plain bool with no `omitempty` — it is always present on the wire — while the TS Sale interface in web/src/types/campaigns/core.ts (lines 114-145) has no forcedLiquidation field at all
  - `internal/domain/inventory/types_core.go:491`
  - Reproduce: `sed -n '488,492p' internal/domain/inventory/types_core.go`
- Zero references to forcedLiquidation anywhere in web/src — the frontend has no type for this field and cannot read or display it
  - Reproduce: `git grep -nE 'forcedLiquidation|ForcedLiquidation' web/src`

### Proposed fix

Add `forcedLiquidation: boolean;` to the Sale interface in web/src/types/campaigns/core.ts to match the Go contract. If the UI is meant to surface this (e.g. a badge distinguishing forced-liquidation sales in sale history/analytics), a follow-up UI task is separate from this type-sync fix.

### Blast radius

- `web/src/types/campaigns/core.ts`

### Acceptance criteria

- [ ] web/src/types/campaigns/core.ts Sale interface includes forcedLiquidation: boolean matching the Go json tag
- [ ] npm run build passes

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-08 — Remove the five dead auth token-lifecycle methods

**Linear:** [SLA-16](https://linear.app/slabledger/issue/SLA-16/remove-the-five-dead-auth-token-lifecycle-methods)  
**Label:** Improvement · **Priority:** 3

<details><summary>Ticket body as posted</summary>

**Category:** Dead code · **Severity:** low · **Confidence:** mechanical · **Effort:** S
**Baseline revision:** `740976ec` · **Audit findings:** DEADGO-004

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> ADJUDICATED — the removal half of the SPLIT finding. GetTokens, GetTokensByUserID, UpdateTokens, DeleteTokens, DeleteAllUserTokens are redundant with a live database constraint: migrations/000001_initial_schema.up.sql:104 declares `session_id TEXT REFERENCES user_sessions(id) ON DELETE CASCADE` on user_tokens, so logout and session cleanup already delete token rows at the DB level. The fixer must check THREE holders of an auth.Repository value, not the two the finding named — internal/domain/auth/service_impl.go:24 is invisible to a package-qualified grep. Do NOT include CleanupExpiredOAuthStates; it is ticketed separately as a bug.

## Claim

Six of auth.Repository's 24 methods are never called through the interface anywhere — five form a dead token-lifecycle path, the sixth is a separately-dead OAuth-state cleanup

**Subject:** `{'kind': 'symbol', 'identity': 'internal/domain/auth.Repository: GetTokens, GetTokensByUserID, UpdateTokens, DeleteTokens, DeleteAllUserTokens, CleanupExpiredOAuthStates'}`

**Verifier correction (already applied to this ticket):** Three defects, none of which overturn the central claim. (1) The holder count is wrong. Evidence #5 asserts "the only other two holders of an auth.Repository-typed variable are cmd/slabledger/main.go's authRepo and google.OAuthService's repo field," and runtime_checks.interface_satisfaction repeats it. There is a third: internal/domain/auth/service_impl.go:24 `repo Repository` (plus the New() parameter at :44), which is unqualified in-package and therefore invisible to the `auth\.Repository` grep the finding used. The finding's own evidence #4 enumerates that holder's call sites separately, so its conclusion survives -- but a verifier trusting the stated count and the stated command would have missed a holder entirely. The count should be three. (2) Evidence #1's output claims each name "resolves to exactly 4 lines"; each actually resolves to 5 lines across 4 files (the doc comment above each implementation matches too). Cosmetic -- the file set is right and the zero-call-site conclusion is right. (3) Substantive: the finding bundles five genuinely-removable methods with one that should not be removed, and its acceptance_criteria do not force the fixer to tell them apart. Criterion 2 reads "git grep for each of the six method names returns no results (or, for whichever is wired instead of removed, shows exactly one new call site...)" -- a fixer can satisfy every criterion by deleting all six, which would make the oauth_states leak permanent and erase the only evidence that the cleanup path was ever intended. The proposed_fix mentions wiring as an option but leaves the choice to the fixer; a mechanical-tier finding must not do that.

### Evidence

- For each of the six candidate methods, the only occurrences anywhere in the tree are the interface declaration, the Postgres implementation, and the two test mocks — no call site through any variable of interface type auth.Repository or auth.Service exists, in production or in tests.
  - Reproduce: `for fn in GetTokens GetTokensByUserID UpdateTokens DeleteTokens DeleteAllUserTokens CleanupExpiredOAuthStates; do git grep -n "\b$fn\b" -- '*.go'; done`
- Sanity check: the same grep technique correctly surfaces live call chains for sibling methods on the same interface (StoreTokens, GetSession, DeleteSession), which show call sites in google/oauth.go, service_impl.go, and httpserver handlers — confirming the absence of such call sites for the six candidates is a real signal, not a blind spot in the search.
  - Reproduce: `for fn in StoreTokens GetSession DeleteSession; do git grep -n "\b$fn\b" -- '*.go' | grep -v auth_token_store.go; done`
- auth.Service — the interface every consumer above the repository actually holds and calls through (handlers, middleware, schedulers) — does not expose GetTokens, GetTokensByUserID, UpdateTokens, DeleteTokens, DeleteAllUserTokens, or CleanupExpiredOAuthStates at all; it only exposes StoreTokens from the token group, and CleanupExpiredSessions (a different method, for session records) instead of CleanupExpiredOAuthStates.
  - Reproduce: `sed -n '1,48p' internal/domain/auth/service.go`
- authService.s.repo (the concrete implementation backing auth.Service) never calls any of the six methods on its embedded repo field — every s.repo.X() call in service_impl.go is enumerated and none of the six appear.
  - Reproduce: `git grep -n 's\.repo\.' internal/domain/auth/service_impl.go`
- The only other two holders of an auth.Repository-typed variable are cmd/slabledger/main.go's authRepo (assigned then only passed into a constructor, never called directly) and google.OAuthService's embedded repo field (whose own methods — StoreTokens/GetSession/DeleteSession/etc. — were individually grepped and never call the six candidates either).
  - Reproduce: `git grep -n 'auth\.Repository\b' -- '*.go' | grep -v '_test.go\|mocks'; git grep -n 'authRepo\.' cmd/slabledger/*.go`
- No admin/session-management handler references token revocation, per-session deletion, or OAuth-state cleanup by name, ruling out an unnamed indirect path.
  - Reproduce: `git grep -niE 'revoke|logout.?all|delete.*token' internal/adapters/httpserver/handlers/*.go | grep -v _test.go`

### Proposed fix

Two logically distinct removals, since the six methods split into a coherent token-lifecycle group and one unrelated method: (1) remove GetTokens, GetTokensByUserID, UpdateTokens, DeleteTokens, DeleteAllUserTokens from the auth.Repository interface, postgres.AuthRepository (auth_token_store.go), and both mocks — the per-session multi-device token read/update/delete path is entirely unused; only StoreTokens (create) is live. (2) separately remove CleanupExpiredOAuthStates from the interface, implementation (auth_oauth_state_store.go), and both mocks, or wire it into the existing session-cleanup scheduler (internal/adapters/scheduler/session_cleanup.go) if unbounded growth of the oauth_states table is an operational concern — that scheduler currently only calls CleanupExpiredSessions, not this method. Keep the auth.Repository interface itself; only these six methods and their implementations are in scope.

### Blast radius

- `internal/domain/auth/repository.go`
- `internal/adapters/storage/postgres/auth_token_store.go`
- `internal/adapters/storage/postgres/auth_oauth_state_store.go`
- `internal/testutil/mocks/auth_inmemory.go`
- `internal/testutil/mocks/auth_repository.go`

### Acceptance criteria

- [ ] go build ./... and go test ./... pass with the six methods removed from the interface, the Postgres implementation, and both mocks (or, for CleanupExpiredOAuthStates, with it wired into session_cleanup.go and covered by a scheduler test).
- [ ] git grep for each of the six method names returns no results (or, for whichever is wired instead of removed, shows exactly one new call site in the scheduler plus its test).
- [ ] The underlying Postgres tables/queries these methods issued are reviewed separately by the db lens before any migration is considered — this finding covers only the Go-level dead methods, not schema changes.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-09 — Remove the unwired AI image-generation cluster

**Linear:** [SLA-17](https://linear.app/slabledger/issue/SLA-17/remove-the-unwired-ai-image-generation-cluster)  
**Label:** Improvement · **Priority:** 3

<details><summary>Ticket body as posted</summary>

**Category:** Dead code · **Severity:** medium · **Confidence:** mechanical · **Effort:** S
**Baseline revision:** `740976ec` · **Audit findings:** DEADGO-002

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

## Claim

AI image-generation cluster (ai.ImageGenerator + azureai.ImageClient) is fully defined but never constructed or wired anywhere

**Subject:** `{'kind': 'symbol', 'identity': 'internal/domain/ai.ImageGenerator, ImageRequest, ImageResult + internal/adapters/clients/azureai.ImageClient, ImageOption, NewImageClient, WithImageLogger, GenerateImage'}`

**Verifier correction (already applied to this ticket):** One real gap, in scope rather than in the central claim: blast_radius lists only the two .go files, but the cluster also has two live documentation references that the deletion must reconcile — docs/LLM_USAGE.md:53, which reproduces the ImageGenerator interface body verbatim under 'Domain interface', and docs/ARCHITECTURE.md:68 and :264, where the interface table carries the row `| ai | ImageGenerator | llm.go | 1 | Image generation |`. I confirmed this bites: after deleting only the two blast_radius files in the scratch copy, a grep for the seven symbol names over docs/, internal/, cmd/, and web/ (excluding docs/audit/ itself) still returns docs/LLM_USAGE.md and docs/ARCHITECTURE.md. So acceptance_criteria #2 ('git grep for ImageGenerator, ImageClient, ... returns no results'), which is not scoped to '*.go', cannot be satisfied by the stated blast_radius alone. The criterion is not wrong — it correctly forces the fixer to the docs — but blast_radius understates the fix by two files, and the ticket should name them. Separately, that ARCHITECTURE.md row misattributes the interface to llm.go when it is declared in image.go; the row is being deleted anyway, so this only matters as a hint that the doc was already stale. Neither point disturbs the central claim, and I found nothing that would justify lowering severity: the cluster is documented in two places as though it were a live capability, which is precisely the kind of misdirection a medium-severity dead-code finding is for.

### Evidence

- azureai.NewImageClient (the cluster's only constructor) has zero callers anywhere in the tree, including tests.
  - Reproduce: `git grep -nE '\bNewImageClient\b' -- '*.go'`
- The azureai package is imported in exactly one production file (cmd/slabledger/init_services.go), and that file calls azureai.NewClient (the unrelated text/LLM client) — never NewImageClient.
  - Reproduce: `git grep -n '"github.com/guarzo/slabledger/internal/adapters/clients/azureai"' -- '*.go'; git grep -n 'azureai\.' cmd/slabledger/init_services.go`
- GenerateImage, the method implementing the interface, is called nowhere — its only two occurrences are its own definition and the interface method declaration it satisfies.
  - Reproduce: `git grep -n 'GenerateImage' -- '*.go'`
- ai.ImageGenerator, ai.ImageRequest, ai.ImageResult have references only from the single azureai/image_client.go file that declares/implements the interface — no third consumer (e.g. an HTTP handler or scheduler) exists anywhere.
  - Reproduce: `git grep -nE '\bImageGenerator\b|\bImageRequest\b|\bImageResult\b' -- '*.go'`
- No _test.go file anywhere references ImageClient, ImageGenerator, GenerateImage, ImageRequest, or ImageResult — the cluster is untested as well as unwired.
  - Reproduce: `git grep -lnE '\bImageClient\b|\bImageGenerator\b|\bGenerateImage\b|\bImageRequest\b|\bImageResult\b' -- '*_test.go'`
- go-reference-map independently flags WithImageLogger, NewImageClient, ImageClient, and ImageOption as external_refs==0.
  - Reproduce: `jq '.records[] | select(.identity | test("azureai\\.(ImageClient|ImageOption|NewImageClient|WithImageLogger)$"))' docs/audit/maps/go-reference-map.json`

### Proposed fix

Delete internal/adapters/clients/azureai/image_client.go and internal/domain/ai/image.go, unless there is a near-term product plan to wire AI image generation (advisor/campaign card imagery), in which case leave a tracking issue instead of deleting.

### Blast radius

- `internal/adapters/clients/azureai/image_client.go`
- `internal/domain/ai/image.go`

### Acceptance criteria

- [ ] go build ./... and go test ./... pass with both files removed.
- [ ] git grep for ImageGenerator, ImageClient, ImageRequest, ImageResult, NewImageClient, WithImageLogger, ImageOption returns no results.
- [ ] make check passes.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-10 — Remove the advisor functional options that are never invoked

**Linear:** [SLA-18](https://linear.app/slabledger/issue/SLA-18/remove-the-advisor-functional-options-that-are-never-invoked)  
**Label:** Improvement · **Priority:** 3

<details><summary>Ticket body as posted</summary>

**Category:** Dead code · **Severity:** low · **Confidence:** mechanical · **Effort:** S
**Baseline revision:** `740976ec` · **Audit findings:** DEADGO-003

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> WithMaxTokens and WithTemperature are never called, so the fields they set are permanently stuck at their defaults. Confirm that freezing those defaults is intended before deleting rather than wiring.

## Claim

advisor.WithMaxTokens and advisor.WithTemperature functional options are never invoked; the fields they set are permanently stuck at their defaults

**Subject:** `{'kind': 'symbol', 'identity': 'internal/domain/advisor.WithMaxTokens, internal/domain/advisor.WithTemperature'}`

**Verifier correction (already applied to this ticket):** Nothing material; both claims survive. Two presentational defects worth noting for whoever reads the finding rather than re-running it. (a) The `output` fields of evidence items 3 and 4 are paraphrases, not verbatim command output — item 3 reflows the source and elides an argument as `WithMaxToolRounds(...)`, and item 4 replaces grep output with an English summary. Both COMMANDS reproduce and both substantively say what the finding says they say, so this is not a reproduction failure, but it violates the LENS-BRIEF §4 rule against commentary in evidence fields. (b) Item 4's summary is slightly incomplete: it omits service.go:42 and :47 (the option bodies, which are themselves writes to the fields) and labels tool_loop.go:49 as the temperature read when :49 takes the address into a local that is used at :54. Neither omission changes the conclusion — I enumerated the full write and read sets myself above. One scoping note for the controller, not an error: `blast_radius` lists only internal/domain/advisor/service.go, which is correct for the removal branch but understates the wiring branch, which would also touch internal/platform/config/{types,defaults,loader}.go and cmd/slabledger/init_services.go.

### Evidence

- WithMaxTokens has no call site anywhere in the tree, including its own package's tests.
  - Reproduce: `git grep -n 'WithMaxTokens(' -- '*.go'`
- WithTemperature has no call site anywhere in the tree, including its own package's tests.
  - Reproduce: `git grep -n 'WithTemperature(' -- '*.go'`
- The only production construction of the advisor service (cmd/slabledger/init_services.go) builds its ServiceOption slice from WithLogger, WithAITracker, conditionally WithMaxToolRounds, WithScoringDataProvider, and WithGapStore — WithMaxTokens/WithTemperature are absent, so s.maxTokens and s.temperature are always left at defaultMaxTokens/defaultTemperature.
  - Reproduce: `sed -n '77,92p' cmd/slabledger/init_services.go`
- s.maxTokens and s.temperature are read only in tool_loop.go, confirming the fields are live business state — it is specifically the setter options that are unreachable, not the fields.
  - Reproduce: `git grep -n 'maxTokens\|temperature' internal/domain/advisor/*.go`

### Proposed fix

Either remove WithMaxTokens and WithTemperature (and hardcode/derive the defaults instead of exposing unreachable configurability), or wire them into cmd/slabledger/init_services.go from config if tunable max-tokens/temperature was the intent (e.g. via cfg.AdvisorRefresh, mirroring how MaxToolRounds is conditionally wired). This is a product decision, not a mechanical deletion call — flagging for the fixer to choose.

### Blast radius

- `internal/domain/advisor/service.go`

### Acceptance criteria

- [ ] go build ./... and go test ./... pass after either removing the two options or wiring them into cmd/slabledger/init_services.go.
- [ ] If removed: git grep for WithMaxTokens and WithTemperature returns no results, and maxTokens/temperature defaults remain documented at their current values.
- [ ] If wired: a config-driven override path exists and is covered by a service_test.go case analogous to the MaxToolRounds override test.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-11 — Resolve the cache-path plumbing that exists only to relocate a startup mkdir

**Linear:** [SLA-19](https://linear.app/slabledger/issue/SLA-19/resolve-the-cache-path-plumbing-that-exists-only-to-relocate-a-startup)  
**Label:** Improvement · **Priority:** 3

<details><summary>Ticket body as posted</summary>

**Category:** Dead code · **Severity:** medium · **Confidence:** strong · **Effort:** M
**Baseline revision:** `740976ec` · **Audit findings:** DCT-014

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> ADJUDICATED — CONSTRAINED FIX. The finding's diagnosis is right: Config.Cache.Path is bound to -cache, syntax-validated, and has its directory created at startup, yet its value never reaches any cache constructor. But its proposed_fix offers 'remove the field, its CLI flag, and its validation/directory-creation code', and THAT OPTION BREAKS PRODUCTION: Dockerfile.harvest:39 passes --cache to the deployed entrypoint, and flag.ContinueOnError at loader.go:239 turns an unknown flag into log.Fatalf. Acceptance criteria MUST require that any removal edits Dockerfile.harvest:39 in the same commit, and MUST state that removing the flag registration alone fails the deployed container at startup with `flag provided but not defined: -cache`. Do NOT merge this with DEADGO-001 (refuted) — they share one wrong premise, and clustering them would launder the error rather than cancel it.

## Claim

Config.Cache.Path is validated and directory-created at startup but its value is never passed to any cache backend — the field is fully wired plumbing to nowhere

**Subject:** `{'kind': 'config', 'identity': 'config:Config.Cache.Path'}`

### Evidence

- Config.Cache.Path (types.go:17) is bound to the -cache CLI flag, syntax-validated in Validate(), and has its directory created in EnsureDirectories() — but is never read by any cache constructor.
  - Reproduce: `git grep -n "Path" -- '*.go' | grep -v internal/platform/config/ | grep -v _test.go`
- The only place a file-based cache is actually constructed reads its path from a different type (cache.SimpleCacheConfig.FilePath), not from config.Config.Cache.Path.
  - Reproduce: `git grep -n "NewFileCacheBackend" -- '*.go'`
- cfg.Cache.Path is referenced only inside internal/platform/config itself (validation.go:41,92 and loader.go:253) — no caller outside that package ever reads it.
  - Reproduce: `git grep -n "cfg\.Cache" -- '*.go'`

### Proposed fix

Either wire cfg.Cache.Path into the actual cache backend construction path in cmd/slabledger (so the -cache flag/CACHE_PATH env var does something), or remove the field, its CLI flag, and its validation/directory-creation code if the file-based cache is meant to be configured a different way.

### Blast radius

- `internal/platform/config/types.go`
- `internal/platform/config/loader.go`
- `internal/platform/config/validation.go`
- `cmd/slabledger (wherever cache construction happens)`

### Acceptance criteria

- [ ] Either `go build ./...` passes with Config.Cache.Path (and its flag/validation/EnsureDirectories logic) removed and no cache functionality regresses, OR the -cache flag's value is demonstrably threaded through to a live NewFileCacheBackend call and a test proves it.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-12 — Drop the dead marketmovers_config table

**Linear:** [SLA-20](https://linear.app/slabledger/issue/SLA-20/drop-the-dead-marketmovers-config-table)  
**Label:** Improvement · **Priority:** 3

<details><summary>Ticket body as posted</summary>

**Category:** Dead code · **Severity:** low · **Confidence:** mechanical · **Effort:** S
**Baseline revision:** `740976ec` · **Audit findings:** DBSCHEMA-004

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> Its sibling mm_* tables were dropped when Market Movers was removed in 000021; this one was missed. Zero Go references anywhere.

## Claim

marketmovers_config is dead schema: created in 000001, RLS-granted in 000003, but — unlike its sibling mm_* tables — never dropped when Market Movers was removed in 000021, and has zero Go references anywhere

**Subject:** `{'kind': 'table', 'identity': 'marketmovers_config'}`

**Verifier correction (already applied to this ticket):** Two defects, neither fatal to the central claim. (1) EVIDENCE TRANSCRIPTION, item 5: the recorded output for 'git grep -cE ...' is wrong in shape and in one value. git grep -c prints 'file:count' per file; the finding recorded bare integers that are file counts, and for cardladder_config it recorded '1' when the real output is 'internal/adapters/storage/postgres/cardladder_store.go:6' (one file, six matches). The command still runs and every table is still non-zero, so the conclusion drawn from it survives; the transcription does not. (2) IRREVERSIBILITY — the material gap. I agree with the controller's expectation: static analysis establishes that the schema OBJECT is dead, but it cannot establish that no production data of value lives in it, and that sub-question is unresolvable by this audit's method. The acceptance_criteria as written are unsafe for an irreversible operation: the proposed .down.sql recreates the table STRUCTURE only, so rollback does not restore rows, yet no criterion tells the fixer to check the live row count first. The ticket must add a precondition: 'SELECT count(*) FROM marketmovers_config returns 0 (or its contents are reviewed and explicitly discarded) before the DROP is authored.' Separately, the finding under-describes what is in the table: it holds username and encrypted_refresh_token (000001:516-517) for a retired vendor. That is a mild argument FOR dropping it (a stale credential sitting in production) rather than against, and does not raise the severity — 'low' is correct for an inert, undepended-upon table.

### Evidence

- marketmovers_config has zero references anywhere in tracked .go files (word-boundary, includes _test.go).
  - Reproduce: `git grep -nE '\bmarketmovers_config\b' -- '*.go'`
- db-map.json independently confirms external_refs=0.
  - Reproduce: `jq '.records[] | select(.identity=="table:marketmovers_config") | .external_refs' docs/audit/maps/db-map.json`
- 000021 (the migration that retired Market Movers) explicitly drops mm_sales_comps and mm_card_mappings by name but does not mention marketmovers_config at all — it was left behind.
  - Reproduce: `cat internal/adapters/storage/postgres/migrations/000021_drop_market_movers.up.sql`
- No later migration (000022-000025) references marketmovers_config either.
  - Reproduce: `grep -rl 'marketmovers_config' internal/adapters/storage/postgres/migrations/000022*.sql internal/adapters/storage/postgres/migrations/000023*.sql internal/adapters/storage/postgres/migrations/000024*.sql internal/adapters/storage/postgres/migrations/000025*.sql`
- The table's 000001 comment ('marketmovers_config: sqlite.MarketMoversConfig [marketmovers_store.go:17]') is one of five identical-form stale sqlite provenance comments in 000001; the other four (cardladder_config, cl_card_mappings, scheduler_run_stats — all live — and mm_card_mappings — dropped by 000021) were individually checked and are NOT additional dead-schema findings: three are actively used by Go code and one no longer exists as a table at all. Only marketmovers_config is both present in the schema and unreferenced.
  - Reproduce: `for t in cardladder_config cl_card_mappings scheduler_run_stats mm_card_mappings; do echo "== $t =="; git grep -cE "\\b$t\\b" -- '*.go'; done`

### Proposed fix

Add a new migration dropping marketmovers_config (DROP TABLE IF EXISTS marketmovers_config;), mirroring how 000021 dropped its siblings. Also remove its entry from docs/SCHEMA.md (see DBSCHEMA-006).

### Blast radius

- `internal/adapters/storage/postgres/migrations/ (new migration file)`
- 
```
docs/SCHEMA.md (### `marketmovers_config` section)
```

### Acceptance criteria

- [ ] A new migration's .up.sql contains 'DROP TABLE IF EXISTS marketmovers_config;' and its .down.sql recreates the table (matching the original 000001 definition) for rollback safety.
- [ ] go test ./... passes after migrations apply against a local Postgres.
- [ ] docs/SCHEMA.md no longer documents marketmovers_config as a live table.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-13 — Remove seven unreferenced React components and eleven unreferenced React Query hooks

**Linear:** [SLA-21](https://linear.app/slabledger/issue/SLA-21/remove-seven-unreferenced-react-components-and-eleven-unreferenced)  
**Label:** Improvement · **Priority:** 3

<details><summary>Ticket body as posted</summary>

**Category:** Dead code · **Severity:** medium · **Confidence:** strong · **Effort:** M
**Baseline revision:** `740976ec` · **Audit findings:** FE-002, FE-003

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> Same component trees, one PR. FE-003's verifier independently checked the factory-registration and test-only-usage escapes that the finding never claimed.

## FE-002 — Seven React components in campaign-detail/inventory and advisor trees have no import site anywhere in web/src

**Subject:** `{'kind': 'component', 'identity': 'CompSummaryPanel, QuickAddSection, ImportResultsDetail, SignalBadge, SignalChip (default exports); ScoreCardHeader, InventoryRow (named exports)'}`

### Evidence

- Five of the seven are default exports, so the map's word-boundary matching cannot see a renamed import; hand-checked import sites by file path for all five and found none
  - Reproduce: `for f in CompSummaryPanel QuickAddSection ImportResultsDetail SignalBadge SignalChip; do echo "=== $f ==="; git grep -n "from '.*$f'" web/src; done`
- The other two (ScoreCardHeader, InventoryRow) are named exports; word-boundary search over all of web/src finds only their own definition line
  - Reproduce: `for f in ScoreCardHeader InventoryRow; do echo "=== $f ==="; git grep -nE "\b$f\b" web/src; done`

### Proposed fix

Delete the 7 component files (and any co-located .module.css / test file that exists only for them) after confirming with git log / PR history that they are leftovers from a removed or superseded UI flow, not a component mid-integration.

### Blast radius

- `web/src/react/pages/campaign-detail/inventory/CompSummaryPanel.tsx`
- `web/src/react/pages/campaign-detail/QuickAddSection.tsx`
- `web/src/react/pages/campaigns/ImportResultsDetail.tsx`
- `web/src/react/pages/campaign-detail/inventory/SignalBadge.tsx`
- `web/src/react/pages/campaign-detail/inventory/SignalChip.tsx`
- `web/src/react/components/advisor/ScoreCardHeader.tsx`
- `web/src/react/components/inventory/InventoryRow.tsx`

### Acceptance criteria

- [ ] npm run build succeeds after deleting the 7 files
- [ ] git grep for each component name returns no hits in web/src

## FE-003 — Eleven React Query hooks in useCampaignQueries.ts / useAdminQueries.ts have no caller

**Subject:** `{'kind': 'hook', 'identity': 'useInventory, useCampaignPNL, usePortfolioChannelVelocity, usePortfolioInsights, useCreateBulkSales, useCreatePurchase, useCreateSale, useDeletePurchase, useImportPSA, useReassignPurchase (web/src/react/queries/useCampaignQueries.ts); useUnmatchDH (web/src/react/queries/useAdminQueries.ts)'}`

### Evidence

- Word-boundary search over web/src finds each hook name only at its own declaration (or, for useCampaignPNL, one incidental mention inside another hook's JSDoc comment)
  - Reproduce: `for f in useInventory useCampaignPNL usePortfolioChannelVelocity usePortfolioInsights useCreateBulkSales useCreatePurchase useCreateSale useDeletePurchase useImportPSA useReassignPurchase useUnmatchDH; do echo "=== $f ==="; git grep -nE "\b$f\b" web/src; done`

### Proposed fix

Remove the 11 unused hooks, or verify with the feature owner whether they are staged for an in-progress UI (campaign P&L view, portfolio velocity/insights widgets, bulk-sale/purchase-edit/DH-unmatch actions) that has not yet been wired to a page.

### Blast radius

- `web/src/react/queries/useCampaignQueries.ts`
- `web/src/react/queries/useAdminQueries.ts`

### Acceptance criteria

- [ ] npm run build and npm test pass in web/ with the 11 hooks removed
- [ ] grep confirms no remaining reference to the removed hook names in web/src

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-14 — Remove unused frontend API-client methods, pricing types, and utility exports

**Linear:** [SLA-22](https://linear.app/slabledger/issue/SLA-22/remove-unused-frontend-api-client-methods-pricing-types-and-utility)  
**Label:** Improvement · **Priority:** 3

<details><summary>Ticket body as posted</summary>

**Category:** Dead code · **Severity:** medium · **Confidence:** strong · **Effort:** M
**Baseline revision:** `740976ec` · **Audit findings:** FE-001, FE-004, FE-006

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> The five pricing.ts interfaces describe a per-grade eBay/estimate pricing API that no longer exists — they are the type-level residue of the removed pricing sources.

## FE-001 — Five API-client prototype methods have zero callers in web/src

**Subject:** `{'kind': 'symbol', 'identity': 'APIClient.deleteCampaign, .getCampaignSuggestions, .globalImportExternal, .listRevocationFlags, .syncPSASheets'}`

### Evidence

- frontend-reference-map.json reports external_refs==0 for these 5 api-client-method records
  - Reproduce: `jq -r '.records[] | select(.kind=="api-client-method" and .external_refs==0) | .identity' docs/audit/maps/frontend-reference-map.json`
- Each method has no caller anywhere in web/src outside its own declaration/prototype-assignment/test-mock-table lines
  - `web/src/js/api/campaigns.ts:53; web/src/js/api/campaignAnalytics.ts:80,88; web/src/js/api/campaignImports.ts:63,68`
  - Reproduce: `for m in deleteCampaign getCampaignSuggestions globalImportExternal listRevocationFlags syncPSASheets; do echo "=== $m ==="; git grep -nE "\b$m\b" web/src | grep -v 'web/src/js/api/'; done`

### Proposed fix

Remove the 5 unused methods (and their interface declarations) from campaigns.ts, campaignAnalytics.ts and campaignImports.ts, or confirm with the backend owner whether the underlying endpoints (DELETE /campaigns/:id, campaign suggestions, external bulk import, revocation-flag listing, PSA sheet sync) are still reachable from any UI path before deleting.

### Blast radius

- `web/src/js/api/campaigns.ts`
- `web/src/js/api/campaignAnalytics.ts`
- `web/src/js/api/campaignImports.ts`
- `web/src/js/api/campaigns.test.ts (mock-table entry exercising deleteCampaign)`

### Acceptance criteria

- [ ] npm run build and npm test pass in web/ with the 5 methods and their interface declarations removed
- [ ] grep confirms no remaining reference to the removed method names in web/src

## FE-004 — Ten unused utility exports across campaignConstants.ts, formatters.ts, sellSheetHelpers.tsx, and inventory/utils.ts

**Subject:** `{'kind': 'symbol', 'identity': 'normalizeChannel, toTitleCase, formatCardName, gradeDisplay, cardSubtitle, marginCode, marketTooltip, statusBorderColor, ProgressBar, saleChannelColors'}`

### Evidence

- Each of the 10 names has exactly one word-boundary hit repo-wide in web/src — its own export line — with no caller anywhere, including its own file
  - Reproduce: `for f in normalizeChannel toTitleCase formatCardName gradeDisplay cardSubtitle marginCode marketTooltip statusBorderColor ProgressBar saleChannelColors; do n=$(git grep -cE "\b$f\b" web/src | awk -F: '{s+=$2} END {print s+0}'); echo "$f total_hits=$n"; done`
- sellSheetHelpers.tsx is a mixed file: 4 of its exports (formatCardName, gradeDisplay, cardSubtitle, marginCode) are dead while checkHotSeller, clPriceDisplayCents, formatLastSaleDate, dollars are live imports elsewhere — confirming this is per-symbol dead code, not a whole orphaned file
  - Reproduce: `git grep -n "from '.*sellSheetHelpers'" web/src`

### Proposed fix

Remove the 10 dead exports. Downgrade any that are demonstrably reserved for a near-term feature (e.g. saleChannelColors for a planned channel-color legend) to a code comment rather than a live export.

### Blast radius

- `web/src/react/utils/campaignConstants.ts`
- `web/src/react/utils/formatters.ts`
- `web/src/react/utils/sellSheetHelpers.tsx`
- `web/src/react/pages/campaign-detail/inventory/utils.ts`

### Acceptance criteria

- [ ] npm run build and npm test pass in web/ with the 10 symbols removed
- [ ] grep confirms no remaining reference to the removed names in web/src

## FE-006 — Five type interfaces in pricing.ts describe a per-grade eBay/estimate pricing API that no longer exists

**Subject:** `{'kind': 'type', 'identity': 'EbayGradeData, EstimateGradeData, GradeData, MarketOverview, SalesVelocity (web/src/types/pricing.ts)'}`

### Evidence

- frontend-reference-map.json reports external_refs==0 for 5 of the 7 types in pricing.ts; only GradeKey and PriceHint are live
  - Reproduce: `jq -r '.records[] | select(.defined_at | test("types/pricing.ts")) | "\(.identity)\t\(.external_refs)"' docs/audit/maps/frontend-reference-map.json`
- Confirmed these 5 types are not merely internal-only (used elsewhere in the same file) — they appear nowhere outside their own declaration in pricing.ts
  - Reproduce: `git grep -n "EbayGradeData\|GradeData\b\|MarketOverview\|SalesVelocity" web/src | grep -v 'types/pricing.ts'`
- No Go handler in internal/adapters/httpserver builds a response using the field names these types describe (price, salesCount, avg7day, volume7day) or references the closest-named Go types (pricing.GradeDetail / EbayGradeDetail / EstimateGradeDetail), and no route named price-estimate exists
  - Reproduce: `grep -rln "GradeDetail\|EbayGradeDetail\|EstimateGradeDetail" internal/adapters/httpserver/*.go 2>/dev/null; git grep -n "price-estimate\|priceEstimate" internal/adapters/httpserver/routes.go web/src/js/api/*.ts`

### Proposed fix

Delete the 5 unused interfaces from pricing.ts. They likely describe a per-grade multi-source pricing view tied to one of the pricing sources CLAUDE.md records as removed on 2026-04-06 (PriceCharting, CardHedger, JustTCG, fusion engine) — confirm with git blame before deleting in case a UI for this is mid-build rather than post-removal debris.

### Blast radius

- `web/src/types/pricing.ts`

### Acceptance criteria

- [ ] npm run build passes with the 5 interfaces removed from pricing.ts
- [ ] grep confirms no remaining reference to the removed type names in web/src

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-15 — Remove UserPreferencesProvider, or wire its hook to a consumer

**Linear:** [SLA-23](https://linear.app/slabledger/issue/SLA-23/remove-userpreferencesprovider-or-wire-its-hook-to-a-consumer)  
**Label:** Improvement · **Priority:** 3

<details><summary>Ticket body as posted</summary>

**Category:** Dead code · **Severity:** medium · **Confidence:** strong · **Effort:** S
**Baseline revision:** `740976ec` · **Audit findings:** FE-005

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> Mounted app-wide but useUserPreferences() is never called, so the whole context has no consumer. Decide removal vs. wiring before writing code — the provider being mounted suggests intent that was never finished.

## Claim

UserPreferencesProvider is mounted app-wide but its own useUserPreferences() hook is never called — the whole context has no consumer

**Subject:** `{'kind': 'hook', 'identity': 'useUserPreferences (web/src/react/contexts/UserPreferencesContext.tsx:157) and its associated types UserPreferencesContextValue, UserPreferences, RecentCard, SavedFilterPreferences, ViewMode'}`

### Evidence

- UserPreferencesProvider is wired into both entry points (main.tsx and App.tsx), so this is not simply an orphaned file — the context genuinely renders in production
  - Reproduce: `git grep -n "UserPreferencesProvider" web/src`
- But useUserPreferences (the only way to read the context's value) has zero callers anywhere, and neither does the default-exported raw context object
  - Reproduce: `git grep -nE "\buseUserPreferences\b" web/src; echo ---; git grep -n "from '.*UserPreferencesContext'" web/src`

### Proposed fix

Either wire a real consumer (the JSDoc at line 152 documents an intended addRecentPriceCheck/preferences usage that was apparently never finished) or remove the entire feature (Provider, hook, and its 4 backing types) if it was superseded by something else. Left as-is, the Provider still runs its state/localStorage-sync logic on every render for a value nothing reads.

### Blast radius

- `web/src/react/contexts/UserPreferencesContext.tsx`
- `web/src/main.tsx`
- `web/src/react/App.tsx`

### Acceptance criteria

- [ ] Either: a real call site for useUserPreferences exists and npm test covers it, or: the Provider, hook, and unused types are removed and npm run build / npm test still pass

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-16 — Reconcile CLAUDE.md with the real architecture

**Linear:** [SLA-24](https://linear.app/slabledger/issue/SLA-24/reconcile-claudemd-with-the-real-architecture)  
**Label:** Improvement · **Priority:** 3

<details><summary>Ticket body as posted</summary>

**Category:** Documentation drift · **Severity:** high · **Confidence:** strong, suspected · **Effort:** M
**Baseline revision:** `740976ec` · **Audit findings:** DCT-001, DCT-002, DCT-003, DCT-004, DCT-019, NB-009

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> The single highest-leverage doc ticket: CLAUDE.md is what every future agent reads first, so each error here propagates. Six findings, all in one file — the false sole-price-source claim, package listings wrong in both directions, an env-var group omitting CardLadder, a named repository interface that has NEVER existed, and the one interface member that dwarfs the rest mislabelled 'focused'.

## DCT-001 — CLAUDE.md falsely claims DH is the sole price source; CardLadder is a fully-wired second pipeline, entirely unmentioned

**Subject:** `{'kind': 'claim', 'identity': 'CLAUDE.md:137 Pricing Pipeline section'}`

### Evidence

- CLAUDE.md:137 states 'DH (DoubleHolo) is the sole price source via DHPriceProvider'.
  - `CLAUDE.md:137`
  - Reproduce: `grep -n 'Pricing Pipeline' -A3 CLAUDE.md`
- CardLadder is a second, independently-wired price/comp source with its own client, scheduler, store, admin API, and config-driven enable flag.
  - Reproduce: `grep -n CardLadder cmd/slabledger/init_services.go; wc -l internal/adapters/scheduler/cardladder_refresh.go; grep -n 'admin/cardladder' internal/adapters/httpserver/routes.go; grep -n CARDLADDER_REFRESH_ENABLED internal/platform/config/loader.go`
- CardLadder values (clValueCents) feed real comp-price computation in the liquidation domain, independent of DHPriceProvider.
  - `internal/domain/liquidation/comp_price.go:9`
  - Reproduce: `sed -n '1,15p' internal/domain/liquidation/comp_price.go`
- The internal/domain/dhpricing package (service.go, types.go) also exists and is unmentioned anywhere in CLAUDE.md's package tree or pricing-pipeline narrative.
  - Reproduce: `git ls-files internal/domain/dhpricing/`

### Proposed fix

Rewrite the Pricing Pipeline section to describe both DH (DHPriceProvider) and CardLadder (client + scheduler + admin endpoints + liquidation comp pricing) as the two live price/valuation sources, or explicitly scope the 'sole price source' claim to a named subsystem (e.g. campaign market-signal pricing) if that narrower claim is what was intended.

### Blast radius

- `CLAUDE.md (every agent session in this repo loads this as project instructions)`

### Acceptance criteria

- [ ] CLAUDE.md's Pricing Pipeline section mentions CardLadder by name alongside DH, or the 'sole price source' claim is scoped precisely enough that it no longer contradicts the CardLadder client/scheduler/routes/config that exist in the repo.
- [ ] A reviewer re-running the grep commands in this finding's evidence sees the CardLadder wiring accounted for in the doc text.

## DCT-002 — CLAUDE.md's internal/domain/ package listing is wrong in both directions: lists two removed packages, omits six live ones

**Subject:** `{'kind': 'claim', 'identity': 'CLAUDE.md:34-58 internal/domain/ package tree'}`

### Evidence

- CLAUDE.md lists 'cards/  # CardRepository interface' and 'favorites/  # Favorites management' under internal/domain/, but neither package exists.
  - Reproduce: `go list ./internal/domain/... | grep -iE 'cards|favorites'`
- CLAUDE.md's domain package list omits six packages that currently exist under internal/domain/.
  - Reproduce: `go list ./internal/domain/... | sed 's#.*/domain/##' | sort`

### Proposed fix

Regenerate the domain package tree in CLAUDE.md from `go list ./internal/domain/...`, removing cards/ and favorites/, and adding demand/, dhevents/, dhpricing/, liquidation/, psacampaign/ with one-line descriptions each.

### Blast radius

- `CLAUDE.md`

### Acceptance criteria

- [ ] Every directory returned by `go list ./internal/domain/...` appears in CLAUDE.md's tree, and every entry in CLAUDE.md's tree corresponds to a real directory.

## DCT-003 — CLAUDE.md's adapters/clients/ tree lists a moved package and a removed package, and omits five that exist

**Subject:** `{'kind': 'claim', 'identity': 'CLAUDE.md:60-65 internal/adapters/clients/ tree'}`

**Verifier correction (already applied to this ticket):** The finding's second evidence command reports 'd555333b removed internal/adapters/clients/tcgdex' as the output of `git log --diff-filter=D --oneline -- internal/adapters/clients/tcgdex | head -3`. Running that exact command returns a different commit: `474072bd remove: reduce surface area — card search, watchlists, picks, orphaned tables (#152)`. Commit d555333b is real but is an unrelated dead-code-removal commit (#154) that does not touch tcgdex. This detail is fabricated/mismatched, but it is supporting color, not the actionable claim — the directory-tree drift itself (pricelookup/tcgdex listed but absent; 5 real dirs omitted) is independently and fully verified, and the acceptance criteria (`ls` output matching CLAUDE.md's tree) do not depend on the commit hash at all.

### Evidence

- CLAUDE.md:62 lists 'pricelookup/ # PriceLookup adapter (wraps PriceProvider for campaigns)' under adapters/clients/, but that adapter now lives at internal/domain/pricing/lookup/, and CLAUDE.md:63 lists 'tcgdex/' which was removed entirely.
  - Reproduce: `find internal/adapters/clients -maxdepth 1 -type d; git log --diff-filter=D --oneline -- internal/adapters/clients/tcgdex | head -3`
- adapters/clients/ contains five directories not listed in CLAUDE.md's tree.
  - Reproduce: `ls internal/adapters/clients/`

### Proposed fix

Update the adapters/clients/ tree in CLAUDE.md: drop pricelookup/ and tcgdex/, add cardladder/, dh/, dhlisting/, psa/, psaportal/ with one-line descriptions, and correct the PriceLookup adapter's real location (internal/domain/pricing/lookup/) elsewhere in the doc if it's referenced.

### Blast radius

- `CLAUDE.md`

### Acceptance criteria

- [ ] `ls internal/adapters/clients/` and CLAUDE.md's tree list the exact same set of directories.

## DCT-004 — CLAUDE.md's env-var 'Key groups' summary omits CardLadder entirely, consistent with the pricing-pipeline drift in DCT-001

**Subject:** `{'kind': 'claim', 'identity': 'CLAUDE.md:125-133 Environment Variables key groups'}`

### Evidence

- CLAUDE.md:125-133 lists DH, AI, Auth, and Schedulers groups (PRICE_REFRESH_ENABLED only) with no CardLadder group, despite CARDLADDER_REFRESH_ENABLED/CARDLADDER_REFRESH_HOUR/CL_SEARCH_URL existing in .env.example and being live-read config.
  - Reproduce: `grep -n -A10 '^## Environment Variables' CLAUDE.md; grep -n 'CARDLADDER\|CL_SEARCH_URL' .env.example; grep -n 'CARDLADDER_REFRESH_ENABLED' internal/platform/config/loader.go`

### Proposed fix

Add a CardLadder group to the Key groups list (CARDLADDER_REFRESH_ENABLED, CARDLADDER_REFRESH_HOUR).

### Blast radius

- `CLAUDE.md`

### Acceptance criteria

- [ ] CLAUDE.md's Environment Variables section lists a CardLadder group alongside DH/AI/Auth/Schedulers.

## DCT-019 — CLAUDE.md names a repository interface that has never existed, and mislabels the one member that dwarfs the rest as 'focused'

**Subject:** `{'kind': 'claim', 'identity': "CLAUDE.md:88 '8 focused repository interfaces'"}`

**Verifier correction (already applied to this ticket):** The blast_radius claim that this 'already misled an earlier pass of this audit's size-and-complexity lens into citing it as the violated convention' does not reproduce: docs/audit/findings/size-and-complexity.json's own PurchaseRepository finding correctly refers to 'seven sibling inventory repository interfaces' and never cites SnapshotRepository or CLAUDE.md's '8 focused' line as a violated convention. This is a narrative embellishment in blast_radius, not load-bearing for the core claim or its acceptance criteria, and doesn't affect ticketability.

### Evidence

- CLAUDE.md:88 lists eight names, one of which — SnapshotRepository — has zero occurrences anywhere else in the repository.
  - `CLAUDE.md:88`
  - Reproduce: `git grep -n "SnapshotRepository"`
- Only seven repository interfaces of this kind actually exist under internal/domain/inventory/.
  - Reproduce: `git grep -n "^type.*Repository interface" internal/domain/inventory/*.go`
- PurchaseRepository has 55 methods — more than the other six combined (46) — directly contradicting 'focused' as a description applied uniformly to all eight.
  - Reproduce: `for f in repository_analytics repository_campaign repository_dh repository_finance repository_pricing repository_purchase repository_sale; do echo -n "$f: "; sed -n '/^type.*Repository interface {/,/^}/p' internal/domain/inventory/$f.go | grep -cE '^\s*[A-Z][A-Za-z0-9]*\('; done`

### Proposed fix

Correct the sentence to name the seven interfaces that actually exist (drop SnapshotRepository), fix the count to 7 (or explicitly mention PendingItemRepository as an eighth, optional repository if that's the intended scope), and stop describing PurchaseRepository as 'focused' — either split it (see the size-and-complexity lens's related finding on PurchaseRepository's breadth) or describe it accurately as the exception.

### Blast radius

- `CLAUDE.md`
- `any agent using this sentence as the authority on inventory repository conventions (this already misled an earlier pass of this audit's size-and-complexity lens into citing it as the violated convention)`

### Acceptance criteria

- [ ] CLAUDE.md:88 lists only interface names that `git grep "^type.*Repository interface"` finds under internal/domain/inventory/.
- [ ] CLAUDE.md no longer describes PurchaseRepository as 'focused' without qualification, given its method count relative to its siblings.

## NB-009 — POINTER (docs-config-tests lens): the documented repository-interface roster names `SnapshotRepository`, which does not exist anywhere in the repo

**Subject:** `{'kind': 'claim', 'identity': "CLAUDE.md: inventory has '8 focused repository interfaces' including SnapshotRepository"}`

### Evidence

- SnapshotRepository is declared nowhere in the repository. Empty output is the evidence of absence.
  - Reproduce: `git grep -nE '\bSnapshotRepository\b' -- '*.go'`
- There are indeed eight *Repository interfaces in inventory, but the eighth is PendingItemRepository, not SnapshotRepository.
  - `internal/domain/inventory/pending_items.go:44`
  - Reproduce: `git ls-files 'internal/domain/inventory/*.go' | grep -v _test | xargs grep -hnE '^type [A-Za-z]+Repository interface' | sort -u`

### Proposed fix

Owned by the docs-config-tests lens. Correct the CLAUDE.md roster to the eight interfaces that exist.

### Blast radius

- `CLAUDE.md`

### Acceptance criteria

- [ ] Every interface named in the CLAUDE.md roster is found by git grep -nE '^type <Name> interface' -- 'internal/domain/inventory/*.go', and the roster count matches the grep count.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-17 — Reconcile docs/SCHEMA.md with the 25-migration Postgres history

**Linear:** [SLA-25](https://linear.app/slabledger/issue/SLA-25/reconcile-docsschemamd-with-the-25-migration-postgres-history)  
**Label:** Improvement · **Priority:** 3

<details><summary>Ticket body as posted</summary>

**Category:** Documentation drift · **Severity:** medium · **Confidence:** mechanical, strong · **Effort:** L
**Baseline revision:** `740976ec` · **Audit findings:** DBSCHEMA-005, DBSCHEMA-006, DBSCHEMA-007, DBSCHEMA-008, DCT-011

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> Seven live tables missing, three dropped tables documented as live, two wrong migration numbers, and pre-cutover SQLite migration numbers up to 000051 that have no corresponding file in this repo — making most 'Added: migration NNNNN' provenance notes unverifiable or actively misleading.

## DBSCHEMA-005 — docs/SCHEMA.md omits seven currently-live tables entirely

**Subject:** `{'kind': 'table', 'identity': 'docs/SCHEMA.md (missing tables)'}`

### Evidence

- Diffing db-map.json's 44 declared tables (minus the 5 that later migrations drop, see DBSCHEMA-004/007 context) against every '### `table_name`' heading in docs/SCHEMA.md shows 7 live tables with no heading at all.
  - Reproduce: 
```
jq -r '.records[] | select(.kind=="table") | .identity' docs/audit/maps/db-map.json | sed 's/^table://' | sort > /tmp/actual_tables.txt; grep -n '^### `' docs/SCHEMA.md | sed -E 's/.*`([a-z_0-9]+)`.*/\1/' | sort > /tmp/documented.txt; comm -23 /tmp/actual_tables.txt /tmp/documented.txt
```
- Of those 9 raw hits, 2 (mm_sales_comps, psa_exchange_policy) are correctly absent because both tables are dropped (000021, 000014 respectively) — their absence from the docs is correct, not drift. The remaining 7 are live tables with real Go callers and are genuinely undocumented.
  - Reproduce: `for t in card_price_trajectory dh_card_tombstones dh_comp_cache dh_state_events psa_portal_snapshot psa_portal_token scheduler_run_stats; do git grep -cE "\\b$t\\b" -- '*.go' | awk -v t=$t '{print t": "$0}'; done`

### Proposed fix

Add a `### table_name` section to docs/SCHEMA.md for each of card_price_trajectory, dh_card_tombstones, dh_comp_cache, dh_state_events, psa_portal_snapshot, psa_portal_token, and scheduler_run_stats, following the existing column/index/FK table format used throughout the doc.

### Blast radius

- `docs/SCHEMA.md`

### Acceptance criteria

- [ ] Each of the 7 tables listed has a `### table_name` section in docs/SCHEMA.md with a column table matching its CREATE TABLE definition in migrations.
- [ ] The diff command in this finding's evidence returns no output when re-run after the doc update.

## DBSCHEMA-006 — docs/SCHEMA.md still documents three tables as live that have been dropped by migrations (advisor_cache, sell_sheet_items, mm_card_mappings)

**Subject:** `{'kind': 'table', 'identity': 'docs/SCHEMA.md (stale dropped-table sections)'}`

**Verifier correction (already applied to this ticket):** Nothing material. One transcription inaccuracy worth flagging so the controller does not mistake it for a reproduction failure: the finding quotes the sell_sheet_items drop as 'DROP TABLE IF EXISTS sell_sheet_items;' when the actual line is 'DROP TABLE IF EXISTS public.sell_sheet_items CASCADE;', and it strips the grep -n line-number prefixes from all three quoted outputs. The commands run and support the claims; only the pasted output is paraphrased.

### Evidence

- advisor_cache is documented at docs/SCHEMA.md:250 with a full column table, but was dropped from the schema in 000013.
  - Reproduce: `sed -n '250,251p' docs/SCHEMA.md; echo ---; grep -n 'DROP TABLE' internal/adapters/storage/postgres/migrations/000013_drop_advisor_cache.up.sql`
- sell_sheet_items is documented at docs/SCHEMA.md:817 as a live table, but was dropped in 000007; the doc's own body text ('Migrated to global in migration 000042') references a pre-cutover SQLite migration number that does not exist in this repo's 25 Postgres migrations, a symptom this section shares with DBSCHEMA-008's broader stale-numbering finding.
  - Reproduce: `sed -n '817,819p' docs/SCHEMA.md; echo ---; grep -n 'DROP TABLE' internal/adapters/storage/postgres/migrations/000007_drop_sell_sheet_items.up.sql`
- mm_card_mappings is documented at docs/SCHEMA.md:868 as a live table ('Added in migration 000045'), but was dropped in 000021 alongside mm_sales_comps.
  - Reproduce: `sed -n '868,869p' docs/SCHEMA.md; echo ---; grep -n 'mm_card_mappings' internal/adapters/storage/postgres/migrations/000021_drop_market_movers.up.sql`

### Proposed fix

Delete the `### advisor_cache`, `### sell_sheet_items`, and `### mm_card_mappings` sections from docs/SCHEMA.md, or replace each with a one-line 'Dropped in migration NNNNN' note consistent with how other removed tables (e.g. psa_exchange_policy, cardhedger-era tables) are already handled elsewhere in the doc, if such a convention exists.

### Blast radius

- `docs/SCHEMA.md`

### Acceptance criteria

- [ ] docs/SCHEMA.md no longer presents advisor_cache, sell_sheet_items, or mm_card_mappings as live tables with column definitions.
- [ ] Re-running the DBSCHEMA-006 evidence commands shows each section either removed or replaced with an explicit dropped-table note.

## DBSCHEMA-007 — docs/SCHEMA.md cites the wrong migration number for two current tables: psa_campaign_snapshot and psa_campaign_push_queue both say 'Added: migration 000017', but both are actually created in 000018

**Subject:** `{'kind': 'table', 'identity': 'docs/SCHEMA.md (wrong migration provenance)'}`

**Verifier correction (already applied to this ticket):** Nothing material. One cosmetic note for the fixer's acceptance criteria: the second criterion, "grep -n 'Added: migration 000017' docs/SCHEMA.md no longer matches either section", is vacuously satisfiable — that literal pattern already matches nothing today, because the file's text is '**Added:** migration 000017' with bold markers. The criterion should be written against the real text (e.g. grep -n '\\*\\*Added:\\*\\* migration 000017' docs/SCHEMA.md) so it can actually fail before the fix and pass after. This does not affect the verdict; the defect and the fix are both correct as stated.

### Evidence

- docs/SCHEMA.md's psa_campaign_snapshot section says 'Added: migration 000017'.
  - Reproduce: `grep -n 'Added: migration' docs/SCHEMA.md | grep -A0 -B1 psa_campaign || sed -n '584,585p' docs/SCHEMA.md`
- psa_campaign_snapshot and psa_campaign_push_queue are both actually created in 000018_psa_campaign_sync.up.sql, not 000017. 000017 instead creates the unrelated psa_portal_snapshot table.
  - Reproduce: `grep -n 'CREATE TABLE' internal/adapters/storage/postgres/migrations/000017_psa_portal_snapshot.up.sql internal/adapters/storage/postgres/migrations/000018_psa_campaign_sync.up.sql`
- docs/SCHEMA.md's psa_campaign_push_queue section repeats the same wrong '000017' citation.
  - Reproduce: `sed -n '612,614p' docs/SCHEMA.md`

### Proposed fix

Change both 'Added: migration 000017' citations in docs/SCHEMA.md to '000018' for psa_campaign_snapshot and psa_campaign_push_queue.

### Blast radius

- `docs/SCHEMA.md`

### Acceptance criteria

- [ ] Both sections read 'Added: migration 000018'.
- [ ] grep -n 'Added: migration 000017' docs/SCHEMA.md no longer matches either section.

## DBSCHEMA-008 — docs/SCHEMA.md still cites pre-cutover SQLite migration numbers (up to 000051) that have no corresponding file in this repo's 25-migration Postgres history, making most of its 'Added: migration NNNNN' provenance notes unverifiable or actively misleading

**Subject:** `{'kind': 'table', 'identity': 'docs/SCHEMA.md (stale pre-cutover migration numbering)'}`

**Verifier correction (already applied to this ticket):** The headline number is wrong and must be corrected before this is ticketed. The finding's title and first evidence item say SCHEMA.md cites numbers 'up to 000051'; the actual maximum is 000067. The finding's recorded output for `grep -oE 'migration 0000[0-9][0-9]' docs/SCHEMA.md | sort -u | tail -5` (000048/000049/000050/000051) does not reproduce — the real output is 000058/000059/000060/000066/000067. The error is conservative rather than inflating: the drift is larger than claimed, 26 orphaned citation values rather than the ~13 the stated ceiling implies. I am scoring this confirmed rather than refuted because I derived the underlying claim independently and it holds in the finding's favor, but the controller should treat the '000051' figure in the title as stale and replace it with 000067. Separately, the second evidence command uses `ls ... | wc -l`, which PREAMBLE.md rule 3 disallows as evidence; `git ls-files` returns the same 25, so the conclusion is unaffected.

### Evidence

- docs/SCHEMA.md cites migration numbers up to 000051, but the repo's Postgres migration set (post-SQLite-cutover) only goes up to 000025.
  - Reproduce: `grep -oE 'migration 0000[0-9][0-9]' docs/SCHEMA.md | sort -u | tail -5; echo ---; ls internal/adapters/storage/postgres/migrations/*.up.sql | wc -l`
- CLAUDE.md documents that 000001_initial_schema 'represents the final-state schema after cutover from SQLite' — i.e. the current 25 migrations are a fresh Postgres history, and any doc citation above 000025 necessarily refers to the old, now-collapsed SQLite migration sequence, not a file that exists in this repo.
  - Reproduce: `grep -n 'final-state schema after cutover' CLAUDE.md`
- The overlap is not merely historical trivia: some SCHEMA.md citations in the 000001-000025 range collide with real, different current migrations (see DBSCHEMA-007, where 'migration 000017' is cited for a table actually created by current migration 000018), so a reader cannot tell from the document alone which 'migration NNNNN' citations are pre-cutover artifacts and which refer to a real file in this repo.
  - Reproduce: `grep -c 'migration 0000' docs/SCHEMA.md`

### Proposed fix

Rewrite docs/SCHEMA.md's per-table provenance notes to cite only the current 25-migration Postgres history (internal/adapters/storage/postgres/migrations/000001..000025), removing or clearly marking as historical any citation above 000025. Tables entirely defined in 000001_initial_schema should say so rather than citing a pre-cutover number.

### Blast radius

- `docs/SCHEMA.md`

### Acceptance criteria

- [ ] grep -oE 'migration 0000[0-9][0-9]' docs/SCHEMA.md, when sorted, has no value greater than 000025.
- [ ] Every remaining 'Added: migration NNNNN' citation matches a real .up.sql filename prefix in internal/adapters/storage/postgres/migrations/.

## DCT-011 — docs/SCHEMA.md cites a migration number that doesn't exist and still documents three dropped tables/columns as if they were live

**Subject:** `{'kind': 'claim', 'identity': 'docs/SCHEMA.md price_history migration number and dropped-table sections'}`

**Verifier correction (already applied to this ticket):** The 'zero Go references remain outside one stale comment' sub-claim cites `git grep -ci marketmovers internal cmd | grep -v ':0'` as producing 'cmd/slabledger/init_services.go:4: 1 (a stale comment only)'. Running that exact command instead returns five migration files (000001, 000003, 000005 — all legitimate table-creation/rollback SQL, not stale references) and does NOT include cmd/slabledger/init_services.go, because that file's comment reads 'Market Movers' (two words, space-separated), which the single-word substring 'marketmovers' never matches. The underlying claim is still true — a broader search (`git grep -in 'Market Movers|MarketMovers' -- '*.go'` excluding tests) confirms init_services.go:4 is indeed the only non-migration Go reference, and `git grep -n 'mm_value_cents|mm_trend_pct|mm_sales_30d' -- '*.go'` returns nothing — but the specific command/output pair in the finding's evidence does not reproduce as written. This does not affect either acceptance criterion, both of which concern only docs/SCHEMA.md's migration-number citation and its documentation of dropped tables, which are independently solid.

### Evidence

- docs/SCHEMA.md:210,212 says price_history was dropped by migration 000038; the migrations directory only goes up to 000025.
  - Reproduce: `ls internal/adapters/storage/postgres/migrations/*.up.sql | wc -l; ls internal/adapters/storage/postgres/migrations/000038* 2>&1`
- docs/SCHEMA.md:250-267,985 fully documents the advisor_cache table (including its trigger/index) though migration 000013 dropped it.
  - Reproduce: `cat internal/adapters/storage/postgres/migrations/000013_drop_advisor_cache.up.sql`
- docs/SCHEMA.md:438,852,868,995-996 documents mm_value_cents, marketmovers_config, mm_card_mappings, and related indexes, all dropped by migration 000021; zero Go references remain outside one stale comment.
  - Reproduce: `cat internal/adapters/storage/postgres/migrations/000021_drop_market_movers.up.sql; git grep -ci marketmovers internal cmd | grep -v ':0'`

### Proposed fix

Fix the migration number cited for price_history's removal (or remove the citation if the real migration number is unknown at this revision), and delete or clearly mark as historical the advisor_cache and marketmovers table/column sections.

### Blast radius

- `docs/SCHEMA.md`

### Acceptance criteria

- [ ] docs/SCHEMA.md cites only migration numbers that exist in internal/adapters/storage/postgres/migrations/.
- [ ] docs/SCHEMA.md no longer documents advisor_cache, marketmovers_config, mm_card_mappings, mm_value_cents, or mm_sales_comps as live schema.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-18 — Reconcile docs/API.md with the live route table

**Linear:** [SLA-26](https://linear.app/slabledger/issue/SLA-26/reconcile-docsapimd-with-the-live-route-table)  
**Label:** Improvement · **Priority:** 3

<details><summary>Ticket body as posted</summary>

**Category:** Documentation drift · **Severity:** medium · **Confidence:** strong · **Effort:** L
**Baseline revision:** `740976ec` · **Audit findings:** DCT-012

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> Documents removed marketmovers/advisor/sell-sheet endpoints and is missing at least 13 live ones.

## Claim

docs/API.md documents removed marketmovers/advisor/sell-sheet endpoints and is missing at least 13 live endpoints

**Subject:** `{'kind': 'claim', 'identity': 'docs/API.md endpoint documentation'}`

### Evidence

- docs/API.md documents 5 marketmovers admin endpoints plus export-mm/refresh-mm purchase endpoints; none are registered.
  - Reproduce: `grep -ohE '"(GET|POST|PUT|PATCH|DELETE) [^"]+"' internal/adapters/httpserver/routes.go internal/adapters/httpserver/router.go | grep -i mm`
- docs/API.md documents GET/POST /api/advisor/cache/{id}, POST /api/advisor/refresh/{id}, POST /api/advisor/purchase-assessment; only /api/advisor/digest and /api/advisor/liquidation-analysis are registered.
  - Reproduce: `grep -n advisor internal/adapters/httpserver/routes.go`
- docs/API.md documents POST /api/campaigns/{id}/sell-sheet, POST /api/portfolio/sell-sheet, and a full /api/sell-sheet/items CRUD set; only GET /api/sell-sheet is registered.
  - Reproduce: `grep -n sell-sheet internal/adapters/httpserver/routes.go internal/adapters/httpserver/router.go`
- At least 13 live routes have no corresponding ### heading in docs/API.md.
  - Reproduce: 
```
comm -23 <(grep -ohE '"(GET|POST|PUT|PATCH|DELETE) [^"]+"' internal/adapters/httpserver/routes.go internal/adapters/httpserver/router.go | tr -d '"' | sed -E 's#\{[a-zA-Z_]+\}#{}#g' | sort -u) <(grep -oE '### `(GET|POST|PUT|PATCH|DELETE) [^`]+`' docs/API.md | sed -E 's/### `//; s/`//' | sed -E 's#\{[a-zA-Z_]+\}#{}#g' | sort -u)
```

### Proposed fix

Remove the marketmovers, extra advisor, and extra sell-sheet endpoint sections from docs/API.md; add sections for the 13+ live routes identified. A field-level pass (request/response shapes) was explicitly out of scope for this sweep and would likely surface more drift.

### Blast radius

- `docs/API.md`
- `API consumers relying on it as ground truth`

### Acceptance criteria

- [ ] Running the `comm -23` command in this finding's evidence against the regenerated docs/API.md returns no live routes missing from the doc.
- [ ] docs/API.md no longer documents any marketmovers, advisor cache/refresh/purchase-assessment, or extra sell-sheet endpoint that doesn't exist in routes.go/router.go.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-19 — Purge removed subsystems from the remaining documentation

**Linear:** [SLA-27](https://linear.app/slabledger/issue/SLA-27/purge-removed-subsystems-from-the-remaining-documentation)  
**Label:** Improvement · **Priority:** 3

<details><summary>Ticket body as posted</summary>

**Category:** Documentation drift · **Severity:** medium · **Confidence:** strong · **Effort:** M
**Baseline revision:** `740976ec` · **Audit findings:** DCT-006, DCT-007, DCT-008, DCT-009, DCT-010, DCT-020

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> internal/README.md, docs/ARCHITECTURE.md, docs/LLM_USAGE.md, docs/DH_INVENTORY.md, docs/USER_GUIDE.md and one package doc comment all still describe a SQLite storage layer, a removed 'picks' package and its 3-stage AI pipeline, a Favorites feature that does not exist in the product, and a pipeline function that exists nowhere in the repo.

## DCT-006 — internal/README.md still describes a sqlite/ storage adapter (removed) and its Large File Awareness table is stale in both directions

**Subject:** `{'kind': 'claim', 'identity': 'internal/README.md sqlite paths and Large File Awareness table'}`

### Evidence

- internal/README.md references internal/adapters/storage/sqlite/ in three places; no such directory exists — postgres/ is the only storage backend.
  - Reproduce: `grep -n sqlite internal/README.md; find internal/adapters/storage -maxdepth 1 -type d`
- The Large File Awareness table (internal/README.md:578-582) claims domain/arbitrage/service.go is 530 lines; it is now 217 (split since). It omits domain/portfolio/service.go (540 lines, over the 500-line budget) and adapters/scheduler/cardladder_refresh.go (525 lines, over budget), neither of which is listed.
  - Reproduce: `wc -l internal/domain/arbitrage/service.go internal/domain/portfolio/service.go internal/adapters/scheduler/cardladder_refresh.go`

### Proposed fix

Replace all storage/sqlite/ references with storage/postgres/, and regenerate the Large File Awareness table from current `wc -l` output, adding portfolio/service.go and cardladder_refresh.go.

### Blast radius

- `internal/README.md`

### Acceptance criteria

- [ ] internal/README.md contains no reference to a sqlite storage path.
- [ ] The Large File Awareness table's line counts match current `wc -l` output and includes every file currently over 500 lines.

## DCT-007 — docs/ARCHITECTURE.md documents a SQLite stack/storage layer and a removed 'picks' package, and still describes Favorites/cards domain packages

**Subject:** `{'kind': 'claim', 'identity': 'docs/ARCHITECTURE.md stack/diagram/domain-package claims'}`

### Evidence

- docs/ARCHITECTURE.md:7 states 'Stack: Go 1.26 | SQLite (WAL) | ...' and :18/:84 diagram/tree a SQLite storage adapter; the project runs on Postgres exclusively.
  - Reproduce: `grep -n 'SQLite' docs/ARCHITECTURE.md; grep -n jackc/pgx go.mod`
- docs/ARCHITECTURE.md:70 documents a 'picks/' domain package (AI-driven acquisition watchlist); it was removed.
  - Reproduce: `go list ./internal/domain/... | grep -i picks; git log --oneline --diff-filter=D -- internal/domain/picks | head -3`
- docs/ARCHITECTURE.md references Favorites domain/service/repository and cards/ CardRepository in several places; neither package exists.
  - Reproduce: `go list ./internal/domain/... | grep -iE 'favorites|cards'`

### Proposed fix

Replace the SQLite stack line, diagram box, and tree entry with Postgres; remove the picks/ package section; remove or clearly mark the Favorites/cards sections as removed.

### Blast radius

- `docs/ARCHITECTURE.md`

### Acceptance criteria

- [ ] docs/ARCHITECTURE.md contains no SQLite references and no mention of a picks/, favorites/, or cards/ domain package.

## DCT-008 — docs/LLM_USAGE.md documents a detailed 3-stage 'Picks' AI pipeline for a domain package that was removed

**Subject:** `{'kind': 'claim', 'identity': 'docs/LLM_USAGE.md:17,19,114-173 Picks pipeline'}`

### Evidence

- docs/LLM_USAGE.md:114-173 describes a 3-stage Picks pipeline in internal/domain/picks/ with a daily 03:00 UTC refresh; the package and its scheduler entry point do not exist.
  - Reproduce: `go list ./internal/domain/... | grep -i picks; git grep -n 'picks.NewService' internal cmd`

### Proposed fix

Remove the Picks pipeline section from docs/LLM_USAGE.md, or replace it with a description of whatever AI-driven feature (if any) superseded it.

### Blast radius

- `docs/LLM_USAGE.md`

### Acceptance criteria

- [ ] docs/LLM_USAGE.md contains no description of a Picks pipeline unless internal/domain/picks (or an equivalent) actually exists in the repo.

## DCT-009 — docs/DH_INVENTORY.md cites two repository file paths under a sqlite/ directory that doesn't exist

**Subject:** `{'kind': 'claim', 'identity': 'docs/DH_INVENTORY.md:199-200 file path table'}`

### Evidence

- docs/DH_INVENTORY.md:199-200 cites internal/adapters/storage/sqlite/purchases_repository_dh.go and .../card_id_mapping_repository.go; both files now live under postgres/, and no sqlite/ directory exists.
  - Reproduce: `find internal/adapters/storage -maxdepth 1 -type d; ls internal/adapters/storage/postgres | grep -i card_id_mapping`

### Proposed fix

Update both file paths in the table to internal/adapters/storage/postgres/.

### Blast radius

- `docs/DH_INVENTORY.md`

### Acceptance criteria

- [ ] The two file paths in docs/DH_INVENTORY.md's table resolve to real files at their cited locations.

## DCT-010 — docs/USER_GUIDE.md documents a dedicated Favorites feature/page that does not exist in the product

**Subject:** `{'kind': 'claim', 'identity': 'docs/USER_GUIDE.md:30,45,244-250 Favorites feature'}`

### Evidence

- docs/USER_GUIDE.md:30,45,244-250 instructs end users on a Favorites page (add/remove favorites); there is no /api/favorites* route and no favorites domain package.
  - Reproduce: `git grep -n favorites internal/adapters/httpserver/routes.go internal/adapters/httpserver/router.go; go list ./internal/domain/... | grep -i favorites`

### Proposed fix

Remove the Favorites sections from docs/USER_GUIDE.md, or restore the feature if it is intended to exist and was accidentally dropped (verify with product owner before assuming either direction).

### Blast radius

- `docs/USER_GUIDE.md`
- `end users following this guide`

### Acceptance criteria

- [ ] docs/USER_GUIDE.md does not instruct users to use a feature that has no backing route or domain package, unless the feature is restored.

## DCT-020 — A package doc comment names a pipeline function, resolvePSACategory, that does not exist anywhere in the repo

**Subject:** `{'kind': 'claim', 'identity': 'internal/platform/cardutil/normalize.go:15 resolvePSACategory'}`

### Evidence

- normalize.go:15's doc comment describes a normalization pipeline ending in 'resolvePSACategory'; the identifier has zero declarations or references anywhere else.
  - `internal/platform/cardutil/normalize.go:15`
  - Reproduce: `grep -n resolvePSACategory internal/platform/cardutil/normalize.go; git grep -n resolvePSACategory`

### Proposed fix

Either rename the doc comment's last pipeline step to the function that actually performs category resolution today, or remove the step if no equivalent exists (verify which by reading the current normalization call chain before editing).

### Blast radius

- `internal/platform/cardutil/normalize.go`

### Acceptance criteria

- [ ] Every function name in normalize.go's doc comment pipeline resolves to a real declared function in the package.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-20 — Reconcile .env.example with what loader.go actually reads

**Linear:** [SLA-28](https://linear.app/slabledger/issue/SLA-28/reconcile-envexample-with-what-loadergo-actually-reads)  
**Label:** Improvement · **Priority:** 3

<details><summary>Ticket body as posted</summary>

**Category:** Documentation drift · **Severity:** low · **Confidence:** strong, suspected · **Effort:** S
**Baseline revision:** `740976ec` · **Audit findings:** DCT-013, DCT-015

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> Two declared vars are read by zero Go code; four more knobs are undocumented or mis-documented. Report variable NAMES only — never values.

## DCT-013 — BASE_URL and MEDIA_DIR are declared in .env.example but read by zero Go code

**Subject:** `{'kind': 'env', 'identity': 'env:BASE_URL, env:MEDIA_DIR'}`

### Evidence

- BASE_URL (.env.example:64) has no Go reference anywhere in the module.
  - Reproduce: `git grep -nE '\bBASE_URL\b' -- '*.go'`
- MEDIA_DIR (.env.example:68) has no Go reference anywhere in the module.
  - Reproduce: `git grep -nE '\bMEDIA_DIR\b' -- '*.go'`

### Proposed fix

Remove BASE_URL and MEDIA_DIR from .env.example, or wire them into config.Config/loader.go if they were meant to configure something (verify intent before deleting, since env var presence alone doesn't rule out a planned-but-unimplemented feature).

### Blast radius

- `.env.example`

### Acceptance criteria

- [ ] `git grep -nE '\bBASE_URL\b|\bMEDIA_DIR\b' -- '*.go'` continues to return nothing after removal, and .env.example no longer lists either variable (or both are wired into config.go and the grep now returns hits).

## DCT-015 — Four config knobs are undocumented or mis-documented in .env.example relative to what loader.go actually reads

**Subject:** `{'kind': 'claim', 'identity': '.env.example vs loader.go config-doc mismatches'}`

### Evidence

- ADVISOR_MAX_TOOL_ROUNDS is read live by loader.go:173 but has no entry at all in .env.example.
  - Reproduce: `grep -n ADVISOR_MAX_TOOL_ROUNDS internal/platform/config/loader.go .env.example`
- CL_SEARCH_URL, SHUTDOWN_TIMEOUT_SECONDS, and AZURE_AI_TIMEOUT are live-read config but appear in .env.example only as commented-out example lines, understating that they are real, working overrides.
  - Reproduce: `grep -n '^#.*\(CL_SEARCH_URL\|SHUTDOWN_TIMEOUT_SECONDS\|AZURE_AI_TIMEOUT\)' .env.example`

### Proposed fix

Add ADVISOR_MAX_TOOL_ROUNDS as a live (uncommented) entry in .env.example, and uncomment CL_SEARCH_URL/SHUTDOWN_TIMEOUT_SECONDS/AZURE_AI_TIMEOUT or add a comment noting they have working defaults and don't need to be set.

### Blast radius

- `.env.example`

### Acceptance criteria

- [ ] Every env var read in loader.go with envBool/envInt/envString-family helpers has a corresponding non-commented entry in .env.example, or an explicit comment explaining why it's commented out.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-21 — Re-enable the scheduler tests blocked by a stale TODO

**Linear:** [SLA-29](https://linear.app/slabledger/issue/SLA-29/re-enable-the-scheduler-tests-blocked-by-a-stale-todo)  
**Label:** Improvement · **Priority:** 3

<details><summary>Ticket body as posted</summary>

**Category:** Test coverage · **Severity:** medium · **Confidence:** strong · **Effort:** M
**Baseline revision:** `740976ec` · **Audit findings:** DCT-016

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> The precondition the TODO waits on was satisfied in the same commit that added the TODO, four months ago.

## Claim

A 4-month-stale TODO blocks re-enabling scheduler tests for a precondition that was satisfied in the same commit that added the TODO

**Subject:** `{'kind': 'test', 'identity': 'internal/adapters/scheduler/price_refresh_test.go TODO(Task 4)'}`

### Evidence

- price_refresh_test.go:25-27 carries a TODO saying tests should be re-enabled/rewritten 'once refreshBatch is reimplemented as a purchase-driven refresh (stale_prices VIEW was dropped in migration 000038)'.
  - Reproduce: `sed -n '25,27p' internal/adapters/scheduler/price_refresh_test.go`
- The TODO and the refreshBatch rewrite it's waiting on were both introduced in the exact same commit, 20cfa6ee ('pricing pipeline simplification — DH-only', 2026-04-06) — refreshBatch already reads via pricing.RefreshCandidateProvider (candidates), i.e. it is already purchase-driven.
  - Reproduce: `git log -1 --format='%H %ad %s' --date=short -L 25,27:internal/adapters/scheduler/price_refresh_test.go; git show 20cfa6ee --stat | grep price_refresh; grep -n 'candidates.GetRefreshCandidates\|pricing.RefreshCandidateProvider' internal/adapters/scheduler/price_refresh.go`
- No 000038 migration exists at this revision (only up to 000025), further confirming the comment's premise no longer describes reality.
  - Reproduce: `ls internal/adapters/storage/postgres/migrations/000038* 2>&1`
- The current test file has no test that exercises refreshBatch's actual logic (candidate batching, rate-limit/budget checks, provider-blocked handling) — only Start/Stop/Health/DefaultConfig are tested.
  - Reproduce: `grep -n '^func Test' internal/adapters/scheduler/price_refresh_test.go`

### Proposed fix

Write direct tests for refreshBatch covering: candidate-fetch error handling and consecutive-failure counting, empty-candidates no-op, provider-blocked skip, and hourly-budget-exhausted skip — then delete the stale TODO comment.

### Blast radius

- `internal/adapters/scheduler/price_refresh.go`
- `internal/adapters/scheduler/price_refresh_test.go`

### Acceptance criteria

- [ ] price_refresh_test.go contains at least one test that calls refreshBatch directly and asserts on its rate-limit/budget/provider-blocked branches.
- [ ] The TODO(Task 4) comment referencing migration 000038 and the pre-rewrite refreshBatch is removed.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-22 — Add test coverage for the dhlisting adapter package

**Linear:** [SLA-30](https://linear.app/slabledger/issue/SLA-30/add-test-coverage-for-the-dhlisting-adapter-package)  
**Label:** Improvement · **Priority:** 3

<details><summary>Ticket body as posted</summary>

**Category:** Test coverage · **Severity:** medium · **Confidence:** strong · **Effort:** M
**Baseline revision:** `740976ec` · **Audit findings:** DCT-017

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> PSA import batch translation, inventory sold-transitions, and PSA key-rotation delegation currently have zero tests.

## Claim

The dhlisting adapter package — PSA import batch translation, inventory sold-transitions, and PSA key-rotation delegation — has zero test coverage

**Subject:** `{'kind': 'package', 'identity': 'internal/adapters/clients/dhlisting'}`

### Evidence

- internal/adapters/clients/dhlisting/ contains one 283-line source file and no test file.
  - Reproduce: `git ls-files internal/adapters/clients/dhlisting/`
- The package contains non-trivial, money- and inventory-state-adjacent logic: PSA import batch-rejection error surfacing (422 handling), and MarkInventorySold/UpdateInventoryStatus transitions with PSA-key-rotation delegation.
  - Reproduce: `grep -n 'func \|psa_import batch rejected\|MarkInventorySold' internal/adapters/clients/dhlisting/adapter.go`

### Proposed fix

Add table-driven tests for PSAImporterAdapter.PSAImport (success, batch-rejected-with-reason, batch-rejected-without-reason) and InventoryAdapter's status-transition and rotation-delegation paths, following the internal/testutil/mocks Fn-field pattern used elsewhere in the adapters layer.

### Blast radius

- `internal/adapters/clients/dhlisting/adapter.go`

### Acceptance criteria

- [ ] internal/adapters/clients/dhlisting/adapter_test.go exists and exercises the batch-rejection and sold-transition branches identified in this finding's evidence.
- [ ] `go test ./internal/adapters/clients/dhlisting/...` passes.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-23 — Move the tuning computation into the package named tuning

**Linear:** [SLA-31](https://linear.app/slabledger/issue/SLA-31/move-the-tuning-computation-into-the-package-named-tuning)  
**Label:** Improvement · **Priority:** 4

<details><summary>Ticket body as posted</summary>

**Category:** Naming & structure · **Severity:** medium · **Confidence:** strong · **Effort:** M
**Baseline revision:** `740976ec` · **Audit findings:** NB-002

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> All 902 LOC of tuning computation and every tuning type live in `inventory`, consumed by nothing but the 162-LOC `tuning` package. Related to NB-004: the flat-sibling rule is part of why the code ended up there.

## Claim

The package named `tuning` contains no tuning logic: all 902 LOC of tuning computation and every tuning type live in `inventory`, consumed by nothing but the 162-LOC `tuning` package

**Subject:** `{'kind': 'package', 'identity': 'internal/domain/tuning'}`

**Verifier correction (already applied to this ticket):** Three things. (1) UNDISCLOSED MOVE BLOCKER. tuning_types.go:4 declares GradePerformance, which is a parameter type of the inventory repository port itself: internal/domain/inventory/repository_analytics.go:11 declares `GetPerformanceByGrade(ctx, campaignID) ([]GradePerformance, error)`, implemented at internal/adapters/storage/postgres/analytics_store.go:144 and mocked in internal/testutil/mocks/inventory_analytics_repo.go:15. Moving tuning_types.go wholesale would force internal/domain/inventory to import internal/domain/tuning — a cycle, since tuning already imports inventory. The exclusions disclose PurchaseWithSale and TuningResponse but not GradePerformance, so the file-level move the proposed_fix describes cannot be executed as written; tuning_types.go must be split, not moved. (2) The 'naming' category and the headline 'the package named tuning contains no tuning logic' are not supported. Package tuning exposes GetCampaignTuning returning a tuning response — the name describes what the package is about accurately. The defect is cohesion/placement, not a misleading name; nothing here meets the bar of a name describing what the code no longer does or colliding with another concept. Recategorize as architecture. (3) blast_radius omits internal/adapters/scoring/provider.go, which imports internal/domain/tuning and dereferences tuningData.MarketAlignment at :77-78, so it takes an import change too. NOT a wire break: the tuning types are JSON-tagged and mirrored at web/src/types/campaigns/analytics.ts:197-203, but a package move preserves JSON tags, so the contract survives as long as no field or type is renamed — the fixer must be told to move only.

### Evidence

- internal/domain/tuning is a single 162-line file whose only exported surface is a Service interface with one method that returns an inventory type.
  - `internal/domain/tuning/service.go:14`
  - Reproduce: `git ls-files 'internal/domain/tuning/*.go' | grep -v _test | xargs wc -l && grep -nE '^(func|type)' internal/domain/tuning/service.go`
- The four tuning-named files inside internal/domain/inventory total 902 lines and hold every Compute* entry point plus all ten tuning types.
  - `internal/domain/inventory/tuning.go:62`
  - Reproduce: `wc -l internal/domain/inventory/tuning.go internal/domain/inventory/tuning_analytics.go internal/domain/inventory/tuning_stddev.go internal/domain/inventory/tuning_types.go | tail -1`
- Every one of those seven tuning entry points is referenced from exactly one file outside internal/domain/inventory, and it is internal/domain/tuning/service.go.
  - Reproduce: `git grep -lE '\b(ComputeRecommendations|ComputePriceTierPerformance|ComputeBuyThresholdAnalysis|ComputeROIStats|EnrichPriceTierStddev|ComputeCardPerformance|EnrichCardPerformance)\b' -- '*.go' ':!internal/domain/inventory'`
- The Phase 1 reference map independently records external_refs == 1 and name_ambiguous false for the tuning entry points.
  - Reproduce: `jq -r '.records[] | select(.identity=="github.com/guarzo/slabledger/internal/domain/inventory.ComputeRecommendations" or .identity=="github.com/guarzo/slabledger/internal/domain/inventory.ComputeBuyThresholdAnalysis" or .identity=="github.com/guarzo/slabledger/internal/domain/inventory.ComputeROIStats") | "\(.identity) refs=\(.external_refs) amb=\(.name_ambiguous // false)"' docs/audit/maps/go-reference-map.json`
- The cluster's only dependencies on the rest of inventory are five shared types, all of which stay behind: Campaign, Phase, Purchase, Sale (types_core.go), CampaignPNL/ChannelPNL/DailySpend (analytics_types.go) and MarketSnapshot (service.go).
  - Reproduce: `git grep -nE '\b(CampaignPNL|ChannelPNL|DailySpend|MarketSnapshot|Campaign|Phase|Purchase|Sale)\b' -- 'internal/domain/inventory/tuning*.go' | grep -v _test | wc -l`

### Proposed fix

Move tuning.go, tuning_analytics.go, tuning_stddev.go, tuning_types.go (and their tests) into internal/domain/tuning/, leaving only genuinely shared types in inventory. This follows the established flat-sibling pattern exactly: tuning already imports inventory, and no new sibling-to-sibling edge is created. Keep TuningResponse in inventory (or move it and update the handler import) — that is the one decision the mover must make consciously.

### Blast radius

- `internal/domain/inventory/tuning*.go`
- `internal/domain/tuning/service.go`
- `internal/testutil/mocks/tuning_service.go`
- `internal/adapters/httpserver/handlers (TuningResponse only)`

### Acceptance criteria

- [ ] go build ./... and go test -race ./... pass after the move.
- [ ] bash scripts/check-imports.sh passes: internal/domain/tuning imports only inventory and observability, and no other sibling imports tuning.
- [ ] git grep -lE '\\bCompute(Recommendations|PriceTierPerformance|BuyThresholdAnalysis|CardPerformance)\\b' -- 'internal/domain/inventory/*.go' returns empty.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-24 — Rename the two mis-named inventory files

**Linear:** [SLA-32](https://linear.app/slabledger/issue/SLA-32/rename-the-two-mis-named-inventory-files)  
**Label:** Improvement · **Priority:** 4

<details><summary>Ticket body as posted</summary>

**Category:** Naming & structure · **Severity:** medium · **Confidence:** strong · **Effort:** S
**Baseline revision:** `740976ec` · **Audit findings:** NB-005, NB-006

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> Pure renames plus content moves, one PR. analytics_types.go and types_analytics.go coexist under two opposite conventions, and the latter mostly holds capital/finance and portfolio-health types. service_advanced.go names an adjective, not a concept.

## NB-005 — `analytics_types.go` and `types_analytics.go` coexist in one package under two opposite naming conventions, and `types_analytics.go` mostly holds capital/finance and portfolio-health types rather than analytics types

**Subject:** `{'kind': 'symbol', 'identity': 'internal/domain/inventory/types_analytics.go'}`

### Evidence

- Both files exist side by side in the same package, and the package uses both the `<subject>_types.go` and the `types_<subject>.go` convention at once.
  - Reproduce: `git ls-files 'internal/domain/inventory/*types*.go' | tr '\n' ' '`
- types_analytics.go declares capital/finance types (CapitalRawData, CapitalSummary, InvoiceSellThrough, ComputeCapitalSummary), portfolio-health types (PortfolioHealth, CampaignHealth, ChannelVelocity, AlertLevel, RecoveryTrend) and a query-filter option type — not analytics types. It also contains a function, despite the name.
  - `internal/domain/inventory/types_analytics.go:53`
  - Reproduce: `grep -nE '^(func|type)' internal/domain/inventory/types_analytics.go`
- The consumers confirm the mismatch: the capital types are used by finance, the health types by portfolio and the advisor tooling. Neither consumer is an analytics path.
  - Reproduce: `git grep -lE '\b(CapitalSummary|ComputeCapitalSummary|PortfolioHealth|CampaignHealth)\b' -- '*.go' ':!internal/domain/inventory' ':!*_test.go' ':!internal/testutil/*'`

### Proposed fix

Pick one convention (the package majority is `<subject>_types.go`) and rename consistently. Then split types_analytics.go by its actual contents: capital_types.go (CapitalRawData, CapitalSummary, InvoiceSellThrough, ComputeCapitalSummary), health_types.go (PortfolioHealth, CampaignHealth, ChannelVelocity, AlertLevel, RecoveryTrend) and purchase_filter.go (PurchaseFilter, PurchaseFilterOpt, With*). These are file renames within one package — no import path changes and no behavior change.

### Blast radius

- `internal/domain/inventory/types_analytics.go`
- `internal/domain/inventory/analytics_types.go`

### Acceptance criteria

- [ ] go build ./... and go test -race ./... pass with no import-path edits anywhere outside internal/domain/inventory (the change is intra-package file renames).
- [ ] git ls-files 'internal/domain/inventory/*.go' shows exactly one of the two conventions in use.
- [ ] No file in internal/domain/inventory is named for a subject whose declarations it does not contain.

## NB-006 — `service_advanced.go` names an adjective, not a concept: it holds two unrelated methods (certificate lookup and quick-add purchase)

**Subject:** `{'kind': 'symbol', 'identity': 'internal/domain/inventory/service_advanced.go'}`

### Evidence

- The file's entire contents are two unrelated service methods. Nothing about either is 'advanced'.
  - `internal/domain/inventory/service_advanced.go:11`
  - Reproduce: `grep -nE '^func ' internal/domain/inventory/service_advanced.go`
- A cert-lookup home already exists in the package — service_cert_entry.go plus psa_resolver.go and cert_import_types.go — so LookupCert is separated from its concept by the filename alone.
  - Reproduce: `git ls-files 'internal/domain/inventory/*cert*.go' | grep -v _test | tr '\n' ' '`

### Proposed fix

Delete the file by relocating its two methods: LookupCert alongside the other cert-entry code, QuickAddPurchase alongside the purchase CRUD in service_crud.go. If service_cert_entry.go is already at its size budget, create service_cert_lookup.go rather than reintroducing an adjective-named file.

### Blast radius

- `internal/domain/inventory/service_advanced.go`
- `internal/domain/inventory/service_cert_entry.go`
- `internal/domain/inventory/service_crud.go`

### Acceptance criteria

- [ ] go build ./... and go test -race ./... pass.
- [ ] git ls-files 'internal/domain/inventory/service_advanced.go' returns empty.
- [ ] bash scripts/check-file-size.sh passes for every file that received code.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-25 — Move invoice projection from inventory to the finance package

**Linear:** [SLA-33](https://linear.app/slabledger/issue/SLA-33/move-invoice-projection-from-inventory-to-the-finance-package)  
**Label:** Improvement · **Priority:** 4

<details><summary>Ticket body as posted</summary>

**Category:** Naming & structure · **Severity:** low · **Confidence:** strong · **Effort:** S
**Baseline revision:** `740976ec` · **Audit findings:** NB-008

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

## Claim

Invoice projection — an unambiguously financial concept — sits in `inventory` with `internal/domain/finance` as its only consumer

**Subject:** `{'kind': 'symbol', 'identity': 'internal/domain/inventory/invoice_projection.go'}`

**Verifier correction (already applied to this ticket):** Evidence claim 2 ('InvoiceProjection itself has no consumer outside the file that declares it') is literally true but reads as stronger than it is: finance does consume the type, via `projection := inventory.ComputeInvoiceProjection(...)` at service.go:49, where type inference means the identifier never appears in the source text. The grep measures textual absence, not absence of use. The conclusion is unaffected — the consumer is finance either way, which is the package the finding proposes moving to.

### Evidence

- The file's whole surface is one type and one function, and the only non-inventory reference is internal/domain/finance.
  - `internal/domain/inventory/invoice_projection.go:30`
  - Reproduce: `git grep -n '\bComputeInvoiceProjection\b' -- '*.go'`
- InvoiceProjection itself has no consumer outside the file that declares it.
  - Reproduce: `git grep -n '\bInvoiceProjection\b' -- '*.go' ':!internal/domain/inventory/invoice_projection.go'`
- This is the same single-sibling-consumer shape as NB-002, at smaller scale: a 93-line computation parked in inventory for a 207-line finance package.
  - Reproduce: `wc -l internal/domain/inventory/invoice_projection.go && git ls-files 'internal/domain/finance/*.go' | grep -v _test | xargs wc -l | tail -1`

### Proposed fix

Move invoice_projection.go into internal/domain/finance/. finance already imports inventory, so no new sibling edge is created and the flat-sibling rule is unaffected. Bundle this with NB-002 as a single 'reunite each sibling with its own computation' change rather than shipping it alone.

### Blast radius

- `internal/domain/inventory/invoice_projection.go`
- `internal/domain/finance/service.go`
- `internal/domain/inventory/types_analytics.go (comment reference at line 63)`

### Acceptance criteria

- [ ] go build ./... and go test -race ./... pass after the move.
- [ ] bash scripts/check-imports.sh passes.
- [ ] git grep -n '\\bComputeInvoiceProjection\\b' -- 'internal/domain/inventory/*.go' returns only the stale comment at types_analytics.go:63, updated or removed.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-26 — Extract the campaign-suggestions cluster out of inventory

**Linear:** [SLA-34](https://linear.app/slabledger/issue/SLA-34/extract-the-campaign-suggestions-cluster-out-of-inventory)  
**Label:** Improvement · **Priority:** 4

<details><summary>Ticket body as posted</summary>

**Category:** Naming & structure · **Severity:** high · **Confidence:** strong · **Effort:** M
**Baseline revision:** `740976ec` · **Audit findings:** NB-003

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> 919 LOC with zero inbound references from the rest of `inventory` and exactly one consumer package — the cleanest seam the audit found.

## Claim

The 919-LOC campaign-suggestions cluster has zero inbound references from the rest of `inventory` and exactly one consumer package — a clean, isolated seam

**Subject:** `{'kind': 'package', 'identity': 'internal/domain/inventory (suggestion*.go cluster)'}`

### Evidence

- The cluster is four files totalling 919 production lines.
  - `internal/domain/inventory/suggestions.go:105`
  - Reproduce: `wc -l internal/domain/inventory/suggestions.go internal/domain/inventory/suggestion_rules.go internal/domain/inventory/suggestion_rules_optimization.go internal/domain/inventory/suggestion_types.go | tail -1`
- No other file in internal/domain/inventory references the cluster's entry points. Empty output is the evidence of absence.
  - Reproduce: `git grep -nE '\b(GenerateSuggestions|CampaignSuggestionParams|ExpectedMetrics)\b' -- 'internal/domain/inventory/*.go' ':!internal/domain/inventory/suggestion*'`
- The Phase 1 reference map records GenerateSuggestions with exactly two external reference sites, both inside internal/domain/portfolio.
  - Reproduce: `jq -r '.records[] | select(.identity|endswith("inventory.GenerateSuggestions")) | "refs=\(.external_refs) sites=\(.ref_sites)"' docs/audit/maps/go-reference-map.json`
- The cluster's outbound dependencies are five inventory types plus two functions — SubjectAxisMatches (exported, matching.go) and confidenceLabelWithAge (unexported, portfolio.go:415). The latter is the only mechanical blocker to a move.
  - `internal/domain/inventory/portfolio.go:415`
  - Reproduce: `git grep -nE '\bconfidenceLabelWithAge\b' -- 'internal/domain/inventory/*.go' | grep -v _test`

### Proposed fix

Move the four suggestion*.go files into internal/domain/portfolio/, their sole computational consumer. Do NOT create a new internal/domain/suggestions/ sibling: portfolio would then have to import it, which is precisely the sibling-to-sibling edge the flat-sibling rule forbids. The move requires exporting confidenceLabelWithAge (or relocating it alongside), and deciding whether SuggestionsResponse moves with the logic or stays in inventory as a shared API type.

### Blast radius

- `internal/domain/inventory/suggestion*.go`
- `internal/domain/inventory/suggestions_test.go`
- `internal/domain/portfolio/service.go`
- `internal/domain/portfolio/snapshot.go`
- `internal/adapters/httpserver/handlers/campaigns_finance.go`

### Acceptance criteria

- [ ] go build ./... and go test -race ./... pass after the move.
- [ ] bash scripts/check-imports.sh passes with no new sibling-to-sibling import.
- [ ] git ls-files 'internal/domain/inventory/suggestion*' returns empty.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-27 — Extract the CSV-import subsystem out of inventory

**Linear:** [SLA-35](https://linear.app/slabledger/issue/SLA-35/extract-the-csv-import-subsystem-out-of-inventory)  
**Label:** Improvement · **Priority:** 4

<details><summary>Ticket body as posted</summary>

**Category:** Naming & structure · **Severity:** medium · **Confidence:** strong · **Effort:** L
**Baseline revision:** `740976ec` · **Audit findings:** NB-004

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> 2,901 LOC, the largest seam that can leave under the existing flat-sibling pattern. `inventory` is 10,428 LOC largely because that rule makes it the only legal home for anything two siblings share — so this ticket and NB-001 are causally linked.

## Claim

`inventory` is 10,428 LOC because the flat-sibling rule makes it the only legal home for anything two siblings share; the CSV-import subsystem (2,901 LOC) is the largest seam that can leave under the existing pattern

**Subject:** `{'kind': 'package', 'identity': 'internal/domain/inventory (decomposition assessment)'}`

**Verifier correction (already applied to this ticket):** The title asserts causation — 'inventory is 10,428 LOC BECAUSE the flat-sibling rule makes it the only legal home for anything two siblings share' — and the finding's own largest exhibit contradicts it. The CSV-import subsystem is 2,901 LOC, 28% of the package, and it is not shared between siblings at all: evidence claim 3 shows its consumers are handlers, schedulers, clients and cmd/, i.e. adapters, none of which the flat-sibling rule constrains. The import subsystem sits in inventory for reasons unrelated to the stated cause, so the cause explains at most the smaller remainder. Evidence claim 4 shows the siblings are thin, which is consistent with the hypothesis but does not establish it — no counterfactual is offered. Separately: claim 2 says the import subsystem spans '19 files'; `git ls-files 'internal/domain/inventory/*.go' | grep -v _test | grep -cE '/(parse_|import_|service_import_|cert_import)'` returns 15. The 19 is the external-consumer count from claim 3, transposed. Scope: step 1 of the proposed fix is NB-002 and NB-003 restated, so Task 14 must cluster rather than count this separately; step 3 ('revisit the flat-sibling rule') has no acceptance criterion and is a design decision, not a ticket. Step 2 (extract internal/domain/csvimport/) is the only independently actionable and independently evidenced unit, and it is provable from acceptance criterion 2. Ticket step 2 only; severity for that unit is medium, not high.

### Evidence

- internal/domain/inventory is 10,428 production lines across 67 files.
  - Reproduce: `git ls-files 'internal/domain/inventory/*.go' | grep -v _test | xargs wc -l | tail -1`
- The import/parse subsystem is 2,901 of those lines — 28% of the package — across 19 files with a coherent single purpose.
  - `internal/domain/inventory/import_parsing.go:1`
  - Reproduce: `git ls-files 'internal/domain/inventory/*.go' | grep -v _test | grep -E '/(parse_|import_|service_import_|cert_import)' | xargs wc -l | tail -1`
- Unlike tuning and suggestions, the import subsystem has a genuine external public surface: its parser entry points and result types are referenced from 19 files outside internal/domain/inventory, spanning handlers, schedulers, the psaportal and dh clients, cmd/psa-harvest and the shared mocks. It is a subsystem with real consumers, not an isolated leaf.
  - `internal/adapters/httpserver/handlers/campaigns_imports.go:1`
  - Reproduce: `git grep -lE '\b(ParsePSAExportRows|ParseEbayOrderRows|ParseOrdersExportRows|ParseShopifyExportRows|PSAExportRow|PSAImportResult|OrdersImportResult)\b' -- '*.go' ':!internal/domain/inventory' | head -20`
- The structural cause is visible in the sibling packages themselves: each is a thin orchestrator over computation that had to be parked in inventory. tuning is 162 LOC over 902 LOC of inventory-resident tuning code; finance is 207 LOC and calls inventory.ComputeInvoiceProjection and inventory.ComputeCapitalSummary; portfolio calls inventory.ComputePortfolioInsights and inventory.GenerateSuggestions.
  - Reproduce: `for p in tuning finance export portfolio arbitrage; do printf '%-10s %s\n' "$p" "$(git ls-files "internal/domain/$p/*.go" | grep -v _test | xargs cat | wc -l)"; done`

### Proposed fix

Decompose in three ordered steps, largest structural win last. (1) Take the two clean leaves first — NB-002 (tuning, 902 LOC) and NB-003 (suggestions, 919 LOC); together they remove 17% of the package with no new sibling edges. (2) Move the CSV-import subsystem to a new internal/domain/csvimport/ sibling: it depends on inventory types (Campaign, Purchase, Sale) but nothing in inventory calls into it except service_interfaces.go, and adapters importing a domain package directly is already the norm. (3) Only then revisit the flat-sibling rule itself. The rule is what forces every shared concept into inventory; a two-tier variant — a leaf tier of shared types that all siblings may import, with the no-cross-imports rule applying only between the orchestrating siblings — would let the remaining shared computation leave inventory too. Step 3 is a deliberate architectural decision and should not be taken as a side effect of steps 1-2.

### Blast radius

- `internal/domain/inventory/parse_*.go`
- `internal/domain/inventory/import_*.go`
- `internal/domain/inventory/service_import_*.go`
- `internal/adapters/httpserver/handlers/campaigns_imports.go`
- `internal/adapters/scheduler/psa_sync.go`
- `internal/adapters/scheduler/dh_orders_poll.go`
- `internal/adapters/clients/psaportal/`
- `cmd/psa-harvest/`
- `internal/testutil/mocks/inventory_service_imports.go`
- `scripts/check-imports.sh`

### Acceptance criteria

- [ ] After each step independently: go build ./... and go test -race ./... pass, and bash scripts/check-imports.sh passes.
- [ ] After step 2, internal/domain/csvimport imports internal/domain/inventory and no other internal/domain sibling, and git grep -lE '\\bcsvimport\\.' -- 'internal/domain/inventory/*.go' returns empty (the dependency runs one way only).
- [ ] git ls-files 'internal/domain/inventory/*.go' | grep -v _test | xargs wc -l reports a total below 7,000 after steps 1 and 2.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-28 — Make the inventory Service interface segmentation buy an actual seam

**Linear:** [SLA-36](https://linear.app/slabledger/issue/SLA-36/make-the-inventory-service-interface-segmentation-buy-an-actual-seam)  
**Label:** Improvement · **Priority:** 4

<details><summary>Ticket body as posted</summary>

**Category:** Interface segregation & size · **Severity:** low · **Confidence:** strong · **Effort:** M
**Baseline revision:** `740976ec` · **Audit findings:** ARCH-004

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> ADJUDICATED — CLUSTER WITH THE NEXT UNIT, DO NOT MERGE. This is the consumer-facing union (internal/domain/inventory/service.go:175, 54 methods). Four of seven sub-interfaces have zero consumers, consumers wanting narrow dependencies declare their own, and the main handler still takes the union.

## Claim

The inventory Service interface-segregation split buys no seam: four of seven sub-interfaces have zero consumers, consumers that want narrow deps declare their own, and the main handler still takes the 54-method union

**Subject:** `{'kind': 'type', 'identity': 'internal/domain/inventory.Service and its seven sub-interfaces in service_interfaces.go'}`

**Verifier correction (already applied to this ticket):** Two defects, neither fatal. (1) The title and evidence item 5 say '54-method union'; the true figure is 55. The undercount comes from the counting command targeting service_interfaces.go alone, which cannot see Close() at service.go:184. Task 14 should use 55 — and should note that ARCH-004's corrected count now coincides numerically with SIZE-001's 55-method PurchaseRepository. That is a coincidence between a service interface and a repository interface on opposite sides of the port, not evidence of duplication; per ADJUDICATIONS.md these remain two findings, and I did not treat either as grounds against the other. (2) Acceptance criterion 2 is defective. 'No adapter declares a comment matching "subset of inventory.Service"' is a comment-text grep satisfiable by rewording two comments without changing a single dependency, and it also contradicts the finding's own fix branch (b), which explicitly accepts the union as the real contract and would leave both local subsets standing. A ticket should drop or restate that criterion. It is not fatal because criterion 1 — every interface in service_interfaces.go has at least one non-test consumer — is a genuine outcome-stated end state, mechanically checkable by one grep per name, and satisfiable by either fix branch, which together with the build/test and unmodified-handler-test gates is enough for a fixer to prove the change correct.

### Evidence

- The Service union documents the segregation as its purpose.
  - `internal/domain/inventory/service.go:170-186`
  - Reproduce: `sed -n '170,185p' internal/domain/inventory/service.go`
- The Phase 1 Go reference map records four of the seven sub-interfaces with zero external references, none flagged name_ambiguous.
  - Reproduce: `jq -r '.records[] | select(.identity|test("inventory.(CRUDService|DHService|CertLookupService|SnapshotService)$")) | "\(.identity) refs=\(.external_refs) amb=\(.name_ambiguous)"' docs/audit/maps/go-reference-map.json`
- Only three of the seven are used by real consumers, in three places total.
  - Reproduce: `git grep -nE '\binventory\.(AnalyticsService|ImportService|PricingService|CRUDService|DHService|CertLookupService|SnapshotService)\b' -- 'internal/**/*.go' 'cmd/**/*.go' | grep -v _test | grep -v testutil`
- Adapters that wanted a narrow dependency bypassed the published sub-interfaces and declared their own local subsets instead.
  - Reproduce: `git grep -nE 'subset of inventory\.' -- internal/adapters | grep -v _test`
- The concrete cost: consumers that take the union force a 546-line mock covering 54 methods.
  - Reproduce: `wc -l internal/testutil/mocks/inventory_service.go && grep -cE '^\t[A-Z][A-Za-z0-9]*\(' internal/domain/inventory/service_interfaces.go && git grep -nE 'inventory\.Service\b' -- internal/adapters | grep -v _test`

### Proposed fix

Either (a) split CampaignsHandler and CampaignToolExecutor to depend on the narrow interfaces they actually use and fold the two ad-hoc local subsets (PSASyncPurchaseCreator, SnapshotEnrichService) into the published set, or (b) accept that the union is the real contract, delete the four zero-consumer sub-interfaces, inline their methods into Service, and remove the ISP claim from the doc comment. Do not leave the split declared but unused.

### Blast radius

- `internal/domain/inventory/service.go`
- `internal/domain/inventory/service_interfaces.go`
- `internal/adapters/httpserver/handlers/campaigns.go`
- `internal/adapters/httpserver/router.go`
- `internal/adapters/advisortool/executor.go`
- `internal/testutil/mocks/inventory_service.go`

### Acceptance criteria

- [ ] Every interface declared in internal/domain/inventory/service_interfaces.go has at least one non-test consumer, verifiable with a single git grep per name.
- [ ] No adapter declares a comment matching 'subset of inventory.Service' — narrow needs are met by a published interface.
- [ ] `go build ./...`, `go test -race ./...`, and `make check` pass.
- [ ] No change to any HTTP response shape: the handler test suite passes unmodified.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-29 — Split the 55-method PurchaseRepository persistence port

**Linear:** [SLA-37](https://linear.app/slabledger/issue/SLA-37/split-the-55-method-purchaserepository-persistence-port)  
**Label:** Improvement · **Priority:** 4

<details><summary>Ticket body as posted</summary>

**Category:** Interface segregation & size · **Severity:** medium · **Confidence:** strong · **Effort:** L
**Baseline revision:** `740976ec` · **Audit findings:** SIZE-001

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> ADJUDICATED — the sibling of ARCH-004, deliberately kept separate. This is the PERSISTENCE PORT (internal/domain/inventory/repository_purchase.go:31), a different declaration in a different file on the other side of the boundary. 55 methods — more than the other six sibling repository interfaces combined (controller counted: Campaign 5, Sale 7, Pricing 8, Analytics 10, Finance 14, DH 2 = 46). Merging this with ARCH-004 yields a ticket no single PR can land. Shared theme: interface segregation was declared, never enforced, and drifted in both directions at once.

## Claim

PurchaseRepository interface has 55 methods — more than the other six sibling inventory repository interfaces combined

**Subject:** `{'kind': 'type', 'identity': 'internal/domain/inventory.PurchaseRepository'}`

**Verifier correction (already applied to this ticket):** The 'DH v2 fields' comment group is claimed as '~25 methods'; hand-counting the methods from the '// DH v2 fields' comment (repository_purchase.go:89) to the end of the interface (:156) gives 19, not ~25. This doesn't change the conclusion (19 is still the largest single group and still supports extracting a DH sub-interface) but the finding overstated the group size by roughly 30%. More materially: severity 'high' is inconsistent with the audit's own adjudicated sibling finding ARCH-004 (internal/domain/inventory/service.go:175, the 54-method consumer-facing Service interface, same theme, same file family, ruled by the controller to be a distinct-but-parallel defect) which was rated 'low'. Nothing in this finding's evidence shows the 55-method interface has caused a bug, an outage, a security issue, or even measurable developer friction beyond the scheduler already routing around it -- it is a structural/maintainability concern with a clear fix path, which argues for a severity in the same band as its sibling, not three tiers higher.

### Evidence

- PurchaseRepository declares 55 methods in one interface — more than the sum of the other six sibling *Repository interfaces declared alongside it in internal/domain/inventory/ (CampaignRepository 5, SaleRepository 7, AnalyticsRepository 10, FinanceRepository 14, PricingRepository 8, DHRepository 2 = 46 combined).
  - `internal/domain/inventory/repository_purchase.go:31-157`
  - Reproduce: `for f in repository_campaign.go repository_purchase.go repository_sale.go repository_analytics.go repository_finance.go repository_pricing.go repository_dh.go; do name=$(grep -oE '^type [A-Za-z]+Repository' internal/domain/inventory/$f | awk '{print $2}'); count=$(awk -v n="$name" '$0 ~ "^type "n" interface"{f=1;next} f && /^}/{exit} f && /^[[:space:]]+[A-Za-z]+\(/{c++} END{print c+0}' internal/domain/inventory/$f); echo "$name: $count"; done`
- The interface is internally organized by the author into ad-hoc comment groups (CRUD, list/count, cert lookups, field updates, price overrides, receipt tracking, ebay export, snapshot status, DH v2 fields, ListDHPriceDrift) rather than separate interfaces — the DH v2 fields group alone is ~25 methods.
  - `internal/domain/inventory/repository_purchase.go:32-89`
  - Reproduce: `grep -nE '^\t// [A-Z]' internal/domain/inventory/repository_purchase.go`
- The concrete Postgres implementation of this single interface is already split across 6 separate files by exactly these concerns — the seam already exists on the implementation side, just not on the interface side.
  - `internal/adapters/storage/postgres/purchase_dh_push_store.go, purchase_dh_query_store.go`
  - Reproduce: `grep -c "^func (ps \*PurchaseStore)" internal/adapters/storage/postgres/purchase_store.go internal/adapters/storage/postgres/purchase_cert_store.go internal/adapters/storage/postgres/purchase_dh_push_store.go internal/adapters/storage/postgres/purchase_dh_query_store.go internal/adapters/storage/postgres/purchase_price_store.go internal/adapters/storage/postgres/purchase_psa_store.go`
- Consumers already avoid depending on the full interface: the scheduler package independently declares its own narrow structural interfaces (DHFieldsUpdater, PurchaseByCertLookup) satisfied by the same *PurchaseStore, rather than taking PurchaseRepository directly — direct evidence that callers use disjoint subsets of the 55 methods.
  - `internal/adapters/scheduler/dh_inventory_poll.go:25,32`
  - Reproduce: `git grep -n 'type DHFieldsUpdater interface\|type PurchaseByCertLookup interface' internal/adapters/scheduler/dh_inventory_poll.go`

### Proposed fix

Split PurchaseRepository along the same lines the Postgres implementation already uses: extract a PurchaseDHRepository (or similarly named) interface for the ~25 DH v2 / push-pipeline methods (mirroring the existing internal/domain/inventory/repository_dh.go DHRepository naming), and consider a smaller PurchasePricingRepository for the price-override/AI-suggestion group. Keep CRUD + list/count + cert lookups as the remaining PurchaseRepository. Update internal/domain/inventory/service.go's `purchases PurchaseRepository` field to compose the split interfaces (embedding), and update internal/testutil/mocks/inventory_purchase_repo.go accordingly.

### Blast radius

- `internal/domain/inventory/repository_purchase.go`
- `internal/domain/inventory/service.go`
- `internal/adapters/storage/postgres/purchase_store.go (and sibling purchase_*_store.go files)`
- `internal/testutil/mocks/inventory_purchase_repo.go`
- `internal/testutil/mocks/inmemory_campaign_store.go (see SIZE-004)`

### Acceptance criteria

- [ ] go build ./... and go test -race ./... pass after the split.
- [ ] No single interface implemented by PurchaseStore exceeds ~20 methods.
- [ ] internal/domain/inventory/service.go compiles using the composed/embedded interfaces without behavior change.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-30 — Decompose the 286-line ListPurchases function

**Linear:** [SLA-38](https://linear.app/slabledger/issue/SLA-38/decompose-the-286-line-listpurchases-function)  
**Label:** Improvement · **Priority:** 4

<details><summary>Ticket body as posted</summary>

**Category:** Interface segregation & size · **Severity:** medium · **Confidence:** strong · **Effort:** M
**Baseline revision:** `740976ec` · **Audit findings:** SIZE-002

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

## Claim

ListPurchases is a 286-line function fusing batch aggregation with a large per-purchase state machine

**Subject:** `{'kind': 'symbol', 'identity': 'internal/domain/dhlisting.(*dhListingService).ListPurchases'}`

### Evidence

- ListPurchases spans 286 lines, in a 451-line file that is otherwise under the 500-line warn threshold.
  - `internal/domain/dhlisting/dh_listing_service.go:157`
  - Reproduce: `awk '/^func \(s \*dhListingService\) ListPurchases/{start=NR} start && /^}/{print NR-start+1; exit}' internal/domain/dhlisting/dh_listing_service.go`
- Within the single `for _, cn := range sortedCerts` loop the function performs: PSA-received eligibility gating, inline match+push, no-op detection, DH status update with three distinct error branches (stale inventory ID reset, PSA key exhaustion short-circuit, generic failure), channel sync with revert-on-failure, and local persistence + event recording — all inline rather than delegated to a helper.
  - `internal/domain/dhlisting/dh_listing_service.go:210-429`
  - Reproduce: `sed -n '210,429p' internal/domain/dhlisting/dh_listing_service.go | grep -c 'continue'`

### Proposed fix

Extract the per-purchase body (everything inside the `for _, cn := range sortedCerts` loop) into a helper method, e.g. `(s *dhListingService) listOnePurchase(ctx, p Purchase) (outcome listOutcome, err error)`, returning a small result enum (listed/synced/skipped/aborted) plus the failure reason. ListPurchases would then own only: the pause gate, batch lookup, iteration, and aggregation of the per-item outcomes — matching the shape of the existing pause-gate/aggregation code that already precedes and follows the loop.

### Blast radius

- `internal/domain/dhlisting/dh_listing_service.go`

### Acceptance criteria

- [ ] go test ./internal/domain/dhlisting/... passes unchanged after the extraction (existing table-driven tests should require no behavior changes, only possibly new unit tests for the extracted helper).
- [ ] ListPurchases itself drops to roughly the length of its pre-loop batch-lookup section plus a short iteration/aggregation loop.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-31 — Decompose the 312-line BuildGroup scheduler constructor

**Linear:** [SLA-39](https://linear.app/slabledger/issue/SLA-39/decompose-the-312-line-buildgroup-scheduler-constructor)  
**Label:** Improvement · **Priority:** 4

<details><summary>Ticket body as posted</summary>

**Category:** Interface segregation & size · **Severity:** medium · **Confidence:** strong · **Effort:** M
**Baseline revision:** `740976ec` · **Audit findings:** SIZE-003

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> ~20 independent, self-contained blocks that could each be its own function.

## Claim

BuildGroup is a 312-line function built from ~20 independent, self-contained scheduler-construction blocks that could each be its own function

**Subject:** `{'kind': 'symbol', 'identity': 'internal/adapters/scheduler.BuildGroup'}`

### Evidence

- BuildGroup spans 312 lines and is the single largest function in the repo outside cmd/ wiring.
  - `internal/adapters/scheduler/builder.go:125`
  - Reproduce: `awk '/^func BuildGroup/{start=NR} start && /^}/{print NR-start+1; exit}' internal/adapters/scheduler/builder.go`
- The body is a sequence of ~20 independent `if deps.X != nil { ... ; schedulers = append(schedulers, ...) }` blocks, each gated on its own disjoint subset of BuildDeps fields and each already introduced by its own doc comment naming the scheduler it builds (e.g. "Price refresh scheduler", "DH intelligence refresh scheduler", "Card Ladder value refresh scheduler").
  - `internal/adapters/scheduler/builder.go:146-425`
  - Reproduce: `grep -c 'schedulers = append(schedulers' internal/adapters/scheduler/builder.go`
- The BuildDeps struct that feeds BuildGroup already has 52 fields grouped by the same per-scheduler comment headers (DH dependencies, DH push dependencies, Card Ladder dependencies, PSA sync dependencies, etc.), mirroring the blocks in BuildGroup — the grouping the split would follow is already documented in the struct.
  - `internal/adapters/scheduler/builder.go:24-112`
  - Reproduce: `awk '/^type BuildDeps struct/,/^}/' internal/adapters/scheduler/builder.go | grep -cE '^\s+[A-Za-z]+\s+[A-Za-z*\[\]]'`

### Proposed fix

Extract each `if deps.X != nil { ... }` block into its own `buildXScheduler(cfg *config.Config, deps BuildDeps) Scheduler` (or equivalent, for the four blocks that also populate a named BuildResult field) helper function in builder.go, following the same one-concern-per-function shape already used for the CSV parsers (parse_psa.go / parse_mm.go / parse_shopify.go). BuildGroup itself would become a flat sequence of `if s := buildXScheduler(cfg, deps); s != nil { schedulers = append(schedulers, s) }` calls plus the four named-result assignments.

### Blast radius

- `internal/adapters/scheduler/builder.go`

### Acceptance criteria

- [ ] go build ./... and go test ./internal/adapters/scheduler/... pass unchanged.
- [ ] BuildGroup's body is reduced to calls into per-scheduler builder functions with no inline construction logic remaining.
- [ ] internal/README.md's 'Adding a new scheduler' recipe still describes an accurate single insertion point.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-32 — Bring inmemory_campaign_store.go under the file-size budget

**Linear:** [SLA-40](https://linear.app/slabledger/issue/SLA-40/bring-inmemory-campaign-storego-under-the-file-size-budget)  
**Label:** Improvement · **Priority:** 4

<details><summary>Ticket body as posted</summary>

**Category:** Interface segregation & size · **Severity:** low · **Confidence:** strong · **Effort:** M
**Baseline revision:** `740976ec` · **Audit findings:** SIZE-004

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> 1,328 lines / 106 methods, invisible to check-file-size.sh because testutil is excluded. The exclusion is arguably correct; the file size is not.

## Claim

inmemory_campaign_store.go is 1,328 lines / 106 methods, invisible to check-file-size.sh because testutil is excluded

**Subject:** `{'kind': 'package', 'identity': 'internal/testutil/mocks.InMemoryCampaignStore (internal/testutil/mocks/inmemory_campaign_store.go)'}`

### Evidence

- The file is 1,328 lines with 106 top-level functions, more than double the next-largest file in the repo (562 lines).
  - `internal/testutil/mocks/inmemory_campaign_store.go`
  - Reproduce: `wc -l internal/testutil/mocks/inmemory_campaign_store.go && grep -c '^func ' internal/testutil/mocks/inmemory_campaign_store.go`
- check-file-size.sh explicitly excludes the testutil path, so this file is never counted by `make check`.
  - `scripts/check-file-size.sh:20`
  - Reproduce: `grep -n testutil scripts/check-file-size.sh`
- The file already carries its own section-comment markers that exactly mirror the seven inventory repository interfaces it implements (CampaignRepository, PurchaseRepository, SaleRepository, AnalyticsRepository, FinanceRepository, PricingRepository, DHRepository), showing the split points are already known to the author.
  - `internal/testutil/mocks/inmemory_campaign_store.go:122-1295`
  - Reproduce: `grep -n '^// ---' internal/testutil/mocks/inmemory_campaign_store.go`

### Proposed fix

Split inmemory_campaign_store.go along its own existing section markers into per-interface files sharing the same InMemoryCampaignStore struct (e.g. inmemory_campaign_store.go for the struct + CampaignRepository methods, inmemory_purchase_store.go, inmemory_sale_store.go, inmemory_analytics_store.go, inmemory_finance_store.go, inmemory_pricing_store.go, inmemory_dh_store.go), matching the split already used for the production Postgres implementation (purchase_store.go / purchase_cert_store.go / purchase_dh_push_store.go / etc., see SIZE-001).

### Blast radius

- `internal/testutil/mocks/inmemory_campaign_store.go`

### Acceptance criteria

- [ ] go build ./... and go test ./... pass unchanged (pure file split, same package, no exported surface change).
- [ ] No resulting file in internal/testutil/mocks/ from this split exceeds ~250 lines.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-33 — Give the demand repository contract a shared Go type across the seam

**Linear:** [SLA-41](https://linear.app/slabledger/issue/SLA-41/give-the-demand-repository-contract-a-shared-go-type-across-the-seam)  
**Label:** Improvement · **Priority:** 4

<details><summary>Ticket body as posted</summary>

**Category:** Architecture · **Severity:** low · **Confidence:** strong · **Effort:** L
**Baseline revision:** `740976ec` · **Audit findings:** ARCH-002

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> Opaque vendor JSON blobs pass through the domain; writer and reader share no type, so the compiler enforces nothing across the boundary.

## Claim

The demand repository contract passes opaque vendor JSON blobs through the domain; writer and reader share no Go type, so the compiler enforces nothing across the seam

**Subject:** `{'kind': 'symbol', 'identity': 'internal/domain/demand.Repository — CardCache/CharacterCache JSON blob fields (DemandJSON, VelocityJSON, TrendJSON, SaturationJSON, PriceDistributionJSON)'}`

**Verifier correction (already applied to this ticket):** Two of the five evidence items do not support the claims attached to them. (1) Evidence item 4 says a storage-decoding failure mode 'has leaked all the way into the API response type ... returns the count as a response field.' It has not. demand.CampaignSignalsResponse is the domain SERVICE return type; it carries no JSON tags at all (internal/domain/demand/campaign_signals.go:28-44), and SkippedRows never reaches the wire — the HTTP body is built by toCampaignSignalsDTO into campaignSignalsResponseDTO, whose only fields are computed_at, data_quality and signals (internal/adapters/httpserver/handlers/campaign_signals_handler.go:46-50). The handler's sole use of SkippedRows is a Warn log at campaign_signals_handler.go:36-39. An observability counter on a service return value is defensible design, not a leak, and this item should carry no weight. (2) Evidence item 5 misreads the package doc. internal/domain/demand/types.go:1-6 promises that DH payloads 'are parsed here into domain-local structs so the SCORING LOGIC stays decoupled from the wire format' — and that is exactly what happens: velocityBlobJSON is a domain-local struct and the scoring path consumes signalEntry, not the blob. The doc makes no promise about the repository interface being wire-free, so citing it as an unmet claim is unfair to it. Stripped of both items the finding still stands on items 1-3, but on a narrower base than 'medium' implies.

### Evidence

- The domain repository DTOs carry five raw-JSON *string fields that are verbatim storage column contents.
  - `internal/domain/demand/types.go:15-27`
  - Reproduce: `sed -n '13,28p' internal/domain/demand/types.go`
- The blob is produced in an adapter (the scheduler) by marshalling a DH client entry type.
  - `internal/adapters/scheduler/dh_analytics_refresh_steps.go:140-143`
  - Reproduce: `sed -n '139,146p' internal/adapters/scheduler/dh_analytics_refresh_steps.go`
- The blob is consumed in the domain by unmarshalling into a hand-mirrored, unexported struct that duplicates the field names as string tags. Nothing ties the two structs together at compile time.
  - `internal/domain/demand/service.go:279-297`
  - Reproduce: `sed -n '279,293p' internal/domain/demand/service.go`
- A storage-decoding failure mode has leaked all the way into the API response type: the domain counts unparseable blobs and returns the count as a response field.
  - `internal/domain/demand/campaign_signals.go:137-152`
  - Reproduce: `sed -n '137,156p' internal/domain/demand/campaign_signals.go`
- The package doc states the intended decoupling that this seam does not achieve — the wire format is exactly what crosses the repository interface.
  - `internal/domain/demand/types.go:1-7`
  - Reproduce: `sed -n '1,7p' internal/domain/demand/types.go`

### Proposed fix

Define the velocity / demand / saturation payloads as exported domain structs in internal/domain/demand and change Repository to accept and return those structs. Let the postgres adapter own the JSON marshal/unmarshal (it already owns nullStringFromPtr/nullStringToPtr), and let the scheduler build the domain struct instead of a JSON string. A renamed DH field then breaks the build in the adapter rather than silently nilling a pointer in the leaderboard.

### Blast radius

- `internal/domain/demand/types.go`
- `internal/domain/demand/repository.go`
- `internal/domain/demand/service.go`
- `internal/domain/demand/campaign_signals.go`
- `internal/adapters/storage/postgres/dh_demand_repository.go`
- `internal/adapters/scheduler/dh_analytics_refresh_steps.go`
- `internal/testutil/mocks (demand repository mock)`

### Acceptance criteria

- [ ] `git grep -nE 'json.Unmarshal' -- internal/domain/demand/ | grep -v _test` returns empty.
- [ ] internal/domain/demand contains no field whose name ends in `JSON` holding a serialized payload.
- [ ] A unit test asserts that a DH velocity payload written by the scheduler round-trips to the domain struct the leaderboard reads, using the same Go type on both sides.
- [ ] GET the niches leaderboard and campaign-signals endpoints return byte-identical JSON for a fixture dataset before and after the change.
- [ ] `go build ./...`, `go test -race ./...`, and `make check` pass.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-34 — Give the DH tombstone threshold a home in the domain

**Linear:** [SLA-42](https://linear.app/slabledger/issue/SLA-42/give-the-dh-tombstone-threshold-a-home-in-the-domain)  
**Label:** Improvement · **Priority:** 4

<details><summary>Ticket body as posted</summary>

**Category:** Architecture · **Severity:** low · **Confidence:** strong · **Effort:** S
**Baseline revision:** `740976ec` · **Audit findings:** ARCH-003

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> Duplicated as an unexported constant in two adapter packages that are forbidden from importing each other — the duplication is a direct consequence of the architecture rule, so the fix belongs in the domain.

## Claim

The DH tombstone policy has no home in the domain: the threshold is duplicated as an unexported constant in two adapter packages that are forbidden from importing each other

**Subject:** `{'kind': 'symbol', 'identity': 'tombstoneThreshold (declared independently in internal/adapters/clients/dhprice and internal/adapters/storage/postgres)'}`

**Verifier correction (already applied to this ticket):** One overstatement in evidence item 1, which says the constant is 'used to make the same decision in each' package. Only the postgres copy makes the decision: dh_card_tombstone_store.go:32 is the authoritative IsTombstoned predicate and :77 bounds the Count query. The dhprice copy at provider.go:171 only selects which of two log messages to emit ('dh card tombstoned' vs 'dh card lookup failed') and gates nothing. Drift between the two therefore produces a misleading log line, not a wrong tombstone decision — which is consistent with the low severity already assigned, so the conclusion is unaffected. Separately, evidence item 3 says the two packages 'cannot share a constant directly'; strictly, check-imports.sh only forbids storage importing clients, so the reverse direction is not mechanically blocked. It would be an obvious layering inversion and no reviewer would take it, so the finding's conclusion that internal/domain or internal/platform is the only sensible home is right, but the word 'cannot' is stronger than the script enforces.

### Evidence

- The same policy constant is declared twice, in two separate adapter packages, and used to make the same decision in each.
  - Reproduce: `git grep -n 'tombstoneThreshold' -- 'internal/**/*.go' | grep -v _test`
- The domain interface encodes the rule only as English in a doc comment — there is no domain-level constant or predicate the two adapters could share.
  - `internal/domain/pricing/provider.go:206-228`
  - Reproduce: `sed -n '206,228p' internal/domain/pricing/provider.go`
- The two declaring packages cannot share a constant directly: the architecture check forbids storage adapters from importing client adapters, so the only legal shared home is internal/domain or internal/platform, and neither holds one.
  - `scripts/check-imports.sh:23-34`
  - Reproduce: `sed -n '23,34p' scripts/check-imports.sh`
- No shareable tombstone threshold identifier exists anywhere under internal/domain or internal/platform — the rule appears there only inside comments (provider.go:212, :226), never as code.
  - Reproduce: `git grep -nE '^[^/]*[Tt]ombstoneThreshold' -- 'internal/domain/**/*.go' 'internal/platform/**/*.go'`

### Proposed fix

Add an exported constant (or an IsTombstoned(attempts int) bool predicate) to internal/domain/pricing next to DHCardTombstoneRepo, and have both adapters reference it. Replace the 'attempts >= 3' prose in the interface doc with a pointer to the constant.

### Blast radius

- `internal/domain/pricing/provider.go`
- `internal/adapters/clients/dhprice/provider.go`
- `internal/adapters/storage/postgres/dh_card_tombstone_store.go`

### Acceptance criteria

- [ ] `git grep -n '= 3' -- internal/adapters/clients/dhprice internal/adapters/storage/postgres/dh_card_tombstone_store.go | grep -i tombstone` returns empty; the only declaration is under internal/domain/.
- [ ] A domain unit test pins the threshold boundary (attempts 2 -> not tombstoned, 3 -> tombstoned).
- [ ] The existing internal/adapters/storage/postgres/dh_card_tombstone_store_test.go passes unchanged.
- [ ] `go build ./...`, `go test -race ./...`, and `make check` pass.

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

## FU-35 — Route the local cents/dollars and grader-regex reimplementations through the shared helpers

**Linear:** [SLA-43](https://linear.app/slabledger/issue/SLA-43/route-the-local-centsdollars-and-grader-regex-reimplementations)  
**Label:** Improvement · **Priority:** 4

<details><summary>Ticket body as posted</summary>

**Category:** Architecture · **Severity:** low · **Confidence:** strong · **Effort:** S
**Baseline revision:** `740976ec` · **Audit findings:** DUP-001, DUP-002

Produced by the read-only tech-debt audit. Every claim below survived an adversarial verification pass whose brief was to refute it. Full record: `docs/audit/REPORT.md`.

> [!IMPORTANT]
> Both are 'call the existing shared function instead'. Note the verifier re-grounded DUP-002: it is a pure maintainability concern, NOT a demonstrated live defect — the divergence cannot be exercised by any valid grader title. Do not let the ticket imply a live bug.

## DUP-001 — Two local cents<->dollars conversion functions reimplement mathutil.ToCentsInt/ToDollars instead of calling it

**Subject:** `{'kind': 'symbol', 'identity': 'dhprice.dollarsToCents, handlers.centsToDollars vs mathutil.ToCentsInt/ToDollars'}`

### Evidence

- internal/domain/mathutil is the established, widely-used single implementation of dollar<->cent conversion (23+ call sites across adapters, scheduler, and inventory).
  - `internal/domain/mathutil/mathutil.go:15-27`
  - Reproduce: `grep -n 'func ToCents\|func ToCentsInt\|func ToDollars' internal/domain/mathutil/mathutil.go`
- mathutil.ToCents/ToCentsInt is already called from many packages, including the same dhprice package (provider.go) as the offending file (batch_adapter.go).
  - `internal/adapters/clients/dhprice/provider.go:13`
  - Reproduce: `grep -n 'mathutil\.ToCents\|mathutil\.ToDollars\|mathutil\.ToCentsInt' internal/adapters/clients/dhprice/provider.go internal/adapters/clients/dh/convert.go internal/adapters/scheduler/card_trajectory_refresh.go internal/domain/inventory/parse_ebay.go`
- batch_adapter.go, in the same dhprice package, defines its own dollarsToCents that reimplements mathutil.ToCentsInt's exact body (int(math.Round(dollars*100))) instead of calling the shared helper.
  - `internal/adapters/clients/dhprice/batch_adapter.go:89-91`
  - Reproduce: `sed -n '89,91p' internal/adapters/clients/dhprice/batch_adapter.go`
- pricing_api.go defines its own centsToDollars that reimplements mathutil.ToDollars's exact body (float64(cents) / 100) instead of calling the shared helper.
  - `internal/adapters/httpserver/handlers/pricing_api.go:58-60`
  - Reproduce: `sed -n '56,60p' internal/adapters/httpserver/handlers/pricing_api.go`
- Neither local function is exported or has its own test file, and each has exactly one call site pattern confined to its own file.
  - `internal/adapters/clients/dhprice/batch_adapter.go:77`
  - Reproduce: `grep -rn 'dollarsToCents\|centsToDollars' --include='*.go' .`

### Proposed fix

Delete dollarsToCents in internal/adapters/clients/dhprice/batch_adapter.go and centsToDollars in internal/adapters/httpserver/handlers/pricing_api.go; replace their call sites with mathutil.ToCentsInt and mathutil.ToDollars respectively (note ToDollars takes int64, so pricing_api.go call sites need an int64 cast or a small int-accepting wrapper if that conversion is undesirable).

### Blast radius

- `internal/adapters/clients/dhprice/batch_adapter.go`
- `internal/adapters/httpserver/handlers/pricing_api.go`

### Acceptance criteria

- [ ] go build ./... succeeds with dollarsToCents and centsToDollars removed and their call sites replaced with mathutil.ToCentsInt / mathutil.ToDollars
- [ ] go test ./internal/adapters/clients/dhprice/... ./internal/adapters/httpserver/handlers/... passes unchanged
- [ ] grep -rn 'func dollarsToCents\\|func centsToDollars' --include='*.go' . returns no results

## DUP-002 — Two independently-maintained grader/grade regexes accept different fractional-grade formats for the same concept

**Subject:** `{'kind': 'symbol', 'identity': 'inventory.ExtractGrade vs inventory.ExtractGraderAndGrade (grader-and-grade regex)'}`

**Verifier correction (already applied to this ticket):** The title's claimed consequence ('causing silent disambiguation misses') is not supported: PSA, BGS, CGC, and SGC grading scales never produce a grade with two or more fractional digits (grades are whole or single-decimal, e.g. 9, 9.5) -- so no real-world grader title can ever trigger the divergence the finding demonstrates with the synthetic input 'PSA 9.55'. The finding's own exclusions section already concedes this ('production PSA-sourced CSV data are effectively always single-decimal... flagged as a latent inconsistency rather than an active production bug'), so the self-reported severity is close to right, but the title overstates it as an active data-quality defect rather than a purely theoretical one. The real, defensible finding is narrower: three independently-maintained copies of the same regex fragment are a maintenance liability (the next person who needs a grade format change must remember to touch three files), not a currently-triggering bug.

### Evidence

- ExtractGrade (PSA-only, used by the PSA CSV import path and cert enrichment) accepts a multi-digit fractional grade component.
  - `internal/domain/inventory/import_parsing.go:43,49`
  - Reproduce: `sed -n '43,49p' internal/domain/inventory/import_parsing.go`
- ExtractGraderAndGrade (4-grader, used by the Shopify import path) is documented as the multi-grader counterpart to ExtractGrade but only accepts a single-digit fractional grade component -- the two regexes were clearly copy-derived (same '\\bGRADER\\s*(\\d{1,2}...)\\b' shape, same 1-10 range check) but have diverged in decimal precision.
  - `internal/domain/inventory/import_types.go:164,168`
  - Reproduce: `sed -n '164,168p' internal/domain/inventory/import_types.go; grep -n 'For multi-grader support' internal/domain/inventory/import_parsing.go`
- Concretely demonstrated divergence: a title containing 'PSA 9.55' matches ExtractGrade's regex in full but ExtractGraderAndGrade's regex only matches the truncated 'PSA 9', producing grade 9 instead of 9.55 for the same input string.
  - `internal/domain/inventory/import_parsing.go:43`
  - Reproduce: `python3 -c "import re; r1=re.compile(r'(?i)\bPSA\s*(\d{1,2}(?:\.\d+)?)\b'); r2=re.compile(r'(?i)\b(CGC|BGS|PSA|SGC)\s*(\d{1,2}(?:\.\d)?)\b'); t='Charizard PSA 9.55'; print(r1.search(t)); print(r2.search(t))"`
- A third, independently-defined grader regex exists in cardutil for suffix stripping (not extraction) and also uses the single-digit fractional form, confirming the pattern was copied a third time rather than shared.
  - `internal/platform/cardutil/normalize.go:30`
  - Reproduce: `grep -n 'PSAGradeSuffixRegex' internal/platform/cardutil/normalize.go`

### Proposed fix

Which regex is correct is undetermined -- no test or documented spec fixes the intended fractional-grade precision. ExtractGrade has exactly one test (TestService_ImportPSAExportGlobal_ExtractGrade, internal/domain/inventory/service_test.go:919) and it only exercises the whole-number case "PSA 9"; ExtractGraderAndGrade has zero test coverage. A fixer must first decide the intended fractional-grade format (consult production PSA-title data for any multi-digit-fraction grades before assuming ExtractGrade's more permissive `\d{1,2}(?:\.\d+)?` form is the one to standardize on) and only then hoist a single shared grade-fraction regex fragment (or helper function) used by ExtractGrade, ExtractGraderAndGrade, and cardutil.PSAGradeSuffixRegex, so future grade-format changes only need to happen once.

### Blast radius

- `internal/domain/inventory/import_parsing.go`
- `internal/domain/inventory/import_types.go`
- `internal/platform/cardutil/normalize.go`

### Acceptance criteria

- [ ] A single shared regex fragment or helper defines the grader+grade pattern; ExtractGrade, ExtractGraderAndGrade, and PSAGradeSuffixRegex all derive from it
- [ ] go test ./internal/domain/inventory/... ./internal/platform/cardutil/... passes unchanged
- [ ] A new table-driven test case confirms 'PSA 9.55'-style multi-digit fractional grades are handled identically by ExtractGrade and ExtractGraderAndGrade (either both accept or both reject, by explicit decision)

### Definition of done

- [ ] `make check` passes
- [ ] `go test -race ./...` passes
- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)

</details>

---

