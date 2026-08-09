# Tech-Debt Audit — Consolidated Report

**Baseline revision:** `06d9f8ce172dada98f307f860e2e2985c9fb5ca6`  
**Scope:** Go backend (domain + adapters), frontend (`web/src`), database + migrations, docs/config/CI/tests  
**Method:** 5 reference-mapping scouts → 8 judgment lenses → 12 adversarial verifiers → controller adjudication  
**The audit was strictly read-only.** No agent edited code. `git diff --stat main..HEAD -- internal/ cmd/ web/src/` is empty by construction.

## Summary

| Metric | Count |
|---|---|
| Findings filed | 49 |
| Confirmed | 39 |
| Confirmed, severity lowered | 9 |
| Refuted | 1 |
| **Confirmation rate** | **48/49 = 97%** |
| Ticketable | 39 |
| Fix units (tickets) | 32 |

Every finding received exactly one verdict from a verifier whose explicit brief was to **refute** it, defaulting to refuted under uncertainty. The refuted section below is the audit's own error rate and stays in the record.

### Findings by category

| Category | Findings | Ticketable |
|---|---|---|
| architecture | 3 | 3 |
| db | 6 | 5 |
| dead-code | 10 | 8 |
| docs | 13 | 9 |
| duplication | 5 | 4 |
| frontend | 3 | 3 |
| naming | 5 | 3 |
| size | 4 | 4 |

### Reconciliation

- Ticketable findings: **39**
- Findings rolled into fix units: **38**
- ⚠️ **Ticketable but not in any fix unit:** DB-001

## Ranked fix units

Ranked by risk × effort. A fix unit is **one independently mergeable PR**. Tier order is the recommended execution order.


---

## Tier: Documentation drift

### FU-01 — Correct the end-user guide against the shipped UI

**Severity:** high · **Effort:** M · **Rolls up:** DCT-001

> **Controller note.** CSV import, the campaign form, the sale-channel roster, and Card Ladder integration are all described as they are not. Highest-severity docs finding in the run and the only one facing end users rather than developers.

#### DCT-001 — The end-user guide misdescribes CSV import, the campaign form, the sale-channel roster, and Card Ladder integration

*Lens:* `docs-config-tests` · *Confidence:* `mechanical` · *Verdict:* `confirmed` · *Severity:* high

**Subject:** `{'kind': 'claim', 'identity': 'docs/USER_GUIDE.md — four load-bearing claims contradicted by the code'}`

**Evidence:**

- USER_GUIDE.md:102 tells the user to 'Upload a CSV with three columns: Card Title, Price, Date'. The PSA CSV parser looks for a header row containing 'cert number', 'listing title' and 'grade', and returns a fatal error when it cannot find one. A user who follows the documented format gets an import that fails outright, not a partial import.
  - `docs/USER_GUIDE.md:102`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/domain/csvimport/parse_helpers.go | sed -n '86,100p'`
- USER_GUIDE.md:107 says the import 'Defaults to grade 9 if no grade is found in the title'. No such default exists anywhere in the CSV import or inventory packages.
  - `docs/USER_GUIDE.md:107`
  - Reproduce: `git grep -nE 'Defaults? to grade 9|defaultGrade' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/domain/csvimport internal/domain/inventory`
- USER_GUIDE.md:50-64 documents (Sport at :57, Inclusion List at :64) the campaign-creation form as having a 'Sport' field and an 'Inclusion List' field. The form has neither. `inclusion_list` survives only as a write-only legacy mirror column derived by deriveLegacyMirror() for rollback compatibility; migration 000024 replaced the inclusion/exclusion model with a language axis and a subject-filter mode, and both of those new controls are undocumented.
  - `docs/USER_GUIDE.md:57`
  - Reproduce: `git grep -niE '"Sport"|label="Sport' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- web/src ; git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:web/src/react/ui/CampaignFormFields.tsx | grep -nE 'label="|ariaLabel='`
- USER_GUIDE.md:137-145 documents exactly four sale channels — eBay, TCGPlayer, Local, Other. Three of those four are marked in the source as legacy values kept only for backward-compatible DB reads, and the two current channels (Website, InPerson) are absent from the guide.
  - `docs/USER_GUIDE.md:137`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/domain/inventory/core_types.go | sed -n '82,96p'`
- USER_GUIDE.md:266 states 'Card Ladder does not offer an API, so CL values are manually entered per purchase.' There is an eight-file Card Ladder HTTP client, a 525-line refresh scheduler that writes CL values back onto purchases, and seven admin routes driving it.
  - `docs/USER_GUIDE.md:266`
  - Reproduce: `git ls-tree --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 internal/adapters/clients/cardladder/ ; git grep -n 'admin/cardladder' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/adapters/httpserver/routes.go ; git grep -n 'UpdatePurchaseCLValue' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/adapters/scheduler/cardladder_refresh.go`
- The document is stamped 'Last updated: March 2026', predating migration 000024's targeting-axes rewrite, the csvimport carve-out (SLA-35) and the Card Ladder client. This is the only end-user-facing document in the repository, and it produced zero docs-map records, so nothing else in this audit covers it.
  - `docs/USER_GUIDE.md:308`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:docs/USER_GUIDE.md | tail -3`

**Proposed fix:** Rewrite the four stale sections of docs/USER_GUIDE.md against the current code: the CSV Import section against ParsePSAExportRows' required headers (cert number / listing title / grade / price paid) and the actual upload routes (/api/purchases/import-certs|-external|-orders|-psa); the campaign-creation table against CampaignFormFields.tsx, dropping Sport and Inclusion List and adding the target-language axis and subject-filter mode; the Sale Channels table against the current constants (Ebay, Website, InPerson), noting the legacy values as historical; and the Card Ladder FAQ entry to describe the client, the refresh scheduler and the admin surface. Re-stamp the 'Last updated' line.

**Blast radius:**

- `docs/USER_GUIDE.md`

**Acceptance criteria:**

- [ ] Every column name in the CSV Import section appears in the `psaHeaderAliases`/`FindPSAHeaderRow` inputs in internal/domain/csvimport/parse_helpers.go.
- [ ] Every field name in the campaign-creation table appears as a `label=` or `ariaLabel=` in web/src/react/ui/CampaignFormFields.tsx, and every such label appears in the table.
- [ ] Every channel named in the Sale Channels table appears in the non-legacy const block of internal/domain/inventory/core_types.go, or is explicitly labelled legacy.
- [ ] `git grep -n 'does not offer an API' docs/USER_GUIDE.md` returns empty.
- [ ] `git grep -nE 'Sport|Inclusion List' docs/USER_GUIDE.md` returns empty.

### FU-02 — Fix the run instructions: the bare binary binds :8080, and --port is a no-op without --web

**Severity:** high · **Effort:** S · **Rolls up:** DCT-002, DCT-003

> **Controller note.** ADJUDICATED — clustered on the -web flag, which is the single subject both findings turn on. DCT-002 is the debt: every document says run ./slabledger and reach :8081, but the bare binary binds 127.0.0.1:8080 and `server --port N` silently does nothing without --web. DCT-003 is NOT a defect report — it is the verdict on a Phase 1 lead, concluding that ModeConfig.WebMode is LIVE and the config map's single dead field is a false positive. Its whole fix is a comment at internal/platform/config/types.go:7 naming WebMode's setters so a later scan does not re-file it. No code is removed. The finding says so itself: 'The genuine debt in this area is DCT-002, not this field.'

#### DCT-002 — Every document tells you to run `./slabledger` and reach :8081; the bare binary binds 127.0.0.1:8080, and `server --port N` is a no-op without --web

*Lens:* `docs-config-tests` · *Confidence:* `mechanical` · *Verdict:* `confirmed` · *Severity:* high

**Subject:** `{'kind': 'config', 'identity': 'Server.ListenAddr default vs the documented :8081 / --port contract'}`

**Evidence:**

- The listen address defaults to 127.0.0.1:8080 and is only re-derived from Mode.WebPort inside FromFlags, and only when Mode.WebMode is true. WebMode has no default and no env binding — it is set exclusively by the -web/--web flag.
  - `internal/platform/config/loader.go:252`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/platform/config/defaults.go | sed -n '7,16p' ; git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/platform/config/loader.go | sed -n '231,259p'`
- cmd/slabledger/server.go only falls back to WebPort when ListenAddr is the empty string. It is never empty — the default is a non-empty literal — so Mode.WebPort never reaches the listener except through the WebMode branch above.
  - `cmd/slabledger/server.go:217`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:cmd/slabledger/server.go | sed -n '216,220p'`
- HTTP_LISTEN_ADDR cannot rescue the bare invocation: .env.example ships it empty, and envString skips empty values.
  - `.env.example:55`
  - Reproduce: `git grep -n 'HTTP_LISTEN_ADDR' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- .env.example internal/platform/config/loader.go`
- Four places document the bare invocation and promise :8081 — including the binary's own help text, which additionally advertises `slabledger server --port 9090` as the way to change the port. Without --web that flag parses and is then discarded.
  - `CLAUDE.md:10`
  - Reproduce: `git grep -n './slabledger$\|Start web server on :8081\|localhost:8081\|go run ./cmd/slabledger' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- CLAUDE.md README.md docs/DEVELOPMENT.md ; git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:cmd/slabledger/main.go | grep -n 'default port 8081\|--port 9090'`
- Every invocation that actually works passes --web explicitly, which is why the drift has stayed invisible: the Makefile run target and the Docker CMD both do. Nothing in the repo runs the binary the way the documentation says to.
  - `Dockerfile:98`
  - Reproduce: `git grep -n 'cmd/slabledger --web\|CMD \[' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- Makefile Dockerfile`
- The consequence is not cosmetic. The Vite dev proxy hard-codes localhost:8081, and the devcontainer publishes 8081, so a developer following README.md gets a server on a port nothing proxies to — bound to 127.0.0.1, which is additionally unreachable through the devcontainer's published port.
  - `web/vite.config.js:56`
  - Reproduce: `git grep -n '8081' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- web/vite.config.js .devcontainer/docker-compose.yml`

**Proposed fix:** Pick one of two directions and make the documentation match. Either (a) change the default so the bare binary does what four documents say — set Server.ListenAddr's default empty (letting cmd/slabledger/server.go:218's existing WebPort fallback fire) or default WebMode to true, since the binary has no non-web mode left; or (b) keep the flag requirement and correct README.md:20, CLAUDE.md:10, docs/DEVELOPMENT.md:256 and showHelp() to say `./slabledger --web --port 8081`. Option (a) is smaller and removes the trap; option (b) preserves current behavior. Either way, showHelp()'s `slabledger server --port 9090` line must stop advertising a flag that is discarded.

**Blast radius:**

- `internal/platform/config/defaults.go`
- `internal/platform/config/loader.go`
- `internal/platform/config/config_test.go`
- `cmd/slabledger/main.go`
- `README.md`
- `CLAUDE.md`
- `docs/DEVELOPMENT.md`

**Acceptance criteria:**

- [ ] Building the binary and running it exactly as the chosen document says produces a server reachable at the address that document names — demonstrated by the startup log line and a successful `curl /api/health`.
- [ ] `go test ./internal/platform/config/...` passes; TestFromFlags_WebMode's assertion that --web --port 3000 yields 0.0.0.0:3000 still holds.
- [ ] A test exists that pins the bare-invocation listen address, so the default and the documentation cannot drift apart again silently.
- [ ] `docker build` + `docker run` still serves on 8081 (the Dockerfile CMD path is unchanged or updated in step).

#### DCT-003 — Verdict on the Phase 1 lead: WebMode is live, not dead — the config map's single dead field is a false positive

*Lens:* `docs-config-tests` · *Confidence:* `mechanical` · *Verdict:* `confirmed` · *Severity:* low

**Subject:** `{'kind': 'config', 'identity': 'ModeConfig.WebMode (internal/platform/config/types.go:7)'}`

**Evidence:**

- The config map scored ModeConfig.WebMode as the run's only dead config field on the grounds that no reference to it exists outside internal/platform/config. That count is correct and the conclusion does not follow: the field's consumer is a CLI flag binding whose only setters are outside Go source entirely.
  - `internal/platform/config/types.go:7`
  - Reproduce: `git grep -nE '\bWebMode\b' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- '*.go'`
- The field is bound to the -web flag and gates a real behavioral branch — it decides whether Server.ListenAddr is re-derived from Mode.WebPort. Deleting it would silently change the address the server binds under every shipping invocation.
  - `internal/platform/config/loader.go:252`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/platform/config/loader.go | sed -n '233p;250,258p'`
- Two live setters exist, both outside Go: the Docker CMD that every deployed container runs, and the Makefile run target. This is preamble mechanism 7 (registration / wiring) in a form no Go-source grep can see.
  - `Dockerfile:98`
  - Reproduce: `git grep -n 'CMD \[\|cmd/slabledger --web' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- Dockerfile Makefile`
- main.go treats --web as a first-class dispatch token, further confirming it is part of the supported CLI surface rather than a vestige.
  - `cmd/slabledger/main.go`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:cmd/slabledger/main.go | grep -n -A2 'case "--web"'`

**Proposed fix:** No removal. Amend the config map's classification, and add a short comment at internal/platform/config/types.go:7 recording that WebMode's only setters are the -web flag (Dockerfile CMD, Makefile run target) so the next reference scan does not re-file it as dead. The genuine debt in this area is DCT-002, not this field.

**Blast radius:**

- `internal/platform/config/types.go`
- `docs/audit/runs/2026-08-08/maps/config-map.json`

**Acceptance criteria:**

- [ ] No code is removed; `go build ./... && go test ./internal/platform/config/...` still pass unchanged.
- [ ] A reviewer reading internal/platform/config/types.go:7 can name WebMode's setters without leaving the file.

### FU-03 — Sweep the developer docs against the tree — five removed subsystems and four wrong filenames

**Severity:** high · **Effort:** L · **Rolls up:** DCT-008, DCT-009

> **Controller note.** DCT-008: five subsystems that no longer exist are documented, three of them in more than one document. DCT-009: one live file is given four different wrong names across four documents, and service_interfaces.go is described as holding interfaces it does not contain. Same sweep, same evidence method. Related to SLA-46 (#560, 'docs: correct CLAUDE.md against the tree'), which is CLOSED and landed BEFORE this baseline — so this is ground that sweep did not reach, not a re-file of it.

#### DCT-008 — The developer docs describe five subsystems that no longer exist, three of them in more than one document

*Lens:* `docs-config-tests` · *Confidence:* `mechanical` · *Verdict:* `confirmed` · *Severity:* high

**Subject:** `{'kind': 'claim', 'identity': 'Five removed subsystems still documented as present: internal/platform/cache, internal/platform/errors, the TCGdex client, the Cache Warmup scheduler, and .github/workflows/fly-deploy.yml'}`

**Verifier correction:** One parenthetical sub-claim inside evidence leg 1 is false: 'canonjson/ and cardutil/ exist and appear in no document.' cardutil/ IS documented — docs/ARCHITECTURE.md:86 ('cardutil/  # Card name/set normalization') and internal/README.md:53. Only canonjson/ is undocumented. The leg's load-bearing claim (cache/ and errors/ are documented and absent) is unaffected. Separately, internal/platform/storage/ exists in the tree and is missing from docs/ARCHITECTURE.md's platform list — an additional instance of the same defect the finding did not claim, so the fix scope in acceptance criterion 2 is slightly larger than the evidence enumerates.

**Evidence:**

- internal/platform/cache and internal/platform/errors are asserted by three documents between them. Neither directory exists; canonjson/ and cardutil/ exist and appear in no document.
  - `CLAUDE.md:38`
  - Reproduce: `git ls-tree --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 internal/platform/`
- docs/DEVELOPMENT.md:134 carries a 30-line 'Cache System' section documenting a type-safe cache API with a worked example. The only occurrence of the API in the whole repository is that documentation line.
  - `docs/DEVELOPMENT.md:134`
  - Reproduce: `git grep -rn 'NewCardSliceCache' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6`
- docs/DEVELOPMENT.md:204 documents a TCGdex.dev integration and cites internal/adapters/clients/tcgdex/. No such client exists; the only surviving trace of TCGdex is example URLs inside mock doc-comments.
  - `docs/DEVELOPMENT.md:204`
  - Reproduce: `git ls-tree --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 internal/adapters/clients/ ; git grep -in 'tcgdex' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal cmd web`
- docs/SCHEDULERS.md:106 devotes a full section to a Cache Warmup scheduler with a file (cache_warmup.go) and three env gates. The env gates do not exist anywhere, and the file does not exist. The same phantom file is repeated in the File Layout block at :205, and the three phantom vars are three of the 25 identifiers the document presents as env gates at :81.
  - `docs/SCHEDULERS.md:106`
  - Reproduce: `git grep -n 'CACHE_WARMUP' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal cmd .env.example ; git ls-tree --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 internal/adapters/scheduler/ | grep cache`
- docs/DEVELOPMENT.md:285 tells an operator that deployment runs through .github/workflows/fly-deploy.yml with a FLY_API_TOKEN secret. That workflow does not exist. fly.toml is tracked, so deployment is real — but an operator debugging a stuck release would go looking for a pipeline that is not in this repository.
  - `docs/DEVELOPMENT.md:285`
  - Reproduce: `git ls-tree --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 .github/workflows/`

**Proposed fix:** Delete the five dead sections rather than trying to salvage them: the Cache System section of docs/DEVELOPMENT.md, its TCGdex entry in API Integrations, its fly-deploy paragraph (replace with how deployment is actually triggered — auto-deploy is configured in Fly, not in this repo), the Cache Warmup section and File Layout entry in docs/SCHEDULERS.md and its three env rows, and the cache/ and errors/ entries in the platform trees at CLAUDE.md:38, internal/README.md:52 and :240 and docs/ARCHITECTURE.md:85. While editing the platform trees, add the two real directories they omit (canonjson, cardutil).

**Blast radius:**

- `docs/DEVELOPMENT.md`
- `docs/SCHEDULERS.md`
- `docs/ARCHITECTURE.md`
- `internal/README.md`
- `CLAUDE.md`

**Acceptance criteria:**

- [ ] `git grep -rn 'platform/cache\|platform/errors\|tcgdex\|CACHE_WARMUP\|cache_warmup\|fly-deploy\|NewCardSliceCache' -- '*.md' :!docs/audit` returns empty.
- [ ] Every directory named in each document's internal/platform tree is present in `git ls-tree --name-only HEAD internal/platform/`, and every directory in that listing appears in the tree or is explicitly marked as an omission.
- [ ] Every env var presented as a gate in docs/SCHEDULERS.md resolves to a read in internal/platform/config.

#### DCT-009 — The same live file is given four different wrong names across four documents, and service_interfaces.go is described as holding interfaces it does not contain

*Lens:* `docs-config-tests` · *Confidence:* `mechanical` · *Verdict:* `confirmed` · *Severity:* medium

**Subject:** `{'kind': 'claim', 'identity': 'File-path claims for live code that resolve to nothing, across CLAUDE.md, docs/ARCHITECTURE.md, docs/DEVELOPMENT.md, internal/README.md and docs/DH_INVENTORY.md'}`

**Verifier correction:** The title is inaccurate: core_types.go is given TWO wrong names (types.go, types_core.go) across THREE documents, not 'four different wrong names across four documents'. The finding's own evidence leg 1 states the correct two-name/three-document picture, so this is a title defect only. Two line-number drifts: docs/ARCHITECTURE.md's `types.go` is at :52 and `service_interfaces.go` at :53, not the :50/:51 the finding cites (the :240-244 table range is exact); CLAUDE.md's parse_*.go locator is at :85, not :84. Also unclaimed but in the same fix scope: the pricelookup mislocation recurs at docs/DEVELOPMENT.md:58 and :224, not only :40.

**Evidence:**

- internal/domain/inventory/core_types.go is named types.go by docs/ARCHITECTURE.md:50 and docs/DEVELOPMENT.md:34, types_core.go by internal/README.md:627, and correctly only by CLAUDE.md:76. None of the three wrong names is a tracked path.
  - `docs/ARCHITECTURE.md:50`
  - Reproduce: `git ls-tree --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 internal/domain/inventory/ | grep -i type`
- docs/ARCHITECTURE.md:51 and five rows of its Key Interfaces table at :240-244, plus docs/DEVELOPMENT.md:34, all point CampaignRepository / PurchaseRepository / SaleRepository / AnalyticsRepository / FinanceRepository at service_interfaces.go. That file declares no repository interface at all — it holds six service-facing interfaces. A reader following the citation finds the wrong contract.
  - `docs/ARCHITECTURE.md:51`
  - Reproduce: `git grep -nE 'type [A-Za-z]+Repository interface' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/domain/inventory/service_interfaces.go`
- docs/DEVELOPMENT.md:40 cites internal/adapters/clients/pricelookup/adapter.go for the PriceLookup adapter. No tracked path contains 'pricelookup'. The adapter is a domain package, internal/domain/pricing/lookup/adapter.go — and the mislocation matters here specifically, because it inverts the hexagonal placement the rest of the docs are teaching.
  - `docs/DEVELOPMENT.md:40`
  - Reproduce: `git ls-tree -r --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 | grep -i pricelookup`
- docs/DH_INVENTORY.md:198 maps card-name cleaning to internal/domain/inventory/dh_helpers.go. The file lives in a different package now.
  - `docs/DH_INVENTORY.md:198`
  - Reproduce: `git ls-tree -r --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 | grep dh_helpers`
- docs/DEVELOPMENT.md:23's 'Price Units (Critical)' section cites two Key Files, neither of which exists. This is the section the document itself marks Critical, and the cents-vs-dollars convention it exists to explain is separately contradicted at CLAUDE.md:194.
  - `docs/DEVELOPMENT.md:23`
  - Reproduce: `git ls-tree -r --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 | grep -E 'handlers/formatter\.go|pages/CampaignDetailPage\.tsx'`
- CLAUDE.md:84 tells the reader to run `ls internal/domain/inventory/parse_*.go` for the CSV parser set. That glob matches nothing; CSV parsing moved to internal/domain/csvimport under SLA-35, a package internal/README.md:86's domain table also omits while still attributing 'CSV import' to inventory/.
  - `CLAUDE.md:84`
  - Reproduce: `git ls-tree --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 internal/domain/inventory/ | grep '^internal/domain/inventory/parse_' ; git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/domain/csvimport/service.go | sed -n '1,6p'`

**Proposed fix:** Correct each citation to the path that exists: core_types.go (ARCHITECTURE.md:50, DEVELOPMENT.md:34, internal/README.md:627); repository_*.go rather than service_interfaces.go for the eight repository interfaces (ARCHITECTURE.md:51 and the five Key Interfaces rows at :240-244, DEVELOPMENT.md:34); internal/domain/pricing/lookup/adapter.go (DEVELOPMENT.md:40); internal/domain/dhlisting/dh_helpers.go (DH_INVENTORY.md:198); internal/domain/csvimport/ (CLAUDE.md:84 and internal/README.md:86). Delete or re-source the two Price Units key-file rows at DEVELOPMENT.md:23. The cluster is large enough that a `make check` step verifying every `internal/`-prefixed path in tracked Markdown resolves would be cheaper than the next repeat.

**Blast radius:**

- `docs/ARCHITECTURE.md`
- `docs/DEVELOPMENT.md`
- `docs/DH_INVENTORY.md`
- `internal/README.md`
- `CLAUDE.md`
- `scripts/ (optional new path-check)`

**Acceptance criteria:**

- [ ] Every path matching `(internal|cmd|web/src|scripts)/[A-Za-z0-9_./-]+\.(go|ts|tsx|sh)` in tracked Markdown outside docs/audit/ and docs/plans/ appears in `git ls-tree -r --name-only HEAD`.
- [ ] `git grep -nE 'service_interfaces\.go' -- '*.md'` no longer associates that file with any *Repository interface.
- [ ] If a checker is added: it is wired into `make check` and fails on a deliberately introduced bad path.

### FU-04 — Remove or register the six API endpoints README advertises, and fix DEVELOPMENT.md's two broken monitoring commands

**Severity:** medium · **Effort:** M · **Rolls up:** DCT-010

> **Controller note.** Both of DEVELOPMENT.md's monitoring commands fail as written. The verifier was instructed not to execute anything that would touch a live service; the endpoint half is established statically against the route table.

#### DCT-010 — README advertises six unregistered API endpoints, and both of DEVELOPMENT.md's monitoring commands fail

*Lens:* `docs-config-tests` · *Confidence:* `strong` · *Verdict:* `confirmed` · *Severity:* medium

**Subject:** `{'kind': 'claim', 'identity': 'README.md:88-113 endpoint tables and docs/DEVELOPMENT.md:235-238 monitoring commands'}`

**Verifier correction:** The count is off by one, in the direction that understates the defect: the finding's title and evidence say 'six of the nineteen', then enumerate seven paths. The enumerated list is complete and correct; the word 'six' (and the docs-map note's 'remaining thirteen', really twelve) is wrong. This miscount originates in the docs-map record README.md:endpoint-table-unregistered-routes and was copied forward. Second, docs/DEVELOPMENT.md's Monitoring section (:228) contains THREE commands, not two — `curl /api/health` is registered at router.go:246 and works. The finding's 'both of DEVELOPMENT.md's monitoring commands fail' is true of the two it names but is not true of the section. Acceptance criterion 2 ('Both commands ... execute successfully against a running local server') is therefore mis-scoped and cannot be satisfied as written for a doc-only fix; a fixer should restate it as 'each command in the Monitoring section names a registered route or an implemented subcommand'.

**Evidence:**

- Six of the nineteen endpoints in README.md's two tables are registered nowhere in router.go or routes.go, per the docs-map's set-difference against every registered path. The contrast with docs/API.md is stark: API.md's 115 documented paths reconcile exactly, so the abbreviated README table is the outlier, not the API surface.
  - `README.md:88`
  - Reproduce: `jq -r '.records[] | select(.identity=="README.md:endpoint-table-unregistered-routes") | .command, .note' docs/audit/runs/2026-08-08/maps/docs-map.json`
- docs/DEVELOPMENT.md's Monitoring section gives two commands; both fail. `./slabledger admin cache-stats` is not a subcommand — admin.go dispatches only version, print-config and help — and the curl targets an unregistered path.
  - `docs/DEVELOPMENT.md:235`
  - Reproduce: `git grep -n 'cache-stats' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- cmd internal`
- The /api/sets and /api/status/api-usage phantoms are not confined to Markdown — they are also asserted inside Go source, which is why documentation-only fixes will leave the contradiction half-standing. That half belongs to another lens; see the pointer record DCT-012.
  - `internal/adapters/httpserver/handlers/api_status_handler.go:30`
  - Reproduce: `jq -r '.records[] | select(.identity=="pointer:stale-endpoint-doc-comments-in-go") | .defined_at, .note' docs/audit/runs/2026-08-08/maps/docs-map.json`

**Proposed fix:** Replace README.md's two hand-maintained endpoint tables with a pointer to docs/API.md, which is accurate and complete — duplicating a 115-endpoint surface in an abbreviated table is what produced six phantoms. In docs/DEVELOPMENT.md, delete the `admin cache-stats` line and correct the curl to /api/admin/api-usage.

**Blast radius:**

- `README.md`
- `docs/DEVELOPMENT.md`

**Acceptance criteria:**

- [ ] Every path named in README.md appears in the set registered by internal/adapters/httpserver/router.go and routes.go, or README no longer enumerates endpoints at all.
- [ ] Both commands in docs/DEVELOPMENT.md's Monitoring section execute successfully against a running local server.

### FU-05 — Make the mock guide's worked example compile

**Severity:** medium · **Effort:** M · **Rolls up:** DCT-011

> **Controller note.** internal/testutil/mocks/README.md is the file CLAUDE.md points every contributor at for the mock pattern. Its worked example, its sentinel-error list, and its exceptions all fail to compile or resolve.

#### DCT-011 — The mock guide's worked example, its sentinel-error list, and its exceptions all fail to compile or resolve

*Lens:* `docs-config-tests` · *Confidence:* `mechanical` · *Verdict:* `confirmed` · *Severity:* medium

**Subject:** `{'kind': 'claim', 'identity': 'internal/testutil/mocks/README.md — the mock-authoring guide CLAUDE.md points contributors at'}`

**Verifier correction:** Leg 3 is the weakest and is loosely described. The README does not 'concede the exception' at :184 — that line says `MockBehavior`/`MockOption` in common.go 'are used only by card/HTTP mocks (legacy); prefer the Fn-field pattern for all new mocks', which implies a legacy exception without naming MockPriceProvider. The contradiction with the :15 universal is real but softer than 'the headline rule and the caveat contradict each other'. The arithmetic is also wrong: :184 is 169 lines after :15, not 176, and the file is 185 lines total. Second, a subject-scope mismatch: the finding's subject identity is internal/testutil/mocks/README.md, but leg 2 (the only 'worked example' leg, and the one carrying the compile claim in the title) lives in internal/README.md:271. A fixer working from the subject line alone would edit the wrong file for a quarter of the finding; the finding's own prose does say so, but acceptance criterion 2 ('the example code block compiles when pasted into a scratch test in internal/testutil/mocks/') does not name which file's example it means.

**Evidence:**

- The README names inventory.ErrDuplicateCert as one of five key sentinel errors. That symbol does not exist — the real one is ErrDuplicateCertNumber. Copying the documented name into a test is a compile error, and this README is the file CLAUDE.md:184 designates as the full mock guide.
  - `internal/testutil/mocks/README.md:176`
  - Reproduce: `git grep -nE 'ErrDuplicateCert\b' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- '*.go'`
- The mock guide's companion example (internal/README.md:271, the copy CLAUDE.md sends contributors to alongside the mocks README) gives MockPriceProvider a public GetPriceFunc field. The real type has two unexported fields and is configured through functional options. The documented field name appears nowhere in Go source — only in internal/README.md's copy of the same wrong example.
  - `internal/README.md:271`
  - Reproduce: `git grep -n 'GetPriceFunc' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 ; git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/testutil/mocks/price_provider.go | sed -n '10,14p'`
- The README opens by asserting 'All mocks use the Fn-field pattern: every interface method has a corresponding Fn field.' MockPriceProvider has no Fn field at all, and the same README concedes the exception 176 lines later — so the headline rule and the caveat contradict each other inside one file.
  - `internal/testutil/mocks/README.md:15`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/testutil/mocks/README.md | sed -n '15p;184p'`
- The Usage Notes carve out an exception for two packages that do not exist. The only occurrence of either package name in the entire tree is the README line asserting the exception.
  - `internal/testutil/mocks/README.md:182`
  - Reproduce: `git grep -n 'package picks\|package favorites' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6`

**Proposed fix:** Correct four things in internal/testutil/mocks/README.md: rename ErrDuplicateCert to ErrDuplicateCertNumber; replace the MockPriceProvider example with the real NewMockPriceProvider(opts ...MockOption) shape (or pick a mock that genuinely uses Fn fields for the illustration); soften the ':15' claim to match the ':184' caveat rather than contradicting it; and delete the picks/favorites exception. Fix the duplicated wrong example at internal/README.md:271-275 in the same pass, since it is a copy.

**Blast radius:**

- `internal/testutil/mocks/README.md`
- `internal/README.md`

**Acceptance criteria:**

- [ ] Every Go identifier named in internal/testutil/mocks/README.md resolves — `git grep -nE '\b<Name>\b' -- '*.go'` is non-empty for each.
- [ ] The example code block compiles when pasted into a scratch test in internal/testutil/mocks/.
- [ ] `git grep -n 'package picks\|package favorites'` returns empty.

### FU-06 — Fix the leaf-rule worked example and CLAUDE.md's sibling roster

**Severity:** low · **Effort:** S · **Rolls up:** DCT-013

> **Controller note.** ADJUDICATED — absorbs pointers FE-006 and ARCH-004, both verified `ticketable: false` precisely because they restate this same internal/README.md material from another lens. Do not file them separately. internal/README.md's leaf-rule example names three packages deleted in the baseline's PARENT commit, and CLAUDE.md's dated sibling roster is missing csvimport. Note this is a regression against SLA-48 (#610, leaf/non-leaf taxonomy) and SLA-46 (#560, CLAUDE.md swept against the tree) — both CLOSED and both landed before this baseline. CLAUDE.md hedges the roster with 'derive it rather than trusting this sentence', which mitigates but does not repair it.

#### DCT-013 — The leaf-rule worked example in internal/README.md names three packages deleted in the baseline's parent commit, and CLAUDE.md's dated sibling roster is missing csvimport

*Lens:* `docs-config-tests` · *Confidence:* `mechanical` · *Verdict:* `confirmed_lower_severity` · *Severity:* low

**Subject:** `{'kind': 'claim', 'identity': 'internal/README.md:138'}`

**Verifier correction:** Nothing factual. The severity is overstated at medium once the two halves are weighed separately. Half B is a self-acknowledged defect: CLAUDE.md:106-118 states the roster is derived not listed, supplies the exact derivation command three lines above the list, and closes the sentence with 'but derive it rather than trusting this sentence.' A reader following the document as written never relies on the stale list, and the finding itself concedes there is no enforcement consequence — so half B is close to zero-impact and cannot carry a medium. Half A is a genuine defect with no disclaimer, but its consequence is comprehension friction in one illustrative paragraph: the rule itself, its enforcement, and the three surviving legal edges are all still correct, and the reader can substitute any live leaf. That is a low. Ticketability is unaffected — the four acceptance criteria are the sharpest in this batch and a fixer can prove the fix from them alone.

**Evidence:**

- internal/README.md:137-138 illustrates the leaf rule with six edges. Three of them name packages that do not exist at this baseline: internal/domain/advisor, internal/domain/ai and internal/domain/scoring. The paragraph is the section's entire worked example, and a reader who tries to resolve `advisor -> ai` or `advisor -> scoring` against the tree finds nothing. This matters more than an ordinary stale example because CLAUDE.md defers to this section for the leaf/non-leaf definition that scripts/check-imports.sh enforces in `make check`.
  - `internal/README.md:138`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/README.md | sed -n '137,139p' ; git ls-tree -d --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 internal/domain/advisor internal/domain/ai internal/domain/scoring`
- The three packages were deleted in e46370e5, the baseline's immediate parent, and that commit DID sweep internal/README.md - it removed two other `scoring/` mentions - but left the leaf-rule example untouched. The staleness is one commit old and was introduced by a sweep that already had this file open.
  - `internal/README.md:138`
  - Reproduce: `git log --oneline -1 --all -- internal/domain/advisor internal/domain/ai internal/domain/scoring ; git show e46370e5 -- internal/README.md | grep -E '^[-+]' | grep -v '^[-+][-+][-+]'`
- The same class of defect appears in CLAUDE.md:116-118, which states the governed-sibling set 'As of 2026-08-08' - the baseline's own commit date - and lists ten packages. The derivation command the document itself supplies at CLAUDE.md:112-113, run at the baseline, yields eleven: csvimport is missing from the prose. csvimport is governed by scripts/check-imports.sh regardless, because the checker derives membership from the tree, so this is a documentation defect and not an enforcement gap.
  - `CLAUDE.md:116`
  - Reproduce: `git grep -l 'internal/domain/inventory' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- 'internal/domain/*/*.go' 'internal/domain/*/*/*.go' | grep -v _test.go | sed 's/^[^:]*://' | xargs -n1 dirname | sort -u`
- Both instances share a root cause with the CSV-parser locator already recorded as the sixth evidence entry of DCT-009 (CLAUDE.md:84's `ls internal/domain/inventory/parse_*.go`, which matches nothing since SLA-35 moved parsing to internal/domain/csvimport). DCT-009 covers the locator; this finding covers the leaf/sibling rule's own illustration and roster. They are filed separately because the fixes are independent - one edits a file-path citation, the other edits the architecture rule's worked example after a package deletion.
  - `CLAUDE.md:84`
  - Reproduce: `git ls-tree --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 internal/domain/inventory/ | grep '^internal/domain/inventory/parse_'`

**Proposed fix:** In internal/README.md:137-138, replace the three edges naming deleted packages with edges that resolve at HEAD (for example `csvimport -> constants`, `tuning -> mathutil`, `finance -> timeutil`), keeping the three that already resolve. In CLAUDE.md:116-118, either add csvimport to the roster or delete the dated sentence outright and leave only the derivation command above it, which cannot go stale.

**Blast radius:**

- `internal/README.md`
- `CLAUDE.md`

**Acceptance criteria:**

- [ ] Every package named in internal/README.md's leaf-rule example resolves: for each `X -> Y` edge in that paragraph, `go list ./internal/domain/X` and `go list ./internal/domain/Y` both succeed.
- [ ] Each example edge is still a legal one under the rule: for every edge `X -> Y`, `go list -deps ./internal/domain/Y | grep -q '/internal/domain/inventory$'` returns non-zero (Y is a leaf).
- [ ] If CLAUDE.md retains a prose roster, it matches the derivation exactly: the output of `grep -rl --include='*.go' 'internal/domain/inventory' internal/domain | grep -v _test.go | xargs -n1 dirname | sort -u` equals the listed set, csvimport included.
- [ ] `make check` still passes, confirming no enforcement behavior was changed by the documentation edit.


---

## Tier: Test coverage

### FU-07 — Give the seven assertion-free integration tests a failure mode

**Severity:** medium · **Effort:** M · **Rolls up:** DCT-005

> **Controller note.** They swallow API errors, cannot go red, and burn real DH Enterprise quota on a schedule in CI. The quota claim is not statically provable and the verifier was explicitly barred from running the suite to check it — the ticket should carry that as a stated assumption, not as a measured fact.

#### DCT-005 — Seven integration tests contain no failure assertion and swallow API errors — they burn real DH Enterprise quota in scheduled CI and cannot go red

*Lens:* `docs-config-tests` · *Confidence:* `mechanical` · *Verdict:* `confirmed` · *Severity:* medium

**Subject:** `{'kind': 'test', 'identity': 'internal/integration/dh_search_diag_test.go and internal/integration/dh_match_search_test.go — 7 assertion-free test functions'}`

**Verifier correction:** One qualifier, not a defect in the finding's core: the word 'paid' in 'consume paid DH Enterprise calls' is not establishable by static analysis — nothing in the repository states DH Enterprise metering or pricing, and I did not run the suite. What IS statically established is that the calls are live, third-party, daily, and at least 15 in one function alone. The finding stands on that leg; a ticket should say 'live API calls on a daily schedule' rather than assert a quota cost.

**Evidence:**

- Across the 243 tracked *_test.go files and 1376 top-level Test functions, 19 have no t.Error/t.Fatal/t.Fail/t.Skip and no require./assert. Twelve of those are legitimate (compile-time interface checks, must-not-panic checks, Stop-idempotency, PrintConfig smoke). The remaining seven are all in two integration files and are diagnostics, not tests.
  - `internal/integration/dh_search_diag_test.go:17`
  - Reproduce: `git ls-tree -r --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 | grep '_test\.go$' | while read f; do git show "06d9f8ce172dada98f307f860e2e2985c9fb5ca6:$f" | awk -v F="$f" '/^func Test.*\(t \*testing\.T\)/{if(n&&!a)print F":"s":"n; n=$2; s=NR; a=0} /t\.(Error|Fatal|Fail|Skip)|require\.|assert\./{a=1} END{if(n&&!a)print F":"s":"n}'; done`
- The files say so themselves in their header comments, and the bodies confirm it: an API error is logged and returned from, not failed on.
  - `internal/integration/dh_search_diag_test.go:66`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/integration/dh_search_diag_test.go | sed -n '1,2p;62,70p'`
- These are not developer-only scratch files. The scheduled integration job runs the whole ./internal/integration/... package with real credentials, and commit 176871e5 deliberately made that job fail loudly rather than skip — so these seven now consume paid DH Enterprise calls on a schedule while being structurally incapable of reporting a regression.
  - `.github/workflows/test.yml:148`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:.github/workflows/test.yml | sed -n '87,150p'`
- TestDHSearchDiag_BasicQueries alone fans out to 15 sub-cases, each a live SearchCards call; the four dh_match_search functions add more. The wall-clock and quota cost is real and recurring.
  - `internal/integration/dh_search_diag_test.go:20`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/integration/dh_search_diag_test.go | sed -n '20,50p'`

**Proposed fix:** Either promote them to real tests or take them out of the scheduled run. Promotion means giving each sub-case a concrete expectation (e.g. 'searching "Charizard" in Pokemon Scarlet & Violet 151 returns at least one result whose Number is 199') and turning the `t.Logf("ERROR: %v"); return` swallows into t.Fatalf. Removal means deleting the two files, or moving them behind a separate build tag (e.g. //go:build dhdiag) so `-tags integration` no longer collects them and a developer opts in explicitly. The header comments already document a -run invocation, which is the shape of an opt-in tool rather than a suite member.

**Blast radius:**

- `internal/integration/dh_search_diag_test.go`
- `internal/integration/dh_match_search_test.go`
- `.github/workflows/test.yml`
- `CLAUDE.md (Testing section, integration-tests description)`

**Acceptance criteria:**

- [ ] If promoted: injecting a wrong expectation into any one of the seven makes `go test -tags integration -run <Name> ./internal/integration/` fail. Today it passes regardless.
- [ ] If separated: `go test -tags integration ./internal/integration/... -v` no longer lists any TestDHSearchDiag_* or TestDHMatchSearch_* case, and the scheduled workflow's runtime drops accordingly.
- [ ] No remaining function under internal/integration/ both makes a live API call and has no assertion — verifiable by re-running the scan command in this finding's first evidence entry.

### FU-08 — Unskip the two ErrorBoundary tests

**Severity:** low · **Effort:** S · **Rolls up:** DCT-006

> **Controller note.** Unconditionally it.skip'd, leaving the dev-vs-prod error-detail branch untested.

#### DCT-006 — Two ErrorBoundary tests are unconditionally it.skip'd, leaving the dev-vs-prod error-detail branch untested

*Lens:* `docs-config-tests` · *Confidence:* `strong` · *Verdict:* `confirmed` · *Severity:* low

**Subject:** `{'kind': 'test', 'identity': 'web/tests/components/ErrorBoundary.test.jsx:327 and :337'}`

**Verifier correction:** Nothing material, but a note for the fixer that the finding does not mention: the two skipped tests have identical bodies (render <ErrorBoundary><BrokenComponent/></ErrorBoundary>) with opposite expectations, so under any single build mode at most one of them can pass as written. Re-enabling them requires rewriting, not just un-skipping — which makes the filed acceptance criterion 'inverting the ErrorBoundary's dev-mode condition makes at least one of the two tests fail' weaker than it reads. Ticketable, but the fixer must rewrite both bodies.

**Evidence:**

- Both tests are disabled with a bare it.skip — no env gate, no condition. They are the only unconditional skips in the frontend suite; every other skip in web/tests is an env-gated Playwright guard.
  - `web/tests/components/ErrorBoundary.test.jsx:327`
  - Reproduce: `git grep -nE '\b(it|test|describe)\.(skip|todo)\b|\bxit\(' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- web/src web/tests`
- The reason given is a real constraint, but a solvable one: Vitest supports vi.stubEnv for import.meta.env, so the stated blocker no longer holds as written.
  - `web/tests/components/ErrorBoundary.test.jsx:325`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:web/tests/components/ErrorBoundary.test.jsx | sed -n '324,346p'`

**Proposed fix:** Re-enable both tests using vi.stubEnv('DEV', true/false) (or split the branch into a prop/injected flag the test can set), or delete them along with the describe block. A skipped test with a stale rationale is worse than no test: it reads as coverage in the file and provides none.

**Blast radius:**

- `web/tests/components/ErrorBoundary.test.jsx`
- `web/src/react/components/ErrorBoundary (the component under test)`

**Acceptance criteria:**

- [ ] `npm test` in web/ reports zero skipped tests in ErrorBoundary.test.jsx.
- [ ] If re-enabled: inverting the ErrorBoundary's dev-mode condition makes at least one of the two tests fail.
- [ ] If deleted: `git grep -n 'it.skip' web/tests/components/` returns empty.


---

## Tier: Dead code

### FU-09 — Remove the orphaned llmutil package

**Severity:** low · **Effort:** S · **Rolls up:** DCG-001

> **Controller note.** ADJUDICATED — NAM-005 is the same subject reported by the naming lens as a pointer and was verified a duplicate (`ticketable: false`). File once, here. StripMarkdownFences has no consumer outside its own test; the package survives the Azure AI / advisor removal.

#### DCG-001 — internal/domain/llmutil is a whole orphaned package: its only exported function StripMarkdownFences has no consumer outside its own test

*Lens:* `dead-code-go` · *Confidence:* `mechanical` · *Verdict:* `confirmed` · *Severity:* low

**Subject:** `{'kind': 'package', 'identity': 'internal/domain/llmutil'}`

**Evidence:**

- The package contains exactly one non-test file declaring exactly one exported symbol, StripMarkdownFences.
  - `internal/domain/llmutil/strip_fences.go:7`
  - Reproduce: `git ls-tree -r --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/domain/llmutil`
- The only import of the package anywhere in the Go tree is its own in-package external test file; there is no other importer.
  - Reproduce: `git grep -nE '\bllmutil\b' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- '*.go'`
- The symbol name itself, searched repo-wide with the defining package excluded, has zero hits — so it is not reached by a dot-import, an alias, or a same-name declaration elsewhere.
  - Reproduce: `git grep -nE '\bStripMarkdownFences\b' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- '*.go' ':!internal/domain/llmutil'`
- There is no LLM consumer left in the Go tree for an LLM-response helper to serve: the sole case-insensitive advisor/azure hit at the baseline is a prose comment about a database index advisor.
  - Reproduce: `git grep -nEi '\b(advisor|azure)\b' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- '*.go'`

**Proposed fix:** Delete internal/domain/llmutil/ entirely (strip_fences.go and strip_fences_test.go). It is a helper for parsing LLM responses in a tree that no longer has an LLM caller.

**Blast radius:**

- `internal/domain/llmutil/strip_fences.go`
- `internal/domain/llmutil/strip_fences_test.go`

**Acceptance criteria:**

- [ ] `go build ./...` succeeds with internal/domain/llmutil/ removed.
- [ ] `go test -race ./...` passes with the directory removed.
- [ ] `go vet ./...` reports no unresolved import of github.com/guarzo/slabledger/internal/domain/llmutil.
- [ ] `make check` passes (the package count in the architecture check drops by one; no import-rule violation is introduced since nothing imported it).

### FU-10 — Remove the orphaned root testutil package

**Severity:** low · **Effort:** S · **Rolls up:** DCG-002

> **Controller note.** Kept separate from the mocks cleanup below: different package, and it carries a same-name-collision caveat worth preserving in the ticket — the one testutil.* selector in the tree resolves to prometheus/testutil, not this package, which is exactly LENS-BRIEF trap 2 running in the direction that makes dead code look live.

#### DCG-002 — The root internal/testutil package is orphaned — GetTestToken has no caller, and the one testutil.* selector in the tree resolves to prometheus/testutil, not this package

*Lens:* `dead-code-go` · *Confidence:* `mechanical` · *Verdict:* `confirmed` · *Severity:* low

**Subject:** `{'kind': 'package', 'identity': 'internal/testutil (root package; sole symbol GetTestToken)'}`

**Evidence:**

- The root package consists of a single file declaring a single exported function, GetTestToken. (internal/testutil/mocks/ is a separate package and is not part of this subject.)
  - `internal/testutil/config.go:26`
  - Reproduce: `git ls-tree -r --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/testutil | grep -v '^internal/testutil/mocks/'`
- GetTestToken has no call site anywhere in the tree — the only three hits are its own doc comment, its own doc-comment example, and its own declaration.
  - Reproduce: `git grep -nE '\bGetTestToken\b' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- '*.go'`
- Same-name collision check (LENS-BRIEF trap 2): the single `testutil.` selector in the repo is prometheus's testutil.GatherAndCount, not this package. That file imports github.com/prometheus/client_golang/prometheus/testutil.
  - `internal/adapters/storage/postgres/metrics_test.go:30`
  - Reproduce: `git grep -nE '\btestutil\.' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- '*.go'`

**Proposed fix:** Delete internal/testutil/config.go, removing the root testutil package. Tests needing an env-var-with-default should use os.Getenv directly at the call site, as the rest of the suite already does.

**Blast radius:**

- `internal/testutil/config.go`
- `internal/testutil/mocks/ remains untouched — it is a separate package with live consumers`

**Acceptance criteria:**

- [ ] `go build ./...` succeeds with internal/testutil/config.go removed.
- [ ] `go test -race ./...` passes with the file removed.
- [ ] `go test -tags integration ./internal/integration/...` still compiles with the file removed.
- [ ] `make check` passes.

### FU-11 — Remove or wire the cardutil normalization-trace facility

**Severity:** low · **Effort:** S · **Rolls up:** DCG-003

> **Controller note.** No production code calls ContextWithTrace or AddStep, so the trace can never be non-empty even if something read it.

#### DCG-003 — The cardutil normalization-trace facility is never wired: no production code calls ContextWithTrace or AddStep, so the trace can never be non-empty even if it were plumbed

*Lens:* `dead-code-go` · *Confidence:* `mechanical` · *Verdict:* `confirmed` · *Severity:* low

**Subject:** `{'kind': 'symbol', 'identity': 'internal/platform/cardutil/trace.go — NormalizationTrace, NormalizationStep, AddStep, ContextWithTrace, TraceFromContext, LogNormalizationTrace, traceKey, traceKeyType'}`

**Evidence:**

- All seven trace identifiers are referenced only inside trace.go (their own declarations and internal use) and trace_test.go. No cardutil normalization function calls AddStep, so a trace obtained via ContextWithTrace would always have zero Steps.
  - `internal/platform/cardutil/trace.go:11`
  - Reproduce: `git grep -nE '\b(NormalizationTrace|NormalizationStep|ContextWithTrace|TraceFromContext|LogNormalizationTrace|AddStep|traceKey)\b' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- '*.go'`
- The Go reference map holds seven matching records — the six exported trace symbols plus the package's own TestNormalizationTrace — and every one reports external_refs 0 with name_ambiguous false, so the zero is a measurement rather than an upper bound in this case.
  - Reproduce: `jq -r '.records[] | select(.identity|test("NormalizationTrace|NormalizationStep|ContextWithTrace|TraceFromContext|LogNormalizationTrace")) | [.identity,.external_refs,.name_ambiguous]|@tsv' docs/audit/runs/2026-08-08/maps/go-reference-map.json`

**Proposed fix:** Delete internal/platform/cardutil/trace.go and internal/platform/cardutil/trace_test.go. If normalization tracing is wanted later, it should be reintroduced together with AddStep calls in the normalization functions — without those, the facility is inert by construction.

**Blast radius:**

- `internal/platform/cardutil/trace.go`
- `internal/platform/cardutil/trace_test.go`

**Acceptance criteria:**

- [ ] `go build ./...` succeeds with both files removed.
- [ ] `go test -race ./...` passes with both files removed.
- [ ] `go test -race ./internal/platform/cardutil/...` passes — the remaining normalization tests are unaffected.
- [ ] `make check` passes.

### FU-12 — Remove the orphaned mock surface in internal/testutil/mocks

**Severity:** low · **Effort:** M · **Rolls up:** DCG-004, DCG-005

> **Controller note.** Two mechanical removals in one tree, reviewable as one PR. DCG-004 is a legacy option/HTTP-client/price-provider framework whose only consumer is its own test file. DCG-005 is fourteen per-interface mocks with no consumer in any package, plus two comments elsewhere that falsely claim other packages use them. DCG-005 carries a deliberate exclusion for DHPushStatusCall, which has zero external refs but is the element type of a field in a live type and is therefore required to compile — the exclusion is correct and must not be folded back in.

#### DCG-004 — A legacy option/HTTP-client/price-provider mock framework in internal/testutil/mocks has no consumer outside its own test file — the purest form of the in-package-caller trap

*Lens:* `dead-code-go` · *Confidence:* `mechanical` · *Verdict:* `confirmed` · *Severity:* low

**Subject:** `{'kind': 'symbol', 'identity': 'internal/testutil/mocks — MockBehavior/MockOption and the eight With* option constructors (common.go), the MockHTTPClient framework (http_client.go, http_client_helpers.go, http_client_test.go), and MockPriceProvider (price_provider.go)'}`

**Verifier correction:** Nothing that affects the verdict. One completeness gap in the acceptance criteria: internal/testutil/mocks/README.md:184 documents `MockBehavior` / `MockOption` as living in common.go, and no acceptance criterion restricts its grep to anything but '*.go', so a fixer who satisfies the criteria literally will leave that README line describing deleted code.

**Evidence:**

- None of the framework's exported symbols is named anywhere outside the mocks package. The single hit is a prose comment in httpx that mentions MockHTTPClient without using it.
  - `internal/adapters/clients/httpx/interface.go:14`
  - Reproduce: `git grep -nE '\b(MockBehavior|MockOption|WithFailAfterN|WithEmptyData|WithDataVariant|WithReturnAllCards|MockHTTPClient|NewMockHTTPClient|MockHTTPCall|MockHTTPResponse|MockHTTPStats|MockPriceProvider|NewMockPriceProvider)\b' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- '*.go' ':!internal/testutil/mocks'`
- The only consumer of the MockHTTPClient framework is its own in-package test file, internal/testutil/mocks/http_client_test.go, whose TestMockHTTPClient_* functions exercise the mock rather than any production code. The framework is internally busy and externally dead.
  - Reproduce: `git ls-tree -r --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/testutil/mocks | grep http_client`
- Same-name collision (LENS-BRIEF trap 2) on the shared option name WithTimeout: the Go map scores mocks.WithTimeout at external_refs 41 with name_ambiguous false, but every one of those 41 hits is context.WithTimeout from the standard library. The map's ambiguity flag only sees duplicate declarations inside the repo, so a stdlib collision is invisible to it.
  - Reproduce: `git grep -nE '\bWithTimeout\b' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- '*.go' ':!internal/testutil/mocks'`

**Proposed fix:** Delete internal/testutil/mocks/common.go, http_client.go, http_client_helpers.go, http_client_test.go, and price_provider.go. Update internal/testutil/mocks/README.md and the stale comment at internal/adapters/clients/httpx/interface.go:14 so neither advertises a mock that no longer exists.

**Blast radius:**

- `internal/testutil/mocks/common.go`
- `internal/testutil/mocks/http_client.go`
- `internal/testutil/mocks/http_client_helpers.go`
- `internal/testutil/mocks/http_client_test.go`
- `internal/testutil/mocks/price_provider.go`
- `internal/testutil/mocks/README.md (documentation update)`
- `internal/adapters/clients/httpx/interface.go (comment update only, no code change)`

**Acceptance criteria:**

- [ ] `go build ./...` succeeds with the five files removed.
- [ ] `go test -race ./...` passes with the five files removed — in particular no package outside internal/testutil/mocks fails to compile.
- [ ] `go test -tags integration ./internal/integration/...` still compiles.
- [ ] `git grep -nE '\b(MockOption|MockHTTPClient|MockPriceProvider)\b' -- '*.go'` returns no hits after the change.
- [ ] `make check` passes.

#### DCG-005 — Fourteen per-interface mocks in internal/testutil/mocks have no consumer in any package, and two comments elsewhere falsely claim they are used by other packages

*Lens:* `dead-code-go` · *Confidence:* `mechanical` · *Verdict:* `confirmed` · *Severity:* low

**Subject:** `{'kind': 'symbol', 'identity': 'internal/testutil/mocks — AnalyticsRepositoryMock, DHCardTombstoneRepoMock, DHRepositoryMock, MockAPITracker, MockAnalyticsService, MockBatchPricer, MockCompReader, MockPriceLookup, MockPriceWriter, MockPurchaseLister, MockTrajectoryRepository, PricingRepositoryMock, ResolverMock, SaleRepositoryMock'}`

**Verifier correction:** Nothing that affects the verdict. Same completeness gap as DCG-004: internal/testutil/mocks/README.md documents four of the fourteen (SaleRepositoryMock:52, AnalyticsRepositoryMock:58, PricingRepositoryMock:60, DHRepositoryMock:61) and every acceptance criterion is scoped to '*.go', so satisfying them literally leaves the README describing removed mocks. The two inaccurate comments are handled — acceptance criterion 4's grep is over '*.go' and would catch both.

**Evidence:**

- None of the fourteen mock type names is used outside the mocks package. The only two hits are comment lines, and both assert the opposite of the truth — they tell a reader that the shared mock exists 'for use by other packages' / 'is used by other packages' when no such package exists.
  - `internal/domain/inventory/service_test.go:229`
  - Reproduce: `git grep -nE '\b(AnalyticsRepositoryMock|DHCardTombstoneRepoMock|DHRepositoryMock|MockAPITracker|MockAnalyticsService|MockBatchPricer|MockCompReader|MockPriceLookup|MockPriceWriter|MockPurchaseLister|MockTrajectoryRepository|PricingRepositoryMock|ResolverMock|SaleRepositoryMock)\b' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- '*.go' ':!internal/testutil/mocks'`
- Within the mocks package itself, every reference to the fourteen names is its own declaration, its own method receiver, or its own `var _ Iface = (*T)(nil)` assertion. None is used as a field type of another mock, so no live type depends on them for compilation.
  - Reproduce: `git grep -nE '\b(AnalyticsRepositoryMock|DHCardTombstoneRepoMock|DHRepositoryMock|MockAPITracker|MockAnalyticsService|MockBatchPricer|MockCompReader|MockPriceLookup|MockPriceWriter|MockPurchaseLister|MockTrajectoryRepository|PricingRepositoryMock|ResolverMock|SaleRepositoryMock)\b' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- 'internal/testutil/mocks/*.go'`
- The one member of the set with a constructor, MockTrajectoryRepository, is not reachable through it either — NewMockTrajectoryRepository has no external caller. The other thirteen are struct-literal-constructed, so a caller would have to name the type, and evidence 1 shows none does.
  - Reproduce: `git grep -nE '\bNewMockTrajectoryRepository\b' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- '*.go' ':!internal/testutil/mocks'`

**Proposed fix:** Delete the fourteen orphaned mocks and their declaring files where a file contains nothing else (api_tracker.go, batch_pricer.go, campaign_service_analytics.go, dh_card_tombstone_repo.go, inventory_analytics_repo.go, inventory_dh_repo.go, inventory_pricing_repo.go, inventory_sale_repo.go, price_lookup.go, psa_resolver.go, trajectory_repository.go), trimming liquidation_service.go to the mocks that remain live. Then correct the two comments that claim the deleted mocks are used by other packages, and update internal/testutil/mocks/README.md. Keep DHPushStatusCall, CapturingLogger, and MockSimplePriceProvider.

**Blast radius:**

- `internal/testutil/mocks/api_tracker.go`
- `internal/testutil/mocks/batch_pricer.go`
- `internal/testutil/mocks/campaign_service_analytics.go`
- `internal/testutil/mocks/dh_card_tombstone_repo.go`
- `internal/testutil/mocks/inventory_analytics_repo.go`
- `internal/testutil/mocks/inventory_dh_repo.go`
- `internal/testutil/mocks/inventory_pricing_repo.go`
- `internal/testutil/mocks/inventory_sale_repo.go`
- `internal/testutil/mocks/price_lookup.go`
- `internal/testutil/mocks/psa_resolver.go`
- `internal/testutil/mocks/trajectory_repository.go`
- `internal/testutil/mocks/liquidation_service.go (partial — MockPurchaseLister, MockCompReader, MockPriceWriter only)`
- `internal/testutil/mocks/README.md (documentation update)`
- `internal/domain/inventory/service_test.go:229 (comment correction)`
- `internal/domain/psacampaign/mapper_test.go:14 (comment correction)`

**Acceptance criteria:**

- [ ] `go build ./...` succeeds after the removals.
- [ ] `go test -race ./...` passes after the removals, with no package failing to compile for a missing mock.
- [ ] `go test -tags integration ./internal/integration/...` still compiles.
- [ ] `git grep -nE '\b(AnalyticsRepositoryMock|DHCardTombstoneRepoMock|DHRepositoryMock|MockAPITracker|MockAnalyticsService|MockBatchPricer|MockCompReader|MockPriceLookup|MockPriceWriter|MockPurchaseLister|MockTrajectoryRepository|PricingRepositoryMock|ResolverMock|SaleRepositoryMock)\b' -- '*.go'` returns no hits after the change.
- [ ] `git grep -n 'DHPushStatusCall' -- internal/testutil/mocks` still returns its declaration and its use as a field type — the exclusion was honoured.
- [ ] `make check` passes.

### FU-13 — Remove the advisor UI cluster and its two runtime dependencies

**Severity:** medium · **Effort:** M · **Rolls up:** FE-001

> **Controller note.** Kept separate from the other frontend deletions because it also drops react-markdown and remark-gfm from the dependency set, which is a different review risk. Its only consumers are its own two test files. Survives commit e46370e5 ('fully remove the Azure AI / advisor feature'); corroborated independently by the frontend map and the db map in Phase 1.

#### FE-001 — Advisor UI cluster survives the Azure AI removal; its only consumers are its own two test files, and it is the sole importer of the react-markdown and remark-gfm runtime dependencies

*Lens:* `frontend-health` · *Confidence:* `mechanical` · *Verdict:* `confirmed` · *Severity:* medium

**Subject:** `{'kind': 'component', 'identity': 'web/src/react/components/advisor/ (SectionedReport.tsx, SectionedReport.test.tsx, splitByH2.ts)'}`

**Verifier correction:** Nothing material. One piece of context the finding does not mention and the controller should have: .superpowers/sdd/2026-08-08-remove-azure-ai/progress.md:115-117 records that the Azure-removal author saw this cluster, judged it out of that change's scope, and deliberately left it. So this is a known deferral rather than an oversight — which strengthens rather than weakens the ticket, but the ticket should say so. Note also that because nothing in the app import graph reaches the cluster, it is not in the shipped bundle; the concrete cost is two production dependencies installed and audited for no reason, three dead source files, and a dead test suite in two copies (.tsx under web/src and .jsx under web/tests).

**Evidence:**

- The advisor directory holds exactly three tracked files at the baseline.
  - `web/src/react/components/advisor/SectionedReport.tsx:58`
  - Reproduce: `git ls-tree -r --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- web/src/react/components/advisor`
- Nothing outside the advisor directory references SectionedReport, splitByH2, or the directory path, except one test file that lives outside web/src and is therefore invisible to the frontend map.
  - `web/tests/components/SectionedReport.test.jsx:10`
  - Reproduce: `git grep -nE 'SectionedReport|splitByH2|components/advisor' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- . ':!docs/audit' | grep -v 'web/src/react/components/advisor/'`
- No non-TS entry point (index.html, package.json scripts, vite config, playwright config) reaches the cluster; the single hit under web/tests is the .jsx test named above.
  - Reproduce: `git grep -cnE 'SectionedReport|splitByH2|advisor' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- web/index.html web/package.json web/vite.config.js web/playwright.config.ts web/tests`
- The frontend reference map independently scores SectionedReport test_only with all four external refs inside its own sibling test, and scores splitByH2 as referenced only by that test plus SectionedReport.tsx itself.
  - Reproduce: `jq -r '.records[] | select(.defined_at|test("advisor|splitByH2")) | "\(.identity) refs=\(.external_refs) test_only=\(.test_only // false) sites=\(.ref_sites|join(\",\"))"' docs/audit/runs/2026-08-08/maps/frontend-reference-map.json`
- SectionedReport.tsx is the only importer in the repository of the react-markdown and remark-gfm production dependencies declared in web/package.json.
  - `web/package.json:79`
  - Reproduce: `git grep -nE "react-markdown|remark-gfm" 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- web/package.json web/src web/tests`
- The feature this component rendered was removed on both the Go and database sides: commit e46370e5 removed the Azure AI / advisor feature and two migrations dropped its tables. No Go package named advisor, ai, or scoring exists at the baseline.
  - Reproduce: `git ls-tree -r --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 | grep -E 'internal/domain/(advisor|ai|scoring)'`

**Proposed fix:** Delete web/src/react/components/advisor/ (all three files) and web/tests/components/SectionedReport.test.jsx, then drop react-markdown and remark-gfm from web/package.json dependencies and refresh web/package-lock.json.

**Blast radius:**

- `web/src/react/components/advisor/SectionedReport.tsx`
- `web/src/react/components/advisor/SectionedReport.test.tsx`
- `web/src/react/components/advisor/splitByH2.ts`
- `web/tests/components/SectionedReport.test.jsx`
- `web/package.json`
- `web/package-lock.json`

**Acceptance criteria:**

- [ ] `cd web && npm run build` succeeds with the advisor directory and both test files removed.
- [ ] `cd web && npm test` passes with no remaining reference to SectionedReport or splitByH2.
- [ ] `git grep -nE 'SectionedReport|splitByH2|components/advisor' -- web` returns no output.
- [ ] `git grep -nE 'react-markdown|remark-gfm' -- web/package.json web/src web/tests` returns no output.
- [ ] `make screenshots` renders every page unchanged (no page rendered advisor output before the change).

### FU-14 — Delete the two orphaned frontend artifacts

**Severity:** low · **Effort:** S · **Rolls up:** FE-002, FE-005

> **Controller note.** Both are plain deletions under web/. FE-002: scoring.ts is the only fully orphaned module under web/src — no importer anywhere, and no Go scoring type it could mirror. Verifier lowered it to low because four of its five declarations are TypeScript types erased at compile time and the fifth is unreachable from the entry graph, so there is no bundle or runtime cost — the ticket must not imply one. FE-005: a 51-file Vitest coverage report checked into git, never regenerated since the initial commit, not gitignored, reporting on 31 files that no longer exist.

#### FE-002 — web/src/types/scoring.ts is the only fully orphaned module under web/src: no importer anywhere in the repository, and no Go scoring type it could be mirroring

*Lens:* `frontend-health` · *Confidence:* `mechanical` · *Verdict:* `confirmed_lower_severity` · *Severity:* low

**Subject:** `{'kind': 'type', 'identity': 'web/src/types/scoring.ts (Verdict, Factor, DataGap, ScoreCard, FACTOR_DISPLAY_NAMES)'}`

**Verifier correction:** The defect is real; the severity is not. Four of the five declarations are TypeScript types, which are erased at compile time, and the fifth (FACTOR_DISPLAY_NAMES) is unreachable from the entry graph, so Vite never bundles the module. There is no runtime cost, no bundle weight, and no behavior at stake — the entire consequence is that a reader may believe a scoring contract exists. That is a low, not a medium. Deletion is still correct and the acceptance criteria prove it cleanly.

**Evidence:**

- Grouping every non-test frontend-map record by its defining file, scoring.ts is the only module whose every declaration has zero external references and zero registration sites. (main.tsx also appears, and is the excluded case: it is the Vite entry point reached from web/index.html, a non-TS consumer the map cannot see.)
  - `web/src/types/scoring.ts:1`
  - Reproduce: `jq -r '[.records[] | select((.defined_at|test("\\.test\\."))|not)] | group_by(.defined_at|sub(":[0-9]+$";"")) | map(select(all(.[]; .external_refs==0 and ((.registration_sites|length)==0)))) | map({file:(.[0].defined_at|sub(":[0-9]+$";"")), syms:[.[]|.identity]}) | .[] | "\(.file)\t\(.syms|join(","))"' docs/audit/runs/2026-08-08/maps/frontend-reference-map.json`
- The word 'scoring' appears nowhere in web/ outside the lockfile and the checked-in coverage artifact, so no import specifier resolves to this module.
  - Reproduce: `git grep -nE "scoring" 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- web ':!web/coverage' ':!web/package-lock.json'`
- The declared symbols are referenced only from within scoring.ts itself, across web, internal, and cmd.
  - Reproduce: `git grep -nE '\b(ScoreCard|FACTOR_DISPLAY_NAMES|DataGap|types/scoring)\b' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- web internal cmd`
- There is no Go counterpart for this contract: no internal/domain/scoring package exists, and the file's snake_case field names (entity_id, engine_verdict, data_gaps, scored_at) match no Go JSON tag in the tree. It is a mirror of a removed backend, consistent with the same Azure AI / scoring removal that stranded FE-001.
  - Reproduce: `git ls-tree -r --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 | grep -E 'internal/domain/(advisor|ai|scoring)'`

**Proposed fix:** Delete web/src/types/scoring.ts.

**Blast radius:**

- `web/src/types/scoring.ts`

**Acceptance criteria:**

- [ ] `cd web && npm run build` succeeds with the file removed.
- [ ] `cd web && npx tsc --noEmit` reports no new errors.
- [ ] `cd web && npm test` passes.
- [ ] `git grep -n 'scoring' -- web/src` returns no output.

#### FE-005 — A 51-file Vitest coverage report is checked into git, never regenerated since the initial commit, not gitignored, and reports on 31 frontend files that no longer exist

*Lens:* `frontend-health` · *Confidence:* `strong` · *Verdict:* `confirmed` · *Severity:* low

**Subject:** `{'kind': 'component', 'identity': 'web/coverage/ (51 git-tracked files)'}`

**Verifier correction:** One arithmetic conflation, immaterial to the conclusion. The report contains 35 source-file pages, of which 28 no longer exist and 7 still do. The finding's '31 of the 38' reaches its numbers by counting the report's own scaffolding assets — block-navigation.js, prettify.js, sorter.js — as covered source files. The fixer should ignore the specific counts and act on the four verified legs.

**Evidence:**

- 51 files under web/coverage/ are tracked in git at the baseline.
  - `web/coverage/index.html`
  - Reproduce: `git ls-tree -r --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- web/coverage | wc -l`
- The directory was committed once, in the repository's initial commit on 2026-03-25, and has never been touched since — so it reflects a codebase state over four months stale at this baseline.
  - Reproduce: `git log -1 --format='%h %ad %s' --date=short 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- web/coverage`
- 31 of the 38 source files the report covers no longer exist anywhere under web/src, including whole removed pages (CollectionPage, OpportunitiesPage, PricingPage) and removed contexts (AppStateContext, ThemeContext, UserPreferencesContext).
  - Reproduce: `git ls-tree -r --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- web/coverage | sed 's|.*/||; s|\.html$||' | grep -E '\.(tsx|ts|jsx|js)$' | sort > /tmp/cov.txt; git ls-tree -r --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- web/src | sed 's|.*/||' | sort -u > /tmp/src.txt; comm -23 /tmp/cov.txt /tmp/src.txt`
- Neither the root .gitignore nor a web/.gitignore excludes the JS coverage directory; the only 'coverage' mention in .gitignore is a comment about the Go coverage tool, so `npm run test -- --coverage` will keep re-dirtying the working tree with a 51-file diff.
  - `.gitignore:11`
  - Reproduce: `git grep -n 'coverage' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- .gitignore web/.gitignore`

**Proposed fix:** Delete web/coverage/ from git and add `coverage/` (or `web/coverage/`) to .gitignore.

**Blast radius:**

- `web/coverage/ (51 files)`
- `.gitignore`

**Acceptance criteria:**

- [ ] `git ls-files web/coverage | wc -l` returns 0.
- [ ] `cd web && npm test -- --coverage` regenerates the directory and `git status --porcelain` stays clean.
- [ ] `cd web && npm run build` and `npm test` both pass (nothing imports from web/coverage).

### FU-15 — Remove the domain/storage package, which is now only a doc.go for a deleted interface

**Severity:** low · **Effort:** S · **Rolls up:** NAM-003

> **Controller note.** The package consists solely of a doc.go documenting a Cache interface, with usage examples, that no longer exists anywhere in the repo.

#### NAM-003 — Package internal/domain/storage consists solely of a doc.go documenting a `Cache` interface, with usage examples, that no longer exists anywhere in the repo

*Lens:* `naming-and-boundaries` · *Confidence:* `strong` · *Verdict:* `confirmed` · *Severity:* low

**Subject:** `{'kind': 'package', 'identity': 'internal/domain/storage'}`

**Verifier correction:** Acceptance criterion 3 is not satisfiable as written. It requires `git grep -n 'domain/storage'` to return nothing outside migration comments and audit run directories, but the string also appears at .claude/skills/polish-all/SKILL.md:45, docs/polish-progress.json:7, and docs/specs/2026-08-07-codebase-tech-debt-audit-design.md:118 — none of which are audit run directories. The controller should widen that criterion to the CLAUDE.md line plus the package path itself, or a fixer will read the ticket as unfinishable. The substantive claim is unaffected.

**Evidence:**

- The package contains exactly one file, doc.go.
  - `internal/domain/storage/doc.go`
  - Reproduce: `git ls-tree -r --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/domain/storage`
- The package declares nothing — no type, func, var, or const. Absence established by the command's empty output and exit status 1, not by a file:line citation.
  - `internal/domain/storage/doc.go`
  - Reproduce: `git grep -cE '^(type|func|var|const)' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/domain/storage; echo "exit=$?"`
- The doc comment asserts the package defines interfaces and names Cache as the core one, with a two-call usage example.
  - `internal/domain/storage/doc.go:1-12`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/domain/storage/doc.go | head -12`
- No `Cache` interface exists anywhere in the repo at this revision. Empty output is the evidence.
  - Reproduce: `git grep -nE 'type Cache\b|Cache interface' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- '*.go'; echo "exit=$?"`
- No Go file imports the package.
  - Reproduce: `git grep -nE '"github.com/guarzo/slabledger/internal/domain/storage"' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6; echo "exit=$?"`
- CLAUDE.md — the orientation file loaded into every session — repeats the false claim, so the misnaming propagates to every reader before they open a file.
  - `CLAUDE.md:58`
  - Reproduce: `git grep -n 'domain/storage' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- CLAUDE.md`

**Proposed fix:** Delete internal/domain/storage/ and the CLAUDE.md line that describes it. Because the package declares nothing and has zero importers, removal cannot break a build — but the delete/keep call belongs to the dead-code-go lens; if that lens argues for keeping the package as a future home for persistence interfaces, the minimum naming fix is to rewrite doc.go so it stops naming a `Cache` interface and stops showing usage examples for a type that does not exist.

**Blast radius:**

- `internal/domain/storage/doc.go`
- `CLAUDE.md:58`
- `internal/README.md (if it repeats the claim)`
- `docs/ARCHITECTURE.md (domain interface list, if it repeats the claim)`

**Acceptance criteria:**

- [ ] `go build ./...` and `go test -race ./...` pass with the package removed.
- [ ] `make check` passes (the architecture import checker derives package membership from the tree, so a removed package must not break it).
- [ ] `git grep -n 'domain/storage'` returns no results outside historical migration comments and audit run directories.
- [ ] If the package is kept instead, `git grep -n 'Cache' internal/domain/storage/` returns no results.

### FU-16 — Drop the three api_rate_limits call-counter columns that read as live telemetry

**Severity:** medium · **Effort:** S · **Rolls up:** DB-002

> **Controller note.** Never written, never read; they sit at DEFAULT 0 forever while presenting as rate-limit telemetry. Distinct from the api_usage_summary view's same-named computed column, which aggregates a different table — LENS-BRIEF trap 2, checked and cleared by the verifier.

#### DB-002 — The three call-counter columns on api_rate_limits are never written and never read; they sit at their DEFAULT 0 forever and read as live telemetry

*Lens:* `db-schema` · *Confidence:* `strong` · *Verdict:* `confirmed` · *Severity:* medium

**Subject:** `{'kind': 'column', 'identity': 'api_rate_limits.calls_last_minute, api_rate_limits.calls_last_hour, api_rate_limits.calls_last_day'}`

**Evidence:**

- The db-map scores calls_last_minute and calls_last_day at zero external refs, unambiguous. calls_last_hour scores 2 refs, also unambiguous by name — the collision is by table, not by name.
  - Reproduce: `jq -r '.records[] | select(.identity|test("^api_rate_limits\\.calls_")) | "\(.identity) refs=\(.external_refs) ambiguous=\(.name_ambiguous//false) sites=\(.ref_sites|join(\" \"))"' docs/audit/runs/2026-08-08/maps/db-map.json`
- Neither of calls_last_hour's two hits is a reference to the api_rate_limits column. api_tracker.go:52 selects calls_last_hour FROM the api_usage_summary VIEW, which computes its own column of that name over api_calls; price_refresh.go:131 is a structured-log key string.
  - `internal/adapters/storage/postgres/api_tracker.go:52`
  - Reproduce: `git grep -nE '\bcalls_last_hour\b' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal web/src`
- api_tracker.go:44-57 shows the SELECT list containing calls_last_hour reads FROM api_usage_summary, not FROM api_rate_limits.
  - `internal/adapters/storage/postgres/api_tracker.go:52`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/adapters/storage/postgres/api_tracker.go | sed -n '42,57p'`
- The only two statements in the repository that touch api_rate_limits name four columns between them — provider, blocked_until, last_429_at, updated_at — and none of the three counters. So the counters are never written either.
  - `internal/adapters/storage/postgres/api_tracker.go:107`
  - Reproduce: `git grep -n 'api_rate_limits' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal web/src ':(exclude)internal/adapters/storage/postgres/migrations'`
- No dynamically-assembled SQL exists in the postgres adapter, so an explicit column list is the only channel by which a column name reaches the database.
  - Reproduce: `git grep -nE 'Sprintf\(.*(SELECT|INSERT|UPDATE|FROM|WHERE)' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/adapters/storage/postgres`

**Proposed fix:** Drop calls_last_minute, calls_last_hour and calls_last_day from api_rate_limits in a new migration, with a .down.sql restoring them as BIGINT DEFAULT 0. Per-window call counts are already derived correctly by the api_usage_summary view over api_calls, which is what the code actually reads — the table columns are a second, never-populated copy of the same idea.

**Blast radius:**

- `internal/adapters/storage/postgres/api_tracker.go — reads api_usage_summary, not these columns; must remain unchanged`
- `internal/adapters/storage/postgres/migrations/ — one new up/down pair`
- `docs/SCHEMA.md — api_rate_limits column list`

**Acceptance criteria:**

- [ ] `go build ./...` and `go test -race ./...` pass with the three columns dropped.
- [ ] DBTracker.GetAPIUsage still returns a non-zero CallsLastHour for a provider with recent api_calls rows (it reads the view, not the table).
- [ ] The new .down.sql restores all three columns; up-then-down-then-up is idempotent.
- [ ] docs/SCHEMA.md's api_rate_limits entry lists only provider, last_429_at, blocked_until, updated_at.

### FU-17 — Drop the four views with no consumer

**Severity:** low · **Effort:** S · **Rolls up:** DB-003

> **Controller note.** Four of the five surviving views have no consumer anywhere, yet migration 000028 still carries them as REVOKE targets — the exact condition 000035 cited when it removed the ai_usage_* views, so there is precedent in-tree for the fix.

#### DB-003 — Four of the five surviving views have no consumer anywhere in the codebase, yet 000028 still carries them as REVOKE targets — the exact condition 000035 cited when it removed the ai_usage_* views

*Lens:* `db-schema` · *Confidence:* `strong` · *Verdict:* `confirmed` · *Severity:* low

**Subject:** `{'kind': 'table', 'identity': 'views active_sessions, expired_sessions, api_hourly_distribution, api_daily_summary'}`

**Evidence:**

- The db-map scores all four at zero external refs, unambiguous. Only api_usage_summary among the live views has a consumer.
  - Reproduce: `jq -r '.records[] | select(.kind=="view") | "\(.identity) refs=\(.external_refs) ambiguous=\(.name_ambiguous//false)"' docs/audit/runs/2026-08-08/maps/db-map.json`
- Grepping the four names across internal and web/src returns only migration files — their own CREATE/DROP statements and 000028's REVOKE target list. No Go or TypeScript file names any of them.
  - Reproduce: `git grep -nE '\bactive_sessions\b|\bexpired_sessions\b|\bapi_daily_summary\b|\bapi_hourly_distribution\b' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal web/src ':(exclude)internal/adapters/storage/postgres/migrations'`
- Without the migrations exclusion the same grep shows exactly what does reference them: 000001/000003 create them, 000001/000003 down-migrations drop them, and 000028 lists them among its REVOKE targets.
  - `internal/adapters/storage/postgres/migrations/000028_tighten_000003_rls_policies.up.sql:56`
  - Reproduce: `git grep -nE '\bactive_sessions\b|\bexpired_sessions\b|\bapi_daily_summary\b|\bapi_hourly_distribution\b' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal web/src`
- 000035 established the precedent and the reasoning verbatim, for the two sibling views in the same 000028 target list.
  - `internal/adapters/storage/postgres/migrations/000035_drop_ai_advisor_tables.up.sql:4`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/adapters/storage/postgres/migrations/000035_drop_ai_advisor_tables.up.sql | sed -n '4,10p'`

**Proposed fix:** Add a migration dropping the four views, following 000035 exactly: explicit DROP VIEW IF EXISTS by name (no CASCADE), and a .down.sql that recreates them WITH (security_invoker = true) plus the pg_roles-guarded REVOKE from anon and authenticated, so a rollback does not reopen what 000028 closed. Remove the four names from any future RLS-maintenance target list. If session and API-usage dashboards are a wanted future feature, keep api_usage_summary (live) and note the four as intentionally deferred rather than dropping them.

**Blast radius:**

- `internal/adapters/storage/postgres/migrations/ — one new up/down pair`
- `Any external Supabase dashboard or SQL console query outside this repository that reads these views — not visible to a repository-scoped audit`
- `docs/SCHEMA.md — view list`

**Acceptance criteria:**

- [ ] `go build ./...` and `go test -race ./...` pass with the four views dropped.
- [ ] Applying the new migration then rolling it back leaves the four views present, security_invoker = true, and revoked from anon and authenticated.
- [ ] `grep -rn 'active_sessions\|expired_sessions\|api_daily_summary\|api_hourly_distribution' internal/ web/src/` returns nothing outside the migrations directory.
- [ ] The maintainer has confirmed no Supabase dashboard outside the repository reads these views.

### FU-18 — Drop the three indexes fully subsumed by another index or by the primary key

**Severity:** low · **Effort:** S · **Rolls up:** DB-005

> **Controller note.** They survived 000003's unused-index purge because that purge went by usage telemetry rather than by containment. Containment is provable from the migration files; this audit had NO database access, so the ticket must not claim anything about live usage.

#### DB-005 — Three live indexes are fully subsumed by another index or by their table's primary key, and survived 000003's unused-index purge because that purge went by usage telemetry rather than by containment

*Lens:* `db-schema` · *Confidence:* `strong` · *Verdict:* `confirmed` · *Severity:* low

**Subject:** `{'kind': 'index', 'identity': 'idx_access_log_covering, idx_dh_suggestions_date, idx_purchases_campaign'}`

**Evidence:**

- idx_access_log_card and idx_access_log_covering are the same four columns in the same order, differing only in the sort direction of the trailing column. Postgres scans a btree in either direction, so one of the pair serves both orderings and the other is pure write amplification. Both are in the final schema.
  - `internal/adapters/storage/postgres/migrations/000001_initial_schema.up.sql:367`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/adapters/storage/postgres/migrations/000001_initial_schema.up.sql | sed -n '366,368p'`
- idx_dh_suggestions_date indexes (suggestion_date), which is the leading column of dh_suggestions' primary key (suggestion_date, type, category, rank). The PK's implicit unique index already serves every query the standalone index could.
  - `internal/adapters/storage/postgres/migrations/000001_initial_schema.up.sql:591`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/adapters/storage/postgres/migrations/000001_initial_schema.up.sql | sed -n '590,592p'`
- idx_purchases_campaign indexes (campaign_id), a strict leading prefix of idx_purchases_campaign_date (campaign_id, purchase_date DESC) declared two lines below it. Both are in the final schema.
  - `internal/adapters/storage/postgres/migrations/000001_initial_schema.up.sql:234`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/adapters/storage/postgres/migrations/000001_initial_schema.up.sql | sed -n '234,236p'`
- All three survive the cumulative replay of all 35 migrations — none was caught by 000003's Fix 5, which dropped 26 indexes by Supabase usage telemetry.
  - Reproduce: `jq -r '.records[] | select(.kind=="index") | select((.note//"")|test("NOT in the final schema")|not) | select(.identity=="idx_access_log_covering" or .identity=="idx_dh_suggestions_date" or .identity=="idx_purchases_campaign") | "\(.identity)\t\(.defined_at)"' docs/audit/runs/2026-08-08/maps/db-map.json`
- campaign_purchases' primary key is a single TEXT id, so idx_purchases_campaign is not additionally covered by the PK — the redundancy is specifically against idx_purchases_campaign_date.
  - `internal/adapters/storage/postgres/migrations/000001_initial_schema.up.sql:157`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/adapters/storage/postgres/migrations/000001_initial_schema.up.sql | sed -n '156,158p'`

**Proposed fix:** Add one migration dropping idx_access_log_covering, idx_dh_suggestions_date and idx_purchases_campaign, with a .down.sql recreating all three verbatim. Confirm against pg_stat_user_indexes on the production database before merging — that check needs database access this audit does not have, and is the one step that turns this from containment reasoning into a measurement.

**Blast radius:**

- `internal/adapters/storage/postgres/migrations/ — one new up/down pair`
- `Query plans over card_access_log, dh_suggestions and campaign_purchases`
- `docs/SCHEMA.md — index list`

**Acceptance criteria:**

- [ ] pg_stat_user_indexes on production shows the three dropped indexes had idx_scan counts that the surviving covering index or PK absorbs, checked before the migration is merged.
- [ ] EXPLAIN on the queries in card_access_log, dh_suggestions and campaign_purchases access paths shows no sequential scan appearing where an index scan ran before.
- [ ] `go test -race ./internal/adapters/storage/postgres/...` passes.
- [ ] The .down.sql recreates all three indexes with their original definitions; up-then-down-then-up is idempotent.


---

## Tier: Correctness

### FU-19 — Fix the dh_push_status index that 000003 never actually created

**Severity:** low · **Effort:** S · **Rolls up:** DB-004

> **Controller note.** CREATE INDEX IF NOT EXISTS collides with 000001's partial index of the same name, so the fix was a silent no-op, and 000003's down-migration then drops an index it never created. Verifier lowered high→low and gutted the impact story: dh_push_status is NOT NULL DEFAULT '' (000001:205), so the IS NULL branch is dead, no query filters the excluded '' rows sargably, and the intended non-partial index would have delivered no measurable benefit at this baseline. The '~27% of query time' framing is inherited from 000003's own comment and does NOT attach to this gap — do not repeat it in the ticket. Acceptance criterion 4 is unprovable as written (CountUnsoldByDHPushStatus contains no dh_push_status filter) and needs rewriting before filing.

#### DB-004 — 000003's index-advisor fix for the ~27%-of-query-time dh_push_status path is a silent no-op: CREATE INDEX IF NOT EXISTS collides with 000001's partial index of the same name, and 000003's down-migration then drops the index it never created

*Lens:* `db-schema` · *Confidence:* `strong` · *Verdict:* `confirmed_lower_severity` · *Severity:* low

**Subject:** `{'kind': 'index', 'identity': 'idx_campaign_purchases_dh_push_status'}`

**Verifier correction:** The impact argument. It claims 'live queries filter on exactly the rows the partial predicate excludes (NULL or empty dh_push_status)'. dh_push_status is NOT NULL DEFAULT '' (000001_initial_schema.up.sql:205), so no row is ever NULL and the IS NULL branch is dead; the only excluded rows are '' rows, and no query in the repository filters on them with a sargable predicate. The intended non-partial index would therefore have delivered no measurable benefit over the partial one for any query at this baseline, so the ~27%-of-query-time framing carried over from 000003's comment does not attach to the gap. Acceptance criterion 4 is also unprovable as written: CountUnsoldByDHPushStatus contains no dh_push_status filter for an index to serve.

**Evidence:**

- 000001 creates the name as a PARTIAL index, excluding empty-string rows.
  - `internal/adapters/storage/postgres/migrations/000001_initial_schema.up.sql:245`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/adapters/storage/postgres/migrations/000001_initial_schema.up.sql | sed -n '245,247p'`
- 000003 'Fix 6' intends a NON-partial index under the same name, citing ~27% of total DB query time. CREATE INDEX IF NOT EXISTS keys on the relation name only, so with 000001's index already present this statement is a no-op and the differing definition is silently discarded.
  - `internal/adapters/storage/postgres/migrations/000003_supabase_security_and_perf_fixes.up.sql:283`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/adapters/storage/postgres/migrations/000003_supabase_security_and_perf_fixes.up.sql | sed -n '281,285p'`
- The db-map's cumulative replay independently reaches the same conclusion: the final state is 000001's partial definition.
  - Reproduce: `jq -r '.records[] | select(.identity=="idx_campaign_purchases_dh_push_status") | "\(.defined_at)\n\(.note)"' docs/audit/runs/2026-08-08/maps/db-map.json`
- 000003's down-migration drops the index under 'Undo Fix 6', on the assumption 000003 created it. It did not — 000001 did. Rolling 000003 back therefore destroys 000001's partial index and leaves campaign_purchases with no dh_push_status index; re-applying 000003 then creates the NON-partial one, so the index definition depends on migration history rather than on the files.
  - `internal/adapters/storage/postgres/migrations/000003_supabase_security_and_perf_fixes.down.sql:11`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/adapters/storage/postgres/migrations/000003_supabase_security_and_perf_fixes.down.sql | sed -n '6,12p'`
- The gap is not academic: live queries filter on exactly the rows the partial predicate excludes (NULL or empty dh_push_status), which the intended non-partial index would have covered.
  - `internal/adapters/storage/postgres/purchase_dh_query_store.go:101`
  - Reproduce: `git grep -nE 'dh_push_status IS NULL|COALESCE\(p\.dh_push_status' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/adapters/storage/postgres`

**Proposed fix:** Decide which definition is wanted and make the files say so. If the non-partial index is wanted (as 000003's telemetry comment argues), add a new migration that DROPs the name and recreates it without the WHERE clause. If the partial one is wanted, delete the dead statement from 000003's up and the matching DROP from 000003's down, so a rollback stops destroying an index 000001 owns. Either way, never reuse an existing index name under CREATE INDEX IF NOT EXISTS with a different definition — that construct matches on name alone.

**Blast radius:**

- `internal/adapters/storage/postgres/migrations/000003_supabase_security_and_perf_fixes.up.sql and .down.sql — editing an already-applied migration is only safe if every deployment has the same checksum state; prefer a new forward migration`
- `internal/adapters/storage/postgres/purchase_dh_query_store.go — the queries whose plans change`
- `docs/SCHEMA.md — index list`

**Acceptance criteria:**

- [ ] A fresh database migrated to head has exactly one index named idx_campaign_purchases_dh_push_status, with the definition the team chose, and no migration file contains a CREATE INDEX IF NOT EXISTS that reuses an existing index name with a different definition.
- [ ] Migrating to head, rolling back to 000002, and migrating to head again yields the same index definition all three times.
- [ ] `go test -race ./internal/adapters/storage/postgres/...` passes.
- [ ] If the non-partial definition is chosen, EXPLAIN on the CountUnsoldByDHPushStatus and DH-pending queries in purchase_dh_query_store.go shows the index used for the NULL/empty branch.

### FU-20 — Collapse the two hand-maintained campaign_sales column lists that have drifted by six columns

**Severity:** medium · **Effort:** M · **Rolls up:** DUP-002

> **Controller note.** The strongest live defect in the run: a JOIN-loaded Sale silently returns zeroed sale-outcome fields. Found by reading, not by the duplication scan — body-similarity detection is blind to duplication expressed as parallel SQL column lists, which is recorded as a scope gap for the next run.

#### DUP-002 — campaign_sales is read through two hand-maintained column-list/scan-destination pairs that have already drifted by six columns, so a JOIN-loaded Sale silently returns zeroed sale-outcome fields

*Lens:* `duplication` · *Confidence:* `strong` · *Verdict:* `confirmed` · *Severity:* medium

**Subject:** `{'kind': 'symbol', 'identity': 'internal/adapters/storage/postgres: saleColumns/scanSale vs saleColumnsAliased/the inline sale block in scanPurchaseWithSale'}`

**Verifier correction:** Only a framing nit: the title asserts a JOIN-loaded Sale 'silently returns zeroed sale-outcome fields' without the qualifier, while evidence item 5 correctly establishes no consumer reads them. A fixer reading the title alone would expect a live defect. The body is accurate and the severity is honestly calibrated.

**Evidence:**

- There are two column lists for the same table and two independent scan implementations: saleColumns + scanSale, and saleColumnsAliased + a 40-line inline block inside scanPurchaseWithSale that re-declares each sale field as a sql.Null* local and assigns it back by hand.
  - `internal/adapters/storage/postgres/purchase_scan_helpers.go:54`
  - Reproduce: `git grep -n 'saleColumnsAliased\|const saleColumns\|func scanSale\|func scanPurchaseWithSale' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/adapters/storage/postgres/purchase_scan_helpers.go`
- The two lists have already diverged: saleColumnsAliased selects 23 columns, saleColumns selects 29. The six missing from the aliased list are original_list_price_cents, price_reductions, days_listed, sold_at_asking_price, was_cracked, order_id.
  - `internal/adapters/storage/postgres/purchase_scan_helpers.go:54-66`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/adapters/storage/postgres/purchase_scan_helpers.go | sed -n '54,58p' | tr ',' '\n' | grep -c 's\.'; git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/adapters/storage/postgres/purchase_scan_helpers.go | sed -n '61,66p' | tr ',' '\n' | sed 's/[`]//g' | grep -cE '[a-z_]'`
- The divergence is silent, not loud: each list is internally consistent with its own scan-destination sequence, so Scan never reports an arity mismatch. Every inventory.PurchaseWithSale returned by GetPurchasesWithSales therefore carries a Sale whose OriginalListPriceCents, PriceReductions, DaysListed, SoldAtAskingPrice, WasCracked and OrderID are the Go zero value, indistinguishable from a genuinely-zero row.
  - `internal/adapters/storage/postgres/analytics_store.go:190`
  - Reproduce: `git grep -n 'saleColumnsAliased' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/adapters/storage/postgres/analytics_store.go`
- The six fields are real, populated columns on the write path — inventory.Service writes them at sale-creation time from BulkSaleInput — so the JOIN path is dropping data that exists in the row, not data that is never set.
  - `internal/domain/inventory/service_crud.go:308`
  - Reproduce: `git grep -nE '\b(OriginalListPriceCents|PriceReductions|DaysListed)\b' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/domain/inventory/service_crud.go`
- No consumer reads the six fields off a PurchaseWithSale today, which is why the drift has gone unnoticed — the hazard is latent, not currently firing. Outside the postgres package and tests, the only references are on the write path.
  - `internal/domain/inventory/service_crud.go:308`
  - Reproduce: `git grep -nE '\b(OriginalListPriceCents|PriceReductions|DaysListed|SoldAtAskingPrice|WasCracked)\b' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- '*.go' | grep -v '/postgres/' | grep -v _test.go | grep -v core_types.go | grep -v import_types.go`
- The same file shows the fix already applied to the purchase half: purchaseColumns/purchaseColumnsAliased share a single purchaseScanDests destination builder used by both scanPurchase and scanPurchaseWithSale. Only the sale half was left duplicated.
  - `internal/adapters/storage/postgres/purchase_scan_helpers.go:107`
  - Reproduce: `git grep -n 'func purchaseScanDests\|purchaseScanDests(' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/adapters/storage/postgres/purchase_scan_helpers.go`

**Proposed fix:** Give the sale half the same treatment the purchase half already has: derive saleColumnsAliased from saleColumns by prefixing, or keep one list and one saleScanDests(*inventory.Sale, ...nulls) []any builder that both scanSale and scanPurchaseWithSale consume. If the six columns are deliberately excluded from the JOIN for query cost, say so in a comment on saleColumnsAliased and make the omission explicit rather than incidental.

**Blast radius:**

- `internal/adapters/storage/postgres/purchase_scan_helpers.go`
- `internal/adapters/storage/postgres/sale_store.go`
- `internal/adapters/storage/postgres/analytics_store.go`

**Acceptance criteria:**

- [ ] The campaign_sales column set is written once: `git grep -c 'sale_price_cents' internal/adapters/storage/postgres/purchase_scan_helpers.go` returns 1, or the aliased list is mechanically derived from the canonical one.
- [ ] `go build ./...` and `go test -race ./...` pass.
- [ ] A test loads a sale through both paths (SaleStore.GetSaleByPurchaseID and AnalyticsStore.GetPurchasesWithSales) for the same purchase and asserts the two inventory.Sale values are equal, so any future column added to one list and not the other fails.

### FU-21 — Reconcile the Go↔TS drift on PortfolioHealth and CampaignHealth

**Severity:** medium · **Effort:** M · **Rolls up:** FE-003, FE-004

> **Controller note.** Two directions of the same seam, fixed together. FE-003: the dashboard hero renders delta chips and a freshness line off two PortfolioHealth fields no Go struct emits, so those branches are unreachable in production and green only against hand-built fixtures. FE-004: the TS CampaignHealth mirror omits seven fields Go does emit, one of which carries a Go comment claiming it was named 'for frontend compatibility' with a frontend that never modelled it. Note the audit's Go↔TS tag check was spot-checked, not systematic — one struct pair was verified and held; a systematic diff was never run, so this pair is not necessarily the whole of it.

#### FE-003 — Type drift: the dashboard hero renders delta chips and a freshness line off two PortfolioHealth fields no Go struct ever emits, so those UI branches are unreachable in production and exercised only by hand-built test fixtures

*Lens:* `frontend-health` · *Confidence:* `mechanical` · *Verdict:* `confirmed` · *Severity:* medium

**Subject:** `{'kind': 'type', 'identity': 'PortfolioHealth.realizedROIDelta / PortfolioHealth.totalRecoveredDelta (web/src/types/campaigns/analytics.ts:251-252) and the PortfolioDelta / DeltaChip / FreshnessLine code they gate'}`

**Verifier correction:** One overstatement in the title. FreshnessLine is not dead — I read it at HeroStatsBar.tsx:211-224 and it is driven by asOfMs, returning null only when no timestamp exists; the delta parameter controls just the optional label span at :220. So the dead surface is the two DeltaChip renders (:89, :145) and one label span, not a whole component. This does not change the severity: the drift is real, three branches never fire, and a reviewer reading HeroStatsBar would reasonably believe the dashboard shows deltas that it cannot.

**Evidence:**

- TS PortfolioHealth declares two optional fields absent from the Go struct it mirrors.
  - `web/src/types/campaigns/analytics.ts:251`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:web/src/types/campaigns/analytics.ts | sed -n '244,253p'`
- The Go PortfolioHealth struct has six fields and no delta field. This is embedding-proof: the struct literal is flat.
  - `internal/domain/inventory/health_types.go:12`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/domain/inventory/health_types.go | sed -n '11,19p'`
- No Go source anywhere emits these keys, under any casing, by any mechanism including embedded structs, map literals, and string-built JSON. This is the decisive check, and it is case-insensitive so it also rules out a differently-cased tag.
  - Reproduce: `git grep -niE 'roidelta|recovereddelta|PortfolioDelta' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal cmd`
- Both fields are read and rendered by HeroStatsBar, so the drift is not merely cosmetic: three render paths are permanently dead.
  - `web/src/react/components/portfolio/HeroStatsBar.tsx:89`
  - Reproduce: `git grep -nE 'delta=' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- web/src`
- PortfolioDelta exists solely to type these two fields and the components that consume them; it has no other declaration site or consumer. Every non-HeroStatsBar reference is the declaration itself.
  - `web/src/types/campaigns/analytics.ts:219`
  - Reproduce: `git grep -nE '\b(PortfolioDelta|DeltaChip|FreshnessLine)\b' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- web/src`
- The only places these fields ever hold a value are the component's own test fixtures, which construct them by hand — which is why the dead branches have never been noticed.
  - `web/src/react/components/portfolio/HeroStatsBar.test.tsx:59`
  - Reproduce: `git grep -nE '\b(realizedROIDelta|totalRecoveredDelta)\b' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- web internal cmd`

**Proposed fix:** Decide the direction and close the gap. Either (a) drop realizedROIDelta, totalRecoveredDelta, and the PortfolioDelta interface from web/src/types/campaigns/analytics.ts and delete the DeltaChip/FreshnessLine delta rendering from HeroStatsBar.tsx plus its five test fixtures; or (b) if week-over-week deltas are wanted, add the fields to inventory.PortfolioHealth in internal/domain/inventory/health_types.go and populate them, at which point the existing UI lights up. Do not leave the two sides diverged.

**Blast radius:**

- `web/src/types/campaigns/analytics.ts`
- `web/src/react/components/portfolio/HeroStatsBar.tsx`
- `web/src/react/components/portfolio/HeroStatsBar.test.tsx`
- `internal/domain/inventory/health_types.go (only if direction (b) is chosen)`

**Acceptance criteria:**

- [ ] Under direction (a): `git grep -nE 'PortfolioDelta|realizedROIDelta|totalRecoveredDelta' -- web/src` returns no output, and `cd web && npm run build && npm test` both pass.
- [ ] Under direction (b): `git grep -niE 'roidelta|recovereddelta' -- internal cmd` returns the new json tags, `go build ./... && go test -race ./...` pass, and a dashboard screenshot from `make screenshots` shows a rendered delta chip.
- [ ] Either way, re-running the TS-vs-Go field comparison reports no divergence for PortfolioHealth.

#### FE-004 — Type drift: the TS CampaignHealth mirror omits seven fields the Go struct emits, including liquidation-awareness and in-hand/in-transit breakdown — one of which carries a Go comment claiming it was named 'for frontend compatibility' with a frontend that never modelled it

*Lens:* `frontend-health` · *Confidence:* `strong` · *Verdict:* `confirmed` · *Severity:* low

**Subject:** `{'kind': 'type', 'identity': 'CampaignHealth (web/src/types/campaigns/analytics.ts:230) vs inventory.CampaignHealth (internal/domain/inventory/health_types.go:22)'}`

**Verifier correction:** The central consequence is wrong for four of the seven fields, and I found it only by widening the search beyond the finding's pathspec. `git grep -nE 'inHandCapitalCents|liquidationLossCents|ebayChannelMarginPct|inTransitUnsoldCount|inHandUnsoldCount|liquidationSaleCount|inTransitCapitalCents' 06d9f8ce -- .claude/skills/campaign-analysis` returns hits in api-cheatsheet.md:173-176 and field-semantics.md:192-210, the latter documenting `/api/portfolio/snapshot` -> `.health.campaigns[].inHandCapitalCents` as an aggregation source. The four in-hand/in-transit fields have a real, tracked-in-repo consumer that reads the JSON API directly rather than through the browser, so 'shipped over the wire on every portfolio-health response and discarded unread' is false for them. Separately, ebayChannelMarginPct is not inert on the Go side either: internal/domain/portfolio/suggestion_rules.go:271, :314, :315 branch on it. What actually remains is the narrow and still-valid defect: the TS mirror is an incomplete copy of the Go contract, which is debt under this repo's manual type-sync convention. Severity stays low — it is already the floor, and I could not lower it further. The controller should rewrite the consequence before ticketing, and should note the finding is `strong`, not `mechanical`, so it is not ticketable unqualified under the preamble's tiering.

**Evidence:**

- The Go struct declares eleven fields the TS mirror has, plus seven it does not.
  - `internal/domain/inventory/health_types.go:37`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/domain/inventory/health_types.go | sed -n '35,46p'`
- The TS interface is flat (no `extends`) and stops at healthReason, so the seven fields are genuinely unmodelled rather than inherited.
  - `web/src/types/campaigns/analytics.ts:230`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:web/src/types/campaigns/analytics.ts | sed -n '230,242p'`
- No frontend code and no API documentation references any of the seven field names, so the data is shipped over the wire on every portfolio-health response and discarded unread.
  - Reproduce: `git grep -nE 'inHandCapitalCents|liquidationLossCents|ebayChannelMarginPct|inTransitUnsoldCount' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- web/src docs/API.md`

**Proposed fix:** Add the seven missing fields to the TS CampaignHealth interface so the mirror is faithful, and either surface them in the portfolio UI (the liquidation-awareness trio in particular exists to distinguish a broken channel from a forced liquidation, which the current UI cannot show) or, if they are genuinely unwanted client-side, drop them from the Go response struct instead. Also correct or remove the 'JSON field name retained for frontend compatibility' comment on EbayChannelMarginPct, which is not true at this baseline.

**Blast radius:**

- `web/src/types/campaigns/analytics.ts`
- `internal/domain/inventory/health_types.go`
- `web/src/react/components/portfolio/ (if the fields are surfaced)`

**Acceptance criteria:**

- [ ] `cd web && npx tsc --noEmit` passes with the TS interface extended.
- [ ] A field-by-field comparison of TS CampaignHealth against inventory.CampaignHealth shows no name present on one side and absent on the other.
- [ ] `go build ./... && go test -race ./...` pass if the Go side is changed instead.
- [ ] The stale 'for frontend compatibility' comment either names a real frontend consumer or is gone.

### FU-22 — Share one row-action handler map between the desktop and mobile inventory rows

**Severity:** low · **Effort:** S · **Rolls up:** DUP-004

> **Controller note.** Each view hand-assembles the same ten-key map and every key but one is optional, so adding an action to one view silently omits it from the other — the type system cannot catch it.

#### DUP-004 — Desktop and mobile inventory rows each hand-assemble the same ten-key row-action handler map, and every key but one is optional, so adding an action to one view silently omits it from the other

*Lens:* `duplication` · *Confidence:* `strong` · *Verdict:* `confirmed` · *Severity:* low

**Subject:** `{'kind': 'component', 'identity': 'web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx and MobileCard.tsx — the duplicated RowActionHandlers wiring block'}`

**Evidence:**

- A sixteen-line block — the ten-key handlers object, the flags object, and the three resolveContextualPrimary / fallbackPrimary / resolveOverflowActions calls — is byte-identical between the two components after whitespace normalization. Found by an exact-body-hash pass over every non-test .ts/.tsx file under web/src at the baseline; it was the only duplicate pair the pass returned.
  - `web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx:128`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx | sed -n '128,143p' | md5sum; git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:web/src/react/pages/campaign-detail/inventory/MobileCard.tsx | sed -n '55,70p' | md5sum`
- The drift would be silent rather than a type error: nine of the ten keys on RowActionHandlers are optional, so a handler added to one view's object and forgotten in the other still type-checks and the action simply disappears from that viewport.
  - `web/src/react/pages/campaign-detail/inventory/rowActions.ts:19`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:web/src/react/pages/campaign-detail/inventory/rowActions.ts | sed -n '19,30p'`
- The action-resolution rules themselves are already correctly shared in rowActions.ts — only the wiring that feeds them is duplicated, so the fix is additive and small.
  - `web/src/react/pages/campaign-detail/inventory/rowActions.ts:48`
  - Reproduce: `git grep -n 'resolveContextualPrimary\|resolveOverflowActions' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- web/src`

**Proposed fix:** Add a small hook or helper in rowActions.ts (e.g. useRowActions(item, callbacks, flags) returning { primary, fallbackPrimary, overflow }) and have both DesktopRow and MobileCard call it, so the handler set is enumerated once. Alternatively, take the handlers object as a single prop from the shared parent that already owns all ten callbacks, rather than re-assembling it per viewport.

**Blast radius:**

- `web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx`
- `web/src/react/pages/campaign-detail/inventory/MobileCard.tsx`
- `web/src/react/pages/campaign-detail/inventory/rowActions.ts`

**Acceptance criteria:**

- [ ] The ten-key handler object literal appears once under web/src/react/pages/campaign-detail/inventory/, verified by `grep -rc 'onRetryDHMatch,' web/src/react/pages/campaign-detail/inventory/` totalling 1.
- [ ] `cd web && npm run build` and `npm test` pass.
- [ ] Adding a new optional action to RowActionHandlers requires editing exactly one file for it to appear in both the desktop row and the mobile card.

### FU-23 — Give the auth session-expiry constant one source, and move the test double out of the production package

**Severity:** low · **Effort:** M · **Rolls up:** DUP-003

> **Controller note.** Verifier lowered medium→low and re-grounded it: authService is NOT a second production implementation. Its own doc comment (service_impl.go:15-22) declares it a deliberate lightweight repository-backed double 'intentionally free of external dependencies', and two of its 21 methods return ERR_NOT_IMPLEMENTED by design pointing at OAuthService as the production path. So the ValidateSession difference is a documented non-goal, not drift between implementations meant to agree — the ticket must NOT say they 'have already drifted'. What survives: a security-relevant expiry constant duplicated with no shared source and no test that would catch a one-sided edit, and a test-shaped implementation living in a production package when the repo's convention puts doubles in internal/testutil/mocks/. Related to SIZE-001, same type, different defect.

#### DUP-003 — auth.Service has two full implementations; the domain one is constructed only by its own test file and has already drifted from the production one on session validation

*Lens:* `duplication` · *Confidence:* `strong` · *Verdict:* `confirmed_lower_severity` · *Severity:* low

**Subject:** `{'kind': 'symbol', 'identity': 'internal/domain/auth.authService (auth.New) — a second full implementation of auth.Service alongside internal/adapters/clients/google.OAuthService'}`

**Verifier correction:** 'Two full implementations' overstates what authService is, and that overstatement drives the severity. Its own doc comment (service_impl.go:15-22) declares it a deliberate lightweight repository-backed double, 'intentionally free of external dependencies... so that it can be instantiated in unit tests without any infrastructure setup,' and two of the 21 methods (ExchangeCodeForTokens, GetUserInfo) return ERR_NOT_IMPLEMENTED by design with a comment pointing at OAuthService as the production path. So the ValidateSession difference is a documented non-goal of a test double, not drift between two production implementations that were meant to agree — the finding's own framing ('have already drifted') attributes an intent the code explicitly disclaims. What survives is genuine but smaller: a security-relevant constant duplicated with no shared source and no test that would catch a one-sided edit, and a test-shaped implementation living in a production package where the repo's stated convention puts doubles in internal/testutil/mocks/. The expiry-constant leg alone justifies a ticket; the acceptance criteria remain provable as written.

**Evidence:**

- Two types implement the same 21-method auth.Service interface with matching method sets, each asserting satisfaction explicitly.
  - `internal/domain/auth/service_impl.go:56`
  - Reproduce: `git grep -n 'var _ auth.Service = \|var _ Service = ' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- '*.go'`
- Only OAuthService is wired into the binary; auth.New is constructed nowhere outside internal/domain/auth/service_test.go. The Phase 1 Go map records internal/domain/auth.New with external_refs=280, but flags name_ambiguous=true with 5 distinct top-level `New` definitions — the count is selector-name inflation, and the targeted verification below is what settles it.
  - `internal/domain/auth/service_test.go:56`
  - Reproduce: `jq -c '.records[] | select(.identity=="internal/domain/auth.New") | {external_refs, name_ambiguous, distinct_definitions_for_name}' docs/audit/runs/2026-08-08/maps/go-reference-map.json; git grep -n 'auth\.New(' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- '*.go'`
- The production binary constructs only google.NewOAuthService.
  - `cmd/slabledger/main.go:232`
  - Reproduce: `git grep -n 'NewOAuthService(' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- cmd/`
- The two implementations have already drifted on ValidateSession: OAuthService refreshes the session's last-accessed timestamp (throttled to one write per 60s) after a successful validation; authService returns without touching it. A unit test written against auth.New therefore cannot observe — and will not regress-catch — the production session-touch behavior.
  - `internal/adapters/clients/google/oauth.go:242`
  - Reproduce: `git grep -n 'UpdateSessionAccess' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- '*.go'`
- The session-expiry window — a security-relevant constant — is declared twice, once per implementation, with no shared source. They agree today, which is exactly the state that makes a future one-sided edit silent.
  - `internal/domain/auth/service_impl.go:13`
  - Reproduce: `git grep -nE 'sessionExpiry|defaultSessionExpiry' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- '*.go'`
- UpdateLastLogin is byte-identical between the two implementations after normalization — found by the same exact-body-hash pass that produced DUP-001 — confirming these are copies rather than independently-motivated behaviors.
  - `internal/domain/auth/service_impl.go:149`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/adapters/clients/google/oauth.go | sed -n '311,319p' | grep -v '^[[:space:]]*$' | md5sum; git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/domain/auth/service_impl.go | sed -n '150,156p' | grep -v '^[[:space:]]*$' | md5sum`

**Proposed fix:** Decide which of the two is the contract. The lowest-risk option is to keep OAuthService as the single implementation and replace the domain authService with a mock or in-memory fake under internal/testutil/mocks/ (where the repo's stated convention already puts test doubles), deleting the parallel production-shaped implementation. If a repository-backed non-OAuth service is genuinely wanted, extract the shared session/user/allowlist logic so ValidateSession, CreateSession and the expiry constant exist once and OAuthService composes it with the Google-specific parts.

**Blast radius:**

- `internal/domain/auth/service_impl.go`
- `internal/domain/auth/service_test.go`
- `internal/adapters/clients/google/oauth.go`
- `internal/testutil/mocks/auth_inmemory.go`

**Acceptance criteria:**

- [ ] `go build ./...` and `go test -race ./...` pass after the second implementation is removed or reduced to a shared core.
- [ ] `git grep -c 'time.Hour' internal/domain/auth internal/adapters/clients/google | grep -i expiry` shows the session-expiry window declared exactly once.
- [ ] The behavior currently unique to OAuthService.ValidateSession (the throttled UpdateSessionAccess write) is exercised by a test that fails if it is removed.
- [ ] `bash scripts/check-imports.sh` passes — no domain package gains an adapter import.


---

## Tier: Architecture

### FU-24 — Route both DH listing-price resolvers through one shared implementation

**Severity:** medium · **Effort:** M · **Rolls up:** DUP-001

> **Controller note.** Verifier lowered high→medium and killed the leg carrying the severity. Evidence item 4 ('no test in dhpricing exercises the reviewed/override precedence') is FALSE: dhpricing/service_test.go:225 TestSyncPurchasePrice_ResolvesNewestCommit covers it, including the cross-offset instant-vs-lexicographic case both source comments name as the subtle failure mode — the lens missed it because it grepped for the resolver name and the test reaches it through the exported SyncPurchasePrice. So a one-sided edit breaking parsed comparison WOULD fail dhpricing's suite; do not claim otherwise. What remains: dhpricing has no cases for the zero-value guards or the empty-timestamp lexicographic fallback that dhlisting's nine-case table covers, and nothing asserts the two agree. The duplication is also a RECORDED decision, scoped out at docs/specs/2026-08-08-flat-sibling-checker-design.md:189 and docs/plans/2026-08-08-flat-sibling-checker.md:589 — read those before re-litigating. The hub is a legal shared home per CLAUDE.md:126.

#### DUP-001 — The DH listing-price resolution rule is implemented verbatim in two sibling domain packages with no shared source and no cross-package consistency test

*Lens:* `duplication` · *Confidence:* `mechanical` · *Verdict:* `confirmed_lower_severity` · *Severity:* medium

**Subject:** `{'kind': 'symbol', 'identity': 'internal/domain/dhpricing.resolveListingPrice + internal/domain/dhpricing.overrideNewer (verbatim copies of internal/domain/dhlisting.ResolveListingPriceCents + internal/domain/dhlisting.overrideNewer)'}`

**Verifier correction:** Evidence item 4 is false, and it is the leg carrying the severity. The finding states 'Only the dhlisting copy is covered by a test. There is no test in dhpricing that exercises resolveListingPrice's reviewed/override precedence.' internal/domain/dhpricing/service_test.go:225, TestSyncPurchasePrice_ResolvesNewestCommit, is a two-case table that sets ReviewedPriceCents/ReviewedAt and OverridePriceCents/OverrideSetAt on the same purchase and asserts the cents value handed to the updater — and its second case is exactly the cross-offset instant-vs-lexicographic case ('2026-04-21T08:00:00-05:00' against '2026-04-21T12:00:00Z') that both source comments name as the subtle failure mode. The lens missed it because its coverage command greps for the function name, and this test reaches the resolver through the exported SyncPurchasePrice instead of naming it — the in-package-caller trap of LENS-BRIEF §3 running in the direction that makes covered code look uncovered. So the claimed consequence (divergence in the hard case goes undetected on the dhpricing side) does not follow: a one-sided edit to dhpricing.overrideNewer that broke parsed comparison would fail dhpricing's own suite. What remains true and unaddressed is narrower — dhpricing has no cases for the zero-value guards or the empty-timestamp lexicographic fallback that dhlisting's nine-case table covers, and nothing asserts the two agree. Separately, the finding presents the duplication as if it were unexamined; it is a recorded decision, explicitly scoped out at docs/specs/2026-08-08-flat-sibling-checker-design.md:189 and docs/plans/2026-08-08-flat-sibling-checker.md:589, which the fixer should read before re-litigating it. The proposed fix and acceptance criteria are otherwise sound and the hub is a legal shared home per CLAUDE.md:126.

**Evidence:**

- The bodies of dhlisting.ResolveListingPriceCents and dhpricing.resolveListingPrice are byte-identical after comment and whitespace normalization, as are the two overrideNewer helpers. Detected by an exact-body-hash pass over every non-test Go file at the baseline (function bodies of >= 6 normalized lines).
  - `internal/domain/dhlisting/dh_push_safety.go:26 and internal/domain/dhpricing/service.go:47`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/domain/dhlisting/dh_push_safety.go | sed -n '27,37p;45,51p' | md5sum; git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/domain/dhpricing/service.go | sed -n '48,58p;65,71p' | md5sum`
- The duplication is self-declared in the source: the dhpricing copy states it 'mirrors dhlisting.ResolveListingPriceCents, inlined here to preserve the flat-siblings invariant', and its helper says it is a 'sibling-local copy to avoid importing dhlisting'.
  - `internal/domain/dhpricing/service.go:40`
  - Reproduce: `git grep -n 'mirrors dhlisting.ResolveListingPriceCents\|sibling-local copy' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/domain/dhpricing`
- Both copies are live and drive real listing prices: the dhlisting original has six external reference sites (four DH match handlers, the DH listing handler, and the dh_push scheduler) per the Phase 1 Go map, and the dhpricing copy gates SyncPurchasePrice.
  - `internal/domain/dhpricing/service.go:87`
  - Reproduce: `jq -c '.records[] | select(.identity=="internal/domain/dhlisting.ResolveListingPriceCents")' docs/audit/runs/2026-08-08/maps/go-reference-map.json; git grep -n 'resolveListingPrice(p)' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/domain/dhpricing`
- Only the dhlisting copy is covered by a test. There is no test in dhpricing that exercises resolveListingPrice's reviewed/override precedence, and no test anywhere asserts that the two copies agree.
  - `internal/domain/dhlisting/push_safety_test.go:13`
  - Reproduce: `git grep -ln 'resolveListingPrice\|ResolveListingPriceCents' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- '*_test.go'`
- The stated justification (the flat-sibling rule bars dhpricing from importing dhlisting) does not force a copy: CLAUDE.md and internal/README.md both state siblings may depend on the inventory hub, and both copies already take an *inventory.Purchase, so the resolver can live in inventory (or a leaf) and be imported by both.
  - `CLAUDE.md:126`
  - Reproduce: `git grep -n 'Siblings may depend on' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- CLAUDE.md`

**Proposed fix:** Move the resolver and its overrideNewer helper to a single home both siblings may legally import — the inventory hub (e.g. alongside internal/domain/inventory/price_flags.go) is the smallest option and is explicitly permitted by the flat-sibling rule. Have dhlisting.ResolveListingPriceCents and dhpricing both delegate to it, and move the existing table-driven cases in internal/domain/dhlisting/push_safety_test.go onto the shared function so one test covers both call paths.

**Blast radius:**

- `internal/domain/dhlisting/dh_push_safety.go`
- `internal/domain/dhlisting/dh_list_one_purchase.go`
- `internal/domain/dhpricing/service.go`
- `internal/domain/inventory/ (new home for the shared resolver)`
- `internal/adapters/httpserver/handlers/campaigns_dh_listing.go`
- `internal/adapters/httpserver/handlers/dh_fix_match_handler.go`
- `internal/adapters/httpserver/handlers/dh_match_handler.go`
- `internal/adapters/httpserver/handlers/dh_retry_match_handler.go`
- `internal/adapters/httpserver/handlers/dh_select_match_handler.go`
- `internal/adapters/scheduler/dh_push.go`

**Acceptance criteria:**

- [ ] `grep -c 'time.Parse(time.RFC3339, overrideSetAt)' $(git ls-files 'internal/**/*.go')` totals 1 across the repository — the reviewed/override precedence rule exists in exactly one place.
- [ ] `go build ./...` and `go test -race ./...` pass with both dhlisting and dhpricing delegating to the single shared resolver.
- [ ] `bash scripts/check-imports.sh` passes — dhpricing still does not import dhlisting, and neither sibling imports the other.
- [ ] The precedence table currently in internal/domain/dhlisting/push_safety_test.go runs against the shared function, and a test in dhpricing (or the shared package) fails if the two paths ever resolve a purchase differently.

### FU-25 — Close the fail-open holes in check-imports.sh and cover both checks in its self-test

**Severity:** medium · **Effort:** M · **Rolls up:** ARCH-001

> **Controller note.** Verifier lowered high→medium after building fixtures. The finding claims any rename turns both checks into vacuous passes that still exit 0; in fact fixture B (internal/domain renamed) and fixture E (module path rewritten) both print the false pass line and then exit 1 via downstream guards, so `make check` still fails. Only the storage-to-clients check is fail-open all the way to exit 0. Combined with the invariant holding at this baseline, the real consequence is one vacuous check plus a self-test that would not notice either breaking. The renamed-directory acceptance criterion should target internal/adapters/storage, where exit 0 was actually demonstrated, not internal/domain. Adjacent to SLA-45 (#574, target-side flat-sibling enforcement), CLOSED and landed pre-baseline — same script, different hole.

#### ARCH-001 — The two hexagonal-invariant checks in check-imports.sh fail open, and the self-test has zero coverage for either of them

*Lens:* `architecture` · *Confidence:* `strong` · *Verdict:* `confirmed_lower_severity` · *Severity:* medium

**Subject:** `{'kind': 'package', 'identity': 'scripts/check-imports.sh (hexagonal-invariant passes 1 and 2)'}`

**Verifier correction:** Evidence item 2 overstates the blast radius, and it is the claim that carries 'high'. It asserts that any rename 'turns both checks into vacuous passes that still print Architecture check passed' — the print happens, but the script does not pass. Fixture B (internal/domain renamed to internal/domainv2, violating file intact) printed the false pass line and then exited 1, because `domain_files=$(find internal/domain ...)` comes back empty and the downstream 'found no non-test .go files' guard fires. Fixture E (module path rewritten from guarzo/slabledger to acme/newname throughout) likewise printed the false pass and then exited 1 via the 'derived 0 sub-package(s)' guard. So for pass 1 and for the module-rename variant, the fail-open is masked at script level and `make check` still fails; only the storage-to-clients check is fail-open all the way to exit 0, because nothing downstream touches internal/adapters/. Combined with the invariant being satisfied at this baseline (which the finding correctly states), the realistic consequence is one vacuous check plus a self-test that would not notice either check breaking — a real gap in the project's sole hexagonal gate, but medium, not high. The proposed fix and all four acceptance criteria remain correct and provable as written; the renamed-directory criterion should target internal/adapters/storage, where I demonstrated the exit-0 pass, rather than only internal/domain.

**Evidence:**

- The domain -> adapters check and the storage -> clients check are each a single grep whose non-zero exit is swallowed by `2>/dev/null || true`. Neither has a fail-closed guard, in contrast to the flat-sibling section below them, which fails closed three separate times (empty file list, grep status > 1, fewer than two derived siblings, and a check-count mismatch).
  - `scripts/check-imports.sh:30,42`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:scripts/check-imports.sh | grep -n 'grep -rn'`
- A grep over a path that does not exist produces empty output and, with `|| true`, a zero status — indistinguishable at the call site from a clean scan. Any tree reorganisation, directory rename, or Go module-path rename therefore turns both checks into vacuous passes that still print 'Architecture check passed'.
  - Reproduce: `grep -rn '"github.com/guarzo/slabledger/internal/adapters' internal/domain-renamed/ 2>/dev/null || true`
- scripts/check-imports-test.sh creates exactly one kind of directory in its fixtures — internal/domain/<pkg>. No fixture ever creates internal/adapters/ or internal/adapters/storage/, so in all five cases both hexagonal greps run against absent paths.
  - `scripts/check-imports-test.sh:29`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:scripts/check-imports-test.sh | grep -n 'mkdir -p\|internal/adapters'`
- Running the self-test confirms the consequence: all five cases pass, and the case explicitly named 'clean tree passes' is green precisely while both hexagonal checks are evaluating nothing. The suite that exists to prove the checker works exercises only the flat-sibling section.
  - Reproduce: `bash scripts/check-imports-test.sh`
- This is the sole mechanical enforcement of the invariant CLAUDE.md calls the Key Principle. `make check` runs the self-test and then the checker; there is no second gate.
  - `Makefile:146`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:Makefile | grep -n -A5 '^check:'`
- The invariant itself is satisfied at this baseline — the finding is about the enforcement, not a live breach. No file under internal/domain/ imports internal/adapters/ at the pinned revision, and no file under internal/platform/ does either.
  - Reproduce: `git grep -nE '"github\.com/guarzo/slabledger/internal/adapters' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- 'internal/domain/*' 'internal/platform/*'`

**Proposed fix:** Give passes 1 and 2 the same fail-closed discipline the flat-sibling section already has: assert that `internal/domain/` and `internal/adapters/storage/` exist and contain at least one non-test .go file before greping them, and treat a grep exit status greater than 1 as a hard error rather than absorbing it with `|| true`. Then add two fixture cases to check-imports-test.sh — a domain package importing an adapter, and a storage package importing a client — each asserting a non-zero exit and the specific error message, plus a case with a renamed domain directory asserting the new fail-closed error. Deriving the module path from `go.mod` instead of hardcoding it would close the rename variant at its source.

**Blast radius:**

- `scripts/check-imports.sh`
- `scripts/check-imports-test.sh`
- `make check / CI`

**Acceptance criteria:**

- [ ] `bash scripts/check-imports-test.sh` passes and its output names at least two new cases covering the domain -> adapters and storage -> clients checks.
- [ ] A fixture in which internal/domain/ has been renamed causes `scripts/check-imports.sh` to exit non-zero with an explicit error naming the missing path, instead of printing 'Architecture check passed'.
- [ ] A fixture containing a domain package that imports an adapter package causes `scripts/check-imports.sh` to exit non-zero and name the offending file.
- [ ] `make check` still passes on the unmodified repository tree.

### FU-26 — Sanction the platform→domain leaf allowlist in the dependency rule and enforce it

**Severity:** low · **Effort:** S · **Rolls up:** ARCH-002

> **Controller note.** Verifier lowered medium→low. The title's 'depend on each other' reads as structural coupling that does not exist: all seven upward edges target domain/constants, domain/errors, or domain/observability — three dependency-free leaf packages in the precise sense internal/README.md defines. No import cycle exists and Go would reject one. The actual defect is that the documented inward-only arrow does not describe the code, and nothing would notice if a platform package started importing something heavier than a leaf. Adjacent to SLA-48 (#610), CLOSED and pre-baseline, which defined the leaf taxonomy this finding leans on.

#### ARCH-002 — internal/platform and internal/domain depend on each other, contradicting the documented inward-only dependency rule, and nothing enforces the boundary

*Lens:* `architecture` · *Confidence:* `strong` · *Verdict:* `confirmed_lower_severity` · *Severity:* low

**Subject:** `{'kind': 'package', 'identity': 'internal/platform <-> internal/domain (layer boundary)'}`

**Verifier correction:** The title's 'depend on each other' is technically true of the two directories but reads as a structural coupling problem that does not exist. Every one of the seven upward edges targets domain/constants, domain/errors, or domain/observability — three dependency-free leaf packages, in the precise sense internal/README.md defines (transitive closure excludes the inventory hub). No import cycle exists, and Go would reject one if it did, so nothing here impedes changing either layer. What is actually wrong is that the documented arrow does not describe the code and no check would notice if a platform package started importing something heavier than a leaf. That is a documentation-and-enforcement gap rather than a medium-severity architecture defect, and the finding's own proposed fix (option (a): sanction the three-package allowlist in the Dependency Rule and add a check) is the right shape for it. Acceptance criteria are provable as written.

**Evidence:**

- internal/README.md places PLATFORM strictly below DOMAIN and states the dependency rule as flowing inward only, listing `domain -> platform` as the permitted edge and naming no reverse edge as legal.
  - `internal/README.md (Dependency Rule block)`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/README.md | grep -n -A4 'Dependency Rule'`
- Seven non-test files under internal/platform/ import internal/domain/ packages, which is the upward edge the rule does not allow.
  - `internal/platform/cardutil/normalize.go:19`
  - Reproduce: `git grep -n --fixed-strings '"github.com/guarzo/slabledger/internal/domain/' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- 'internal/platform/*' | grep -v _test.go`
- The downward edge exists simultaneously, so the two layer groups are mutually dependent. internal/platform/cardutil is on both sides of the boundary at once: it imports internal/domain/constants and is imported by internal/domain/inventory.
  - `internal/domain/inventory/matching.go:8`
  - Reproduce: `git grep -n --fixed-strings '"github.com/guarzo/slabledger/internal/platform/' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- 'internal/domain/*'`
- No check guards this boundary in either direction. scripts/check-imports.sh mentions internal/platform only in prose — a comment and an error-message string — and never scans it.
  - `scripts/check-imports.sh:6,51`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:scripts/check-imports.sh | grep -n 'internal/platform'`
- The documented taxonomy is itself unsettled on where these packages belong, which is why the drift went unnoticed: internal/README.md's platform structure block lists `errors/` under internal/platform, but the errors package lives at internal/domain/errors at this baseline, and internal/platform has no errors directory.
  - Reproduce: `git ls-tree -r --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/platform | xargs -n1 dirname | sort -u`

**Proposed fix:** Decide the taxonomy explicitly and then make it checkable. Either (a) accept that internal/domain/{constants,errors,observability} are a shared bottom layer that platform may depend on, and say so in internal/README.md's Dependency Rule with that exact three-package allowlist; or (b) relocate them under internal/platform/ so the drawn arrow is true. Either way, add a pass to scripts/check-imports.sh that scans internal/platform/ for imports of internal/domain/ and fails on anything outside the sanctioned set, so the boundary stops depending on reviewer memory.

**Blast radius:**

- `internal/platform/cardutil`
- `internal/platform/config`
- `internal/platform/resilience`
- `internal/platform/telemetry`
- `internal/README.md`
- `scripts/check-imports.sh`

**Acceptance criteria:**

- [ ] internal/README.md's Dependency Rule block states the platform -> domain policy explicitly, and the packages it names match `git grep -n --fixed-strings '"github.com/guarzo/slabledger/internal/domain/' -- 'internal/platform/*'` exactly.
- [ ] `scripts/check-imports.sh` exits non-zero on a fixture where a package under internal/platform/ imports a domain package outside the sanctioned set, and the new case is covered in `scripts/check-imports-test.sh`.
- [ ] `go build ./... && go test ./...` pass, and `make check` passes, after whichever of (a) or (b) is chosen.

### FU-27 — Give the CardLadder handler a consumer-defined interface instead of the concrete adapter

**Severity:** medium · **Effort:** S · **Rolls up:** ARCH-003

> **Controller note.** It depends on the concrete persistence adapter, breaking the consumer-defined-interface pattern every other handler in the package follows — so the fix has an in-package reference implementation to copy.

#### ARCH-003 — The CardLadder HTTP handler depends on the concrete persistence adapter, breaking the consumer-defined-interface pattern every other handler in the package follows

*Lens:* `architecture` · *Confidence:* `strong` · *Verdict:* `confirmed` · *Severity:* medium

**Subject:** `{'kind': 'symbol', 'identity': 'handlers.CardLadderHandler.store (*postgres.CardLadderStore)'}`

**Evidence:**

- CardLadderHandler holds a concrete *postgres.CardLadderStore and its constructor takes one, so an inbound adapter is compile-time coupled to a specific outbound adapter rather than to a seam it owns.
  - `internal/adapters/httpserver/handlers/cardladder.go:31,47`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/adapters/httpserver/handlers/cardladder.go | sed -n '28,48p'`
- The inconsistency is internal to the same struct: three of its six dependencies (refresher, purchaseLister, syncUpdater) are locally-defined interfaces, and only the store and client are concrete. The deviation is not a house style, it is an outlier.
  - `internal/adapters/httpserver/handlers/cardladder.go:16`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/adapters/httpserver/handlers/cardladder.go | sed -n '15,26p'`
- The package's established pattern is the opposite and is documented in place: pricing_api.go declares the seam it needs, names the concrete implementor in a comment, and does not import postgres at all.
  - `internal/adapters/httpserver/handlers/pricing_api.go:14`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/adapters/httpserver/handlers/pricing_api.go | sed -n '14,19p'`
- cardladder.go is the only file in the entire httpserver tree that imports the persistence adapter. Every other handler that needs storage reaches it through a locally-declared interface (dh_handler.go:136,141,148 carry the same 'Satisfied by *postgres.X' comments with no postgres import).
  - `internal/adapters/httpserver/handlers/cardladder.go:11`
  - Reproduce: `git grep -nE '"github\.com/guarzo/slabledger/internal/adapters/storage' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- 'internal/adapters/httpserver/*'`
- The seam is small and well-bounded — the handler calls seven methods on the store — so the coupling buys nothing that an interface would not.
  - Reproduce: `git grep -ohE 'h\.store\.[A-Za-z0-9_]+' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- 'internal/adapters/httpserver/handlers/cardladder*.go' | sort -u`
- Nothing detects this. scripts/check-imports.sh forbids exactly one adapter-to-adapter edge — storage importing clients — and says nothing about an inbound adapter importing storage, so the rule's own stated rationale ('adapters should communicate through domain interfaces') is enforced in one direction only.
  - `scripts/check-imports.sh:42`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:scripts/check-imports.sh | sed -n '42,53p'`

**Proposed fix:** Declare a CardLadderStore interface in the handlers package covering the seven methods the handler actually calls, following the pricing_api.go pattern including the 'Satisfied by *postgres.CardLadderStore' comment, and change the field and NewCardLadderHandler parameter to that interface. The concrete type continues to be supplied at the wiring site in cmd/slabledger. This also drops the last httpserver -> storage import, which makes an httpserver-scoped rule enforceable in a follow-up.

**Blast radius:**

- `internal/adapters/httpserver/handlers/cardladder.go`
- `internal/adapters/httpserver/handlers/cardladder_sync.go`
- `cmd/slabledger (wiring site)`
- `internal/adapters/httpserver/handlers tests`

**Acceptance criteria:**

- [ ] `git grep -nE '"github\.com/guarzo/slabledger/internal/adapters/storage' -- 'internal/adapters/httpserver/*'` returns no output.
- [ ] `go build ./... && go test -race ./...` pass with the handler depending on the interface rather than *postgres.CardLadderStore.
- [ ] The handler's tests construct it with a mock satisfying the new interface, with no postgres import in the test file.
- [ ] `make check` passes.


---

## Tier: Naming & structure

### FU-28 — Put the unit on the five unsuffixed sell-sheet money fields

**Severity:** low · **Effort:** S · **Rolls up:** NAM-001

> **Controller note.** ADJUDICATED — first filing, NOT a duplicate and NOT a regression. The 2026-08-07 run recorded the frontend half as pointer FE-008 and deliberately never ticketed it (suspected-tier, `ticketable: false`, ADJUDICATIONS.md:377); it has no entry in linear-ids.json, so there is no open ticket to duplicate and no closed one to regress against. This run reached the producing Go side — five fields in inventory/analytics_types.go against FE-008's two TS fields. Verifier lowered medium→low: the finding argued an unsuffixed field 'actively asserts USD' per CLAUDE.md:194, but the repo emits cents over the wire pervasively with a Cents suffix and genuinely-USD fields carry their own explicit suffix. The real convention is 'the suffix carries the unit', so these fields record NO unit rather than asserting dollars, and no consumer reads them — a latent hazard, not a live defect.

#### NAM-001 — Five cent-denominated sell-sheet money fields carry no unit in either the Go name or the JSON tag, in a repo whose stated convention makes an unsuffixed API field mean USD

*Lens:* `naming-and-boundaries` · *Confidence:* `strong` · *Verdict:* `confirmed_lower_severity` · *Severity:* low

**Subject:** `{'kind': 'type', 'identity': 'internal/domain/inventory.SellSheetItem.TargetSellPrice, .MinimumAcceptPrice; internal/domain/inventory.SellSheetTotals.TotalCostBasis, .TotalExpectedRevenue, .TotalProjectedProfit'}`

**Verifier correction:** Evidence claim 4 overstates the convention. The finding argues an unsuffixed JSON field 'actively asserts the wrong unit' because CLAUDE.md:194 says API responses use USD. That does not hold at this baseline: the repo emits cents over the wire pervasively with a Cents tag suffix, and the handful of genuinely-USD API fields carry their own explicit suffix (campaigns_analytics.go:68-69 spendUSD/capUSD from `float64(d.SpendCents) / 100.0`; analytics_types.go:202-203 overrideTotalUsd/suggestionTotalUsd). The real convention is 'the suffix carries the unit', so these five fields record no unit anywhere rather than asserting dollars. Severity lowered to low on that basis plus the finding's own concession that no consumer reads the fields, so there is no live defect — this is a latent hazard for the next consumer, not a bug. The defect itself, and the exhaustiveness of the exception set, are confirmed.

**Evidence:**

- Five money-carrying fields in SellSheetItem/SellSheetTotals declare no unit in the Go field name and no unit in the JSON tag, while every money sibling in the same two structs (BuyCostCents, CostBasisCents, CLValueCents, OverridePriceCents, ComputedPriceCents, AISuggestedPriceCents) carries the Cents suffix in both.
  - `internal/domain/inventory/analytics_types.go:123-142`
  - Reproduce: `git grep -nE '^\s+(TargetSellPrice|MinimumAcceptPrice|TotalCostBasis|TotalExpectedRevenue|TotalProjectedProfit)\s' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/domain/inventory/analytics_types.go`
- The values are unambiguously cents: MinimumAcceptPrice is assigned MarketSnapshot.ConservativeCents, TargetSellPrice is assigned purchase.CLValueCents and purchase.OverridePriceCents and is then copied into item.ComputedPriceCents, TotalCostBasis accumulates item.CostBasisCents, and TotalExpectedRevenue accumulates item.TargetSellPrice.
  - `internal/domain/export/service_sell_sheet.go:47-180`
  - Reproduce: `git grep -nE 'item\.(TargetSellPrice|MinimumAcceptPrice)|Totals\.(TotalCostBasis|TotalExpectedRevenue)' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/domain/export/service_sell_sheet.go`
- The frontend mirrors all five as bare `number` with no unit in the identifier and no comment, so nothing on the consuming side records the denomination either. Grep across web/src returns only the type declarations — no component reads them, so no live 100x display bug is being claimed here, only the naming hazard for the next consumer.
  - `web/src/types/campaigns/market.ts:101-122`
  - Reproduce: `git grep -nE 'targetSellPrice|minimumAcceptPrice|totalCostBasis|totalExpectedRevenue|totalProjectedProfit' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- web/`
- The repo's declared convention is 'Backend uses cents internally, API responses use USD (dollars)'. Under that rule an unsuffixed JSON field on an API response reads as dollars, so these five tags actively assert the wrong unit rather than merely omitting it.
  - `CLAUDE.md:194`
  - Reproduce: `git grep -nE 'cents internally' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- CLAUDE.md`

**Proposed fix:** Rename the five Go fields to TargetSellPriceCents, MinimumAcceptPriceCents, TotalCostBasisCents, TotalExpectedRevenueCents, TotalProjectedProfitCents, and add the matching Cents suffix to their JSON tags plus the mirrored TypeScript members in web/src/types/campaigns/market.ts. Because no web/src component reads any of the five, the JSON tags can be renamed rather than kept for compatibility — but confirm no non-TS consumer (Playwright spec, external client) reads them before changing the tags; if one does, rename only the Go fields and leave the tags.

**Blast radius:**

- `internal/domain/inventory/analytics_types.go`
- `internal/domain/export/service_sell_sheet.go`
- `web/src/types/campaigns/market.ts`
- `docs/API.md (sell-sheet response shape, if the JSON tags change)`

**Acceptance criteria:**

- [ ] `go build ./...` and `go test -race ./...` pass after the rename.
- [ ] `git grep -nE '\b(TargetSellPrice|MinimumAcceptPrice|TotalCostBasis|TotalExpectedRevenue|TotalProjectedProfit)\b' -- '*.go'` returns no results (every remaining occurrence carries the Cents suffix).
- [ ] `cd web && npm run build && npm test` pass with the TypeScript members renamed in step with the JSON tags.
- [ ] Every money-carrying field in SellSheetItem and SellSheetTotals has a name ending in Cents, Usd, or Pct.

### FU-29 — Rename EbayChannelMarginPct to match the combined marketplace margin it holds

**Severity:** low · **Effort:** S · **Rolls up:** NAM-002

> **Controller note.** It names a combined eBay+TCGPlayer margin, and the misnomer has already propagated into internal/domain/portfolio as `ebayMarginPct` plus a second same-named field — so the rename has a blast radius worth scoping before starting.

#### NAM-002 — EbayChannelMarginPct names a combined eBay+TCGPlayer marketplace margin; the misnomer has already propagated into internal/domain/portfolio as `ebayMarginPct` and a second same-named field

*Lens:* `naming-and-boundaries` · *Confidence:* `strong` · *Verdict:* `confirmed` · *Severity:* low

**Subject:** `{'kind': 'symbol', 'identity': 'internal/domain/inventory.CampaignHealth.EbayChannelMarginPct'}`

**Verifier correction:** The finding accepted the declaration comment's premise that the JSON name is 'retained for frontend compatibility' and preserved the tag on that basis. That premise is unverified and appears false at this baseline: `git grep -nE 'EbayChannelMarginPct|ebayChannelMarginPct' 06d9f8ce` over all tracked files returns hits only in internal/domain/{inventory,portfolio} Go files — no web/ consumer of the JSON field exists. Keeping the tag is still the conservative fix and costs nothing, but the fixer should know the compatibility constraint is not evidenced, and the comment asserting it is itself stale. Does not change severity.

**Evidence:**

- The producer aggregates eBay AND TCGPlayer sales into locals it names marketplaceRevenue/marketplaceNetProfit, then stores the resulting ratio into a field named EbayChannelMarginPct. The author's own local naming shows the concept is 'marketplace', not 'eBay'.
  - `internal/domain/portfolio/service.go:242-257`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/domain/portfolio/service.go | sed -n '236,260p'`
- The declaration itself records that the name is wrong, but attributes the constraint to frontend JSON compatibility — which binds the JSON tag, not the Go identifier.
  - `internal/domain/inventory/health_types.go:39`
  - Reproduce: `git grep -nE 'EbayChannelMarginPct' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/domain/inventory/health_types.go`
- Consumers outside the declaring package re-narrow the value back to eBay: internal/domain/portfolio binds it to locals named ebayMarginPct in two files and copies it into a second struct field also named EbayChannelMarginPct, and internal/domain/portfolio/suggestion_rules.go gates a suggestion on it and feeds it to expectedROIFromMargin. None of these sites sees the corrective comment.
  - `internal/domain/portfolio/service.go:124`
  - Reproduce: `git grep -nE '\bEbayChannelMarginPct\b|\bebayMarginPct\b' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- 'internal/domain/portfolio/*.go' | grep -v _test.go`

**Proposed fix:** Rename the Go field to MarketplaceChannelMarginPct (and the derived locals ebayMarginPct to marketplaceMarginPct), keeping the `json:"ebayChannelMarginPct"` tag as-is so the frontend contract the comment protects is untouched. Delete the now-redundant 'JSON field name retained' clause from the comment and keep the definition clause.

**Blast radius:**

- `internal/domain/inventory/health_types.go`
- `internal/domain/portfolio/service.go`
- `internal/domain/portfolio/snapshot.go`
- `internal/domain/portfolio/suggestion_rules.go`
- `internal/domain/portfolio/analysis_types.go`
- `internal/domain/portfolio/service_test.go and suggestions_test.go (field literals)`

**Acceptance criteria:**

- [ ] `go build ./...` and `go test -race ./...` pass after the rename.
- [ ] `git grep -n 'EbayChannelMarginPct' -- '*.go'` returns no results.
- [ ] `git grep -n 'ebayChannelMarginPct' -- '*.go' web/` still shows the JSON tag and any frontend consumer unchanged, proving the wire contract did not move.
- [ ] The declaration comment states the eBay+TCGPlayer definition without claiming the Go identifier is compatibility-constrained.


---

## Tier: Interface segregation & size

### FU-30 — Split the 20-method auth.Service interface

**Severity:** medium · **Effort:** L · **Rolls up:** SIZE-001

> **Controller note.** Twenty methods across six unrelated concerns, and every consumer needing two of them hand-rolls a 20-method fake — the fakes are the evidence the segregation is missing. Same type as DUP-003, deliberately kept separate: that one is about a duplicated constant and a misplaced double, this one is about the interface shape. Merging them yields a PR no single review can land.

#### SIZE-001 — auth.Service is 20 methods across six unrelated concerns, and every consumer that needs two of them hand-rolls a 20-method fake

*Lens:* `size-and-complexity` · *Confidence:* `strong` · *Verdict:* `confirmed` · *Severity:* medium

**Subject:** `{'kind': 'type', 'identity': 'internal/domain/auth.Service'}`

**Verifier correction:** One overstatement in evidence claim 3's parenthetical. It characterizes the three inline mocks as stubbing 20 methods 'to exercise 2, 2 and 4 real methods respectively' and generalizes to '18 no-op stub methods written three times over'. That is right for middleware/auth_test.go and scheduler/session_cleanup_test.go, but handlers/auth_test.go covers handlers/auth.go, which calls 11 of the 20 (`git grep -ohE 'h\.authService\.[A-Za-z]+' 06d9f8ce -- internal/adapters/httpserver/handlers/auth.go | sort -u`: ConsumeOAuthState, CreateSession, DeleteSession, ExchangeCodeForTokens, GetLoginURL, GetOrCreateUser, GetUserInfo, IsEmailAllowed, SetUserAdmin, StoreOAuthState, StoreTokens). So that file carries 9 no-op stubs, not 18. The headline claim — no consumer in the repo uses the interface as a whole, maximum is 11 of 20 — survives, and the proposed narrow seams for the scheduler, middleware and admin handler are unaffected. The fixer should expect the auth handler to keep a wide seam.

**Evidence:**

- auth.Service declares 20 methods, grouped by its own comments into six concerns: OAuth flow, session management, user management, token storage, email allowlist, admin.
  - `internal/domain/auth/service.go:20`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/domain/auth/service.go | awk '/^type Service interface \{/,/^\}/' | grep -cE '^\t[A-Z][A-Za-z0-9]*\('`
- Three separate test packages each declare their own inline mockAuthService implementing all 20 methods. This also breaches CLAUDE.md's stated rule 'Mocks: Import from internal/testutil/mocks/ - never create inline mocks'; internal/testutil/mocks/ has no auth.Service mock (only auth_repository.go and auth_inmemory.go, which mock auth.Repository).
  - Reproduce: `git grep -c 'func (m \*mockAuthService)' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- '*_test.go'`
- The scheduler consumer calls exactly 2 of the 20 methods, yet its test must stub all 20. This is the concrete cost: 18 no-op stub methods written three times over to exercise 2, 2 and 4 real methods respectively.
  - `internal/adapters/scheduler/session_cleanup.go:18`
  - Reproduce: `git grep -oE 's\.authService\.[A-Za-z]+' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/adapters/scheduler/session_cleanup.go | sort -u`
- The middleware consumer calls 2 of 20 (GetOrCreateUser, ValidateSession) and the admin handler calls 4 of 20 (AddAllowedEmail, ListAllowedEmails, ListUsers, RemoveAllowedEmail) — no consumer in the repo uses the interface as a whole.
  - Reproduce: `git grep -ohE '(m|h)\.authService\.[A-Za-z]+' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/adapters/httpserver/middleware/auth.go internal/adapters/httpserver/handlers/admin.go | sort -u`
- Two production types must implement all 20 methods, so adding a seventh concern to the interface costs 2 production implementations plus 3 test fakes.
  - Reproduce: `git grep -nE 'func \([a-z]+ \*[A-Za-z]+\) GetLoginURL' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- 'internal/**/*.go'`

**Proposed fix:** Leave auth.Service as the composite the composition root wires, but declare narrow consumer-side interfaces at each point of use, as the repo already does elsewhere (e.g. dhpricing.NewService takes PurchaseLookup / DHPriceUpdater / DHPriceWriter seams rather than one wide store). Concretely: a 2-method SessionJanitor for scheduler/session_cleanup.go, a 2-method SessionValidator for middleware/auth.go, and a 4-method AdminDirectory for handlers/admin.go. Each test then fakes 2-4 methods instead of 20, and the three inline mockAuthService declarations collapse. Alternatively, if a shared fake is preferred, add one auth.Service mock to internal/testutil/mocks/ per the stated convention — that removes the duplication but not the width.

**Blast radius:**

- `internal/domain/auth/service.go`
- `internal/adapters/scheduler/session_cleanup.go`
- `internal/adapters/scheduler/session_cleanup_test.go`
- `internal/adapters/httpserver/middleware/auth.go`
- `internal/adapters/httpserver/middleware/auth_test.go`
- `internal/adapters/httpserver/handlers/admin.go`
- `internal/adapters/httpserver/handlers/admin_test.go`
- `internal/adapters/httpserver/handlers/auth_test.go`
- `internal/adapters/clients/google/oauth.go`
- `internal/domain/auth/service_impl.go`

**Acceptance criteria:**

- [ ] `go build ./...` and `go test -race ./...` pass after the change.
- [ ] `git grep -c 'func (m \*mockAuthService)' -- '*_test.go'` returns either no matches or counts of 4 or fewer per file, proving no test still stubs 20 methods to exercise 2.
- [ ] `var _ auth.Service = (*google.OAuthService)(nil)` still compiles, proving the composite interface was not silently narrowed out from under its production implementations.
- [ ] `make check` passes (lint + import check + file size + Playwright version).

### FU-31 — Bring core_types.go back under the size budget before it hard-fails CI

**Severity:** medium · **Effort:** M · **Rolls up:** SIZE-003

> **Controller note.** It emerged from the SLA-32 file split already at 562 lines — 38 from the 600-line hard fail — on the one file every inventory feature must touch, so the next feature to land plausibly breaks the build.

#### SIZE-003 — core_types.go emerged from the SLA-32 file split already at 562 lines — 38 from the CI hard fail — on the one file every inventory feature must touch

*Lens:* `size-and-complexity` · *Confidence:* `strong` · *Verdict:* `confirmed` · *Severity:* medium

**Subject:** `{'kind': 'symbol', 'identity': 'internal/domain/inventory/core_types.go'}`

**Verifier correction:** Two immaterial date/extent slips. Evidence claim 2 says the file 'was created two days before this baseline'; `git log -1 --date=short c624b9a7` and the Run Card both give 2026-08-08, the same day as the baseline commit. Evidence claim 3 gives Purchase as lines 274-373 (100 lines); the closing brace is at 371, so it is 98 lines. Neither affects the conclusion or the fix.

**Evidence:**

- core_types.go is 562 lines, the largest non-excluded source file in the repo and the closest to the 600-line hard fail. It is one of six files in the WARN band; there are zero FAILs at baseline.
  - Reproduce: `mkdir -p /tmp/sizecheck && git archive 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 | tar -x -C /tmp/sizecheck && cd /tmp/sizecheck && bash scripts/check-file-size.sh`
- The file was created two days before this baseline by commit c624b9a7, a refactor whose stated purpose was to split inventory files by contents (SLA-32) — and it landed at 562 lines, already in the WARN band. The split axis that produced it is therefore already spent: this file is the residue after the obvious extractions were made.
  - Reproduce: `git log -1 --format='%h %ad %s' --date=short c624b9a7 && git show c624b9a7:internal/domain/inventory/core_types.go | wc -l`
- The file holds the hub's shared persisted types — Campaign, Purchase, Sale, and 18 more type declarations — so almost any inventory feature that persists a new field must edit it. Purchase alone spans 100 lines (274-373).
  - `internal/domain/inventory/core_types.go:274`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/domain/inventory/core_types.go | grep -cE '^type '`
- The maintenance cost is a CI failure landing on an unrelated author: 38 lines of headroom is roughly one feature that adds a handful of fields to Campaign plus Purchase plus Sale. Whoever's PR crosses 600 must split the hub's core type file as an unplanned prerequisite, and the check hard-fails rather than warning.
  - `scripts/check-file-size.sh:7`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:scripts/check-file-size.sh | grep -nE 'FAIL_LIMIT=|exit 1'`

**Proposed fix:** Split core_types.go along the aggregate boundary the file already exhibits: move Sale, BulkSaleInput, BulkSaleResult and BulkSaleError (lines 465-536) into a sale_types.go, and the activation/revocation group (ActivationCheck, ActivationChecklist, RevocationFlag, lines 537-562) into campaign_activation_types.go. Both are pure type moves within the same package, so no import changes anywhere. That leaves core_types.go around 460 lines, back under the 500 guideline with real headroom, without disturbing Campaign/Purchase — the two types with the most call sites.

**Blast radius:**

- `internal/domain/inventory/core_types.go`

**Acceptance criteria:**

- [ ] `bash scripts/check-file-size.sh` no longer lists internal/domain/inventory/core_types.go in its WARN output.
- [ ] `go build ./...` and `go test -race ./internal/domain/inventory/...` pass — a pure intra-package type move must not change any import or any behavior.
- [ ] `git diff --stat` shows only additions and deletions of type declarations, with no edits to any type body, confirming the change is a move rather than a rewrite.
- [ ] `make check` passes.

### FU-32 — Close the two blind spots in the file-size check

**Severity:** medium · **Effort:** M · **Rolls up:** SIZE-002, SIZE-004

> **Controller note.** One ticket, one script, two ways it fails to measure what it exists to measure. SIZE-002: cmd/slabledger/main.go is exempted by name with no ceiling and no stated reason, and holds the largest function in the repository. SIZE-004: the two largest domain functions each sit in a file just under the 500-line warn line, so a per-file budget reports nothing about them. Both argue the check should measure functions, not only files.

#### SIZE-002 — The size check exempts cmd/slabledger/main.go by name with no ceiling and no stated reason, and that file holds the largest function in the repository

*Lens:* `size-and-complexity` · *Confidence:* `strong` · *Verdict:* `confirmed` · *Severity:* low

**Subject:** `{'kind': 'symbol', 'identity': 'cmd/slabledger/main.go (unconditional exemption in scripts/check-file-size.sh)'}`

**Verifier correction:** Trivial off-by-one: runServer's signature is at main.go:149, not :150, so the extent is 149-418 = 270 lines rather than 269. The lens's awk started at NR>=150 and so measured from the body's first line. Nothing turns on it — the ordering and the 'largest function in the repo' claim are unaffected.

**Evidence:**

- scripts/check-file-size.sh excludes cmd/slabledger/main.go by exact path. The script comments the *_test.go exclusion ('table-driven tests grow naturally') but gives no reason for the main.go or testutil exclusions.
  - `scripts/check-file-size.sh:20`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:scripts/check-file-size.sh | grep -n 'main.go'`
- The exemption is effective: main.go is absent from the set the check iterates, so no line count of any size would ever produce a WARN or FAIL for it.
  - Reproduce: `mkdir -p /tmp/sizecheck && git archive 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 | tar -x -C /tmp/sizecheck && cd /tmp/sizecheck && find internal/ cmd/ -name '*.go' ! -name '*_test.go' ! -path '*/testutil/*' ! -path 'cmd/slabledger/main.go' -type f | grep -c '^cmd/slabledger/main.go$'`
- runServer occupies lines 150-418 of main.go — 269 lines, the single largest function in all non-test, non-testutil Go under internal/ and cmd/, and 64% of the exempted file.
  - `cmd/slabledger/main.go:150`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:cmd/slabledger/main.go | awk 'NR>=150 && /^}$/{print "runServer: lines 150-" NR " = " NR-150+1 " lines"; exit}'`
- The exemption is a single-file special case, not a directory policy: main.go's siblings in the same package (handlers.go, init_schedulers.go, init_inventory_services.go, server.go) are all inside the checked set and subject to the 500/600 budget.
  - Reproduce: `git ls-tree -r --name-only 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- cmd/slabledger | grep -v _test.go`

**Proposed fix:** Replace the unconditional `! -path 'cmd/slabledger/main.go'` exemption with either (a) no exemption — main.go is 418 lines at this baseline and passes the 500 guideline today, so removing the special case costs nothing and restores the ceiling; or (b) if the exemption is deliberate, a comment stating why, matching the style of the *_test.go comment two lines above. Separately, extracting runServer's distinct phases (metrics server setup, auth wiring, DH wiring, scheduler deps assembly) into named helpers alongside the existing initializeCampaignsService / initializeSchedulers / createHandlers would bring the largest function in the repo in line with the composition-root helpers it already calls.

**Blast radius:**

- `scripts/check-file-size.sh`
- `cmd/slabledger/main.go`

**Acceptance criteria:**

- [ ] `bash scripts/check-file-size.sh` exits 0 with the exemption removed, proving main.go is under the 600 hard limit without the special case.
- [ ] `make check` passes.
- [ ] If runServer is also split: `go build ./cmd/slabledger` succeeds and `./slabledger --help` still prints usage, proving startup wiring was not reordered.

#### SIZE-004 — The two largest domain functions each sit in a file just under the 500-line warn line, so the enforced size budget reports nothing about them

*Lens:* `size-and-complexity` · *Confidence:* `strong` · *Verdict:* `confirmed` · *Severity:* medium

**Subject:** `{'kind': 'symbol', 'identity': 'internal/domain/inventory.(*service).ImportCerts'}`

**Verifier correction:** Two of evidence claim 3's sub-counts are prose with no command behind them and are slightly off when checked. Early exits: `grep -cE '^\t{2,}(continue|return|break)'` over the ImportCerts body returns 13, not the stated 14. The 'thirteen concerns in one function body' enumeration has no reproducible command at all and is the lens's reading of the code — defensible from the 29 branches and 6 indentation levels I did verify, but it is judgment, not measurement. Neither weakens the finding; the falsifiable core (two longest domain functions, both in files just under the warn line, invisible to the enforced check) is fully confirmed.

**Evidence:**

- ImportCerts spans lines 63-296 of service_cert_entry.go — 234 lines, 48% of a 491-line file. The file is 9 lines below the 500-line WARN threshold, so `make check` is silent about it.
  - `internal/domain/inventory/service_cert_entry.go:63`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/domain/inventory/service_cert_entry.go | wc -l && git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/domain/inventory/service_cert_entry.go | awk 'NR>=63 && /^}$/{print "ImportCerts: 63-" NR " = " NR-63+1 " lines"; exit}'`
- The same shape repeats one package over: ImportPSAExportGlobal is 239 lines inside a 498-line file — two lines under the warn line. The two largest domain functions in the repo are both hidden by a metric measured per file rather than per function.
  - `internal/domain/csvimport/service_import_psa.go:26`
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/domain/csvimport/service_import_psa.go | wc -l && git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/domain/csvimport/service_import_psa.go | awk 'NR>=26 && /^}$/{print "ImportPSAExportGlobal: 26-" NR " = " NR-26+1 " lines"; exit}'`
- ImportCerts is not merely long: it carries 29 control-flow branches, 14 early exits from a single loop body, and 6 levels of indentation, all mutating one shared `result` accumulator. Its responsibilities are input dedup, campaign bootstrap, two batch lookups, sold-item detection, export-flag and received-at writes, DH push enrollment, circuit-breaker latching, PSA category/name normalization, purchase construction, duplicate-cert recovery, two enrichment queues, and a background goroutine — thirteen concerns in one function body.
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/domain/inventory/service_cert_entry.go | awk 'NR>=63 && NR<=296' | grep -cE '^\t+(if|for|switch|case) '`
- The maintenance cost is measurable in the test suite: exercising this one function requires service_cert_entry_test.go, the largest test file in the repository at 1288 lines, which names ImportCerts 38 times. Every new branch inside the function multiplies against the setup those tests already carry.
  - Reproduce: `git show 06d9f8ce172dada98f307f860e2e2985c9fb5ca6:internal/domain/inventory/service_cert_entry_test.go | wc -l && git grep -c 'ImportCerts' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- internal/domain/inventory/service_cert_entry_test.go`
- 1288 lines is the maximum across every *_test.go in the repo, so this is the worst case rather than a typical one.
  - Reproduce: `git grep -c '' 06d9f8ce172dada98f307f860e2e2985c9fb5ca6 -- '*_test.go' | awk -F: '{print $3, $2}' | sort -rn | head -3`

**Proposed fix:** Extract the two arms of ImportCerts' loop body into named methods on the service: `handleExistingCert(ctx, existing, certNum, now, salesMap, result)` for the already-in-inventory branch (lines 108-157) and `importNewCert(ctx, certNum, now, result) (imported bool)` for the lookup-and-create branch (lines 159-281), leaving ImportCerts as dedup, batch lookups, loop, and the background resolver kick-off — roughly 60 lines. This is the same decomposition csvimport already applies elsewhere (handleExistingPSAPurchase is a named method extracted from the import path). Apply the equivalent split to ImportPSAExportGlobal. Separately, consider whether the size budget should measure functions as well as files — the current per-file metric is what allowed both of these to grow unremarked.

**Blast radius:**

- `internal/domain/inventory/service_cert_entry.go`
- `internal/domain/inventory/service_cert_entry_test.go`
- `internal/domain/csvimport/service_import_psa.go`
- `internal/domain/csvimport/service_import_psa_global_test.go`

**Acceptance criteria:**

- [ ] `go test -race ./internal/domain/inventory/... ./internal/domain/csvimport/...` passes with no changes to the existing test assertions — the extraction must be behavior-preserving, and the 1288-line test file is the proof harness.
- [ ] An awk scan for top-level function extents over the changed files shows no function above 120 lines.
- [ ] The circuit-breaker latch is still batch-wide after the split: a test where the first cert lookup returns ErrCodeProviderCircuitOpen must still mark every remaining cert Retryable without issuing further lookups (the psaUnavailable flag must be threaded, not duplicated per-call).
- [ ] `make check` passes.


---

## Investigation tier — recorded, deliberately not ticketed

These are real observations that a developer could not act on and prove correct from the finding alone. They are kept so a later pass starts from them rather than rediscovering them.

### ARCH-004 — POINTER (docs lens): internal/README.md's architecture sections name packages that no longer exist at this baseline

*Verdict:* `confirmed` · *Lens:* `architecture` · *Confidence:* `suspected`

**Why not ticketed:** Judged as what it is — a pointer handing a docs-category lead to the docs lens — so the test is whether the lead is sound and specific enough to act on, not whether it stands as a full finding. All three legs reproduce under my own commands. internal/README.md names cache/ (lines 52, 240) and errors/ (lines 56, 243) under internal/platform, and `git ls-tree -r --name-only 06d9f8ce -- internal/platform | xargs -n1 dirname | sort -u` returns only canonjson, cardutil, config, crypto, resilience, storage, telemetry — neither exists. README:138 cites `advisor -> ai` and `advisor -> scoring` as worked examples of legal edges, and `git ls-tree -r --name-only 06d9f8ce -- internal/domain/advisor internal/domain/ai internal/domain/scoring` returns empty, consistent with commit e46370e5. The derived governed-sibling set is eleven — arbitrage, csvimport, demand, dhlisting, dhpricing, export, finance, portfolio, pricing/lookup, psacampaign, tuning — against the ten CLAUDE.md enumerates; csvimport is the omission, and since the checker derives membership from the tree, csvimport is genuinely governed while going unlisted. low/suspected is the correct filing and the pointer stays in its lane.

**What the finding got wrong:** Worth flagging for whoever actions it, though it does not weaken the lead: CLAUDE.md's sibling sentence is prefixed with 'derive it rather than trusting this sentence' and the surrounding section tells the reader to compute the set, so the ten-vs-eleven gap is a self-disclaimed staleness rather than a doc asserting something false. The README items are unqualified and are the substantive half.

### DB-006 — 000021's rollback recreates mm_card_mappings and mm_sales_comps with no RLS statement at all, unlike the three later drop-rollbacks which all restore it — a rollback past 000021 leaves those tables outside the RLS regime every other table is under

*Verdict:* `confirmed` · *Lens:* `db-schema` · *Confidence:* `strong`

**Why not ticketed:** The defect holds for one of the two tables and I could not refute that half. I read 000021_drop_market_movers.down.sql in full: it recreates mm_card_mappings and mm_sales_comps with structure and one index, and `git grep -c -iE 'ROW LEVEL SECURITY|CREATE POLICY|REVOKE' 06d9f8ce -- .../000021_drop_market_movers.down.sql` returns empty output — no RLS statement of any kind. mm_card_mappings genuinely had a policy in the state 000021 destroyed (000003_supabase_security_and_perf_fixes.up.sql:173), and nothing in 000021.down restores it, so a sequential rollback to 000020 brings that table back without the RLS state it had. That is a real reversibility gap. Two qualifications that bound it. First, the security consequence is thin: the state being lost is 000003's `USING (true)` policy with no TO clause, which admits anon and authenticated anyway (this is exactly what 000027-000029 were written to correct), so what is lost is regime consistency, not an access boundary — low is the right severity. Second, and materially, the finding's second table does not support it (see below).

**What the finding got wrong:** mm_sales_comps never had RLS, so recreating it without RLS is a faithful reversal, not a regression. `git grep -n 'mm_sales_comps' 06d9f8ce -- internal/adapters/storage/postgres/migrations` shows it created by 000005_add_comp_sources.up.sql:2 with no RLS statement, dropped by 000021.up:8, and absent from 000027_rls_remaining_tables — which ran after 000021, when the table no longer existed. At the 000020 state that 000021.down is supposed to restore, mm_sales_comps had no RLS at all. This makes acceptance criteria 1 and 2 wrong as written: AC 1 demands rowsecurity = true for both tables, and AC 2 demands that neither anon nor authenticated hold any privilege on either recreated table — a state stricter than 000003's TO PUBLIC policy that mm_card_mappings actually had at 000020. Both contradict the finding's own exclusion, which correctly defends 000028.down's `USING (true)` recreation on the grounds that a down-migration's job is to restore the exact prior state. A developer working from these criteria alone would write a down-migration that also fails to reverse 000021, in the opposite direction. Hence ticketable: false — the defect is real and worth filing, but the acceptance criteria must be rewritten to cover mm_card_mappings only and to target 000003's actual policy shape before this can be handed to anyone.

### DCG-006 — Pointer to naming-and-boundaries: dh_fusion.go is named after the removed price-fusion engine but contains live, unrelated DH mocks

*Verdict:* `confirmed` · *Lens:* `dead-code-go` · *Confidence:* `suspected`

**Why not ticketed:** Both evidence commands reproduce exactly, including the single-hit output. I established the substantive claim by my own route in two halves. First, the file is misnamed: `git show 06d9f8ce:internal/testutil/mocks/dh_fusion.go` contains only MockDHMarketDataClient and MockDHCardIDLookup, test doubles for dhprice.MarketDataClient and dhprice.CardIDLookup, with no fusion concept anywhere in the body. Second — and this is the half that matters for a pointer, since a rename recommendation would be wrong if the contents were themselves dead — I confirmed the contents are live: `git grep -nE '\b(MockDHMarketDataClient|MockDHCardIDLookup)\b' 06d9f8ce -- '*.go' ':!internal/testutil/mocks'` returns 27 real construction sites across internal/adapters/clients/dhprice/provider_test.go. The finding correctly files this as a naming pointer at `suspected`, not as dead code in its own lane.

**What the finding got wrong:** The acceptance criteria bundle two separate objects: the rename of dh_fusion.go, and correcting an unrelated stale comment in a different file (internal/adapters/scheduler/inventory_refresh_test.go:342, which is the only 'fusion' hit in Go source). A fixer can satisfy both, but they are not one change.

### DCT-004 — The visual-regression suite is permanently skipped — its gate env var is set nowhere in the repository, yet CI runs the spec on every PR and on the browser matrix

*Verdict:* `confirmed_lower_severity` · *Lens:* `docs-config-tests` · *Confidence:* `mechanical`

**Why not ticketed:** The finding's first evidence entry claims 'Every one of the six describe blocks in the spec is gated on RUN_VISUAL_REGRESSION.' It is not. I counted by reading the source, not by substring: `git show 06d9f8ce:web/tests/e2e/visual-regression.spec.ts | grep -nE 'describe\(|test\(|test.skip|toHaveScreenshot'` gives six describes at lines 46, 84, 122, 133, 169, 195 and only five test.skip guards at 50, 126, 137, 173, 199. Reading lines 84-119 in full: `test.describe('Visual Regression - Components @visual')` has a beforeEach and two tests and NO skip guard at all. Those two tests are 'Header navigation' (toHaveScreenshot('header-navigation.png'), :98) and 'Search form' (toHaveScreenshot('search-form.png'), :114) — which are precisely the two names carried by the ten committed baselines under web/tests/e2e/visual-regression.spec.ts-snapshots/, five per name, one per Playwright project. Those baselines are live: browser-matrix.yml:82/84/174 runs the unfiltered suite per browser and per device, which is why five project-specific PNGs exist for each. So the third evidence claim ('They can never be compared against anything') and the fourth ('none of which has ever run in CI at this revision') are both false. What does survive, verified independently: `git grep -n 'RUN_VISUAL_REGRESSION' 06d9f8ce` returns five hits, all guards in that one spec, and nothing sets the variable — no workflow, no Makefile target, no package.json script, no .env.example entry. So 4 of 6 describe blocks and 6 of 8 toHaveScreenshot assertions are gated on a variable with no setter anywhere in the tree. That is a real but much smaller defect than filed.

**What the finding got wrong:** Three of its four evidence claims are wrong about scope. The 'Visual Regression - Components' describe block (visual-regression.spec.ts:84) carries no gate, so 2 of the 8 screenshot assertions DO execute in CI, and the ten committed baseline PNGs are live comparison targets, not orphans. The correct statement is: 6 of 8 screenshot assertions (4 of 6 describe blocks) are permanently skipped because RUN_VISUAL_REGRESSION is set nowhere. I am marking this not ticketable because a fixer acting on the filed text — in particular its acceptance criterion 'If removed: git grep -n RUN_VISUAL_REGRESSION returns empty' — could delete the ten baselines that the browser matrix is actually asserting against on every run. Re-scope before ticketing.

### DCT-012 — POINTER (dead-code-go / frontend-health): two unregistered API paths are asserted inside Go source, one of them rendered to users

*Verdict:* `confirmed` · *Lens:* `docs-config-tests` · *Confidence:* `suspected`

**Why not ticketed:** Judged as a lead, per the controller's instruction. Both citations are accurate at 06d9f8ce and both paths are genuinely unregistered. internal/adapters/httpserver/handlers/api_status_handler.go:30 reads `// apiUsageResponse is the JSON response for GET /api/status/api-usage.` internal/adapters/httpserver/handlers/spa_handler.go:133 emits `<li><a href="/api/sets">/api/sets</a> - List available sets</li>` into the fallback HTML page. Neither /api/status/api-usage nor /api/sets appears in routes.go's 127 registered path literals or router.go's three; `git grep -nE '"/api/(sets|favorites|cards/search|campaigns/cash-flow)' <rev> -- internal cmd` returns only spa_handler.go:133 and middleware/security_headers_test.go:188. The lead is sound and correctly routed out of the docs lane for the spa_handler half.

**What the finding got wrong:** The dead-code framing is wrong for the first of the two items. The pointer asks 'whether the handler behind the first is reachable at all' — but that handler IS registered and live: routes.go:43 wires rt.apiStatusHandler.HandleAPIUsage behind /api/admin/api-usage under RequireAdmin. So there is no reachability question there at all, only a stale path string in a doc-comment on the apiUsageResponse type (which is itself live, being the marshalled response). Only the spa_handler.go:133 half is a genuine reachability lead, and even that is one step removed: the question is whether the fallback HTML page is ever served, not whether the symbol is referenced. dead-code-go should be told the first half is a comment fix, not a reachability investigation, or it will spend a pass proving a live handler live. Not ticketable on its own by construction — it is a pointer whose acceptance criterion is that the owning lens absorbs it.

### DUP-005 — The "invalid campaign fee falls back to the default marketplace rate" rule exists in three places inside one call graph, and two of the three disagree on what counts as invalid

*Verdict:* `confirmed` · *Lens:* `duplication` · *Confidence:* `suspected`

**Why not ticketed:** All three predicates are where the finding says. inventory.EffectiveFeePct (channel_fees.go:16-20) rejects <=0 and >=1; arbitrage/acquisition.go:41-43 restates that predicate inline; arbitrage/expected_value.go:57-60 implements only `if in.FeePct > 0`, with no upper bound, so an out-of-range fee that acquisition would replace with the default is used verbatim by EV — I read expected_value.go:50-75 and confirmed feePct flows straight into `expectedFees := int(expectedSale * feePct)` at :71 with no further clamp. The latency claim also holds: my own `git grep -nE 'computeAcquisitionOpportunity\(|computeExpectedValue\(|EffectiveFeePct\(' 06d9f8ce -- internal/domain/arbitrage | grep -v _test.go` shows every production call site (service_batch.go:136, :209 via the map built at :16; service_ev.go:67, :131, :146 each preceded by an EffectiveFeePct call at :66, :130, :144) already passes a sanitized value, so neither inline guard can fire today.

**What the finding got wrong:** Nothing substantive. Evidence item 4's `output` field is a prose paraphrase of the command's result rather than its literal text (it summarizes service_batch.go:136 and :209 in a sentence); the command itself reproduces and the substance is accurate, but a verifier expecting verbatim output would flag the mismatch. Filed at suspected, which is the correct tier for a divergence that no current caller can reach.

### FE-006 — Pointer to docs-config-tests: internal/README.md illustrates the flat-sibling leaf rule with three domain packages that do not exist at this baseline

*Verdict:* `confirmed` · *Lens:* `frontend-health` · *Confidence:* `suspected`

**Why not ticketed:** Judged as a pointer, i.e. whether the lead is sound. It is. git show 06d9f8ce:internal/README.md:137-139 presents `advisor -> ai`, `advisor -> scoring` alongside four live edges as current legal examples, with no past-tense framing. I enumerated the actual package set rather than grepping for absence: `git ls-tree -r --name-only 06d9f8ce -- internal/domain | sed 's|internal/domain/||' | cut -d/ -f1 | sort -u` yields arbitrage, auth, constants, csvimport, demand, dhevents, dhlisting, dhpricing, errors, export, finance, intelligence, inventory, liquidation, llmutil, mathutil, observability, portfolio, pricing, psacampaign, storage, timeutil, tuning. Neither advisor, ai, nor scoring is among them, so three of the six illustrative edges name packages that do not exist. The same stale trio survives in docs/specs/2026-08-08-domain-leaf-taxonomy-design.md:40-41, :68, :79-80 and docs/plans/2026-08-08-flat-sibling-target-based-enforcement.md:539, which the pointer does not mention and the docs lens should sweep with it.

**What the finding got wrong:** Nothing. I agree with the controller's disposition: this is a docs-category pointer covering the same internal/README.md material the docs lens already filed, so it should be folded into that finding rather than ticketed on its own — hence ticketable false here, which reflects duplication, not doubt about the lead.

### NAM-004 — Twelve money fields carry the unit in the JSON tag but not in the Go identifier, making the repo's Cents-suffix convention unreliable to read against

*Verdict:* `confirmed` · *Lens:* `naming-and-boundaries` · *Confidence:* `suspected`

**Why not ticketed:** The single evidence command re-runs at 06d9f8ce and returns exactly the twelve quoted lines, no more and no fewer. I checked the adjacency claims by reading the declarations: health_types.go:31 CapitalAtRisk sits between HealthReason and the correctly-suffixed LiquidationLossCents / InHandCapitalCents / InTransitCapitalCents block, and arbitrage/expected_value.go:17-19 sit immediately above CarryingCostCents at :20. Both hold.

**What the finding got wrong:** Nothing factually. I am marking it not ticketable rather than disputing it: the lens filed it at `suspected`, which LENS-BRIEF §5 defines as recorded-not-ticketed, and I agree with that self-assessment on the merits. Unlike NAM-001, the unit is recorded at the declaration line by the adjacent JSON tag, and unlike NAM-002 the Go name is incomplete rather than false — it omits a unit, it does not assert a wrong one. There is no consequence a developer could name beyond consistency, so this is the 'inelegant name is not debt' case my brief warns about. It is worth carrying in the report as a cleanup-of-opportunity for whoever next edits those four files, not as a branch of its own.

### NAM-005 — POINTER for dead-code-go: internal/domain/llmutil survives the Azure AI / advisor removal with its only consumer being its own test

*Verdict:* `confirmed` · *Lens:* `naming-and-boundaries` · *Confidence:* `suspected`

**Why not ticketed:** Both evidence commands reproduce exactly. Independently: `git grep -nl 'llmutil' 06d9f8ce` over all tracked files returns two Go files (internal/domain/llmutil/strip_fences.go and its own external test package strip_fences_test.go) plus only documentation and audit artifacts; `git grep -nE 'StripMarkdownFences' 06d9f8ce` returns hits only inside that package and the Phase 1 Go map. So the lead is sound as a lead. The lens correctly declined to assert reachability and correctly routed it as a pointer under LENS-BRIEF §6.

**What the finding got wrong:** It is a duplicate. I verified the overlap the dispatch flagged rather than assuming it: docs/audit/runs/2026-08-08/findings/dead-code-go.json DCG-001 has subject.identity `internal/domain/llmutil` and the title 'internal/domain/llmutil is a whole orphaned package: its only exported function StripMarkdownFences has no consumer outside its own test', filed at `mechanical` — the tier NAM-005 explicitly could not reach. The owning lens has therefore already done the reachability work this pointer asked for, and NAM-005 adds nothing to it. Controller: collapse NAM-005 into DCG-001 and ticket only DCG-001. One detail DCG-001 may not carry: CLAUDE.md lists llmutil in its leaf-utilities line, so a deletion needs that line updated; NAM-005's blast_radius omits it too.


---

## Refuted findings — the audit's own error rate

One of 49 findings did not survive adversarial verification. One was **mechanical-tier** — the audit's highest confidence band. This section stays in the record permanently.

### DCT-007 — docs/polish-report.md is a two-line orphaned checkpoint stub from a tool run in April 2026

*Claimed confidence:* `mechanical` · *Lens:* `docs-config-tests`

**How it was refuted:** The file-content half is accurate — `git show 06d9f8ce:docs/polish-report.md` is exactly two lines ('# Polish-All Report' / 'Base commit: ec4d261b... | Started: 2026-04-20') and `git ls-tree -r -l` gives 96 bytes. The orphan half fails, and it is an absence claim, so it needed a command with empty output. The finding records `git grep -n 'polish-report' 06d9f8ce -- ':!docs/audit'` as '(second command: empty — no inbound reference anywhere outside this audit's own artifacts)'. I ran that exact command and it exits 0 with five hits, all in a tracked file: .claude/skills/polish-all/SKILL.md:60, :97, :249, :277, :295. I re-ran it unfiltered (`git grep -n 'polish-report' 06d9f8ce`) and got the same five. The references are not incidental — SKILL.md:97 says 'If docs/polish-report.md doesn't exist (or --fresh), create it with:', :249 and :277 append to it, and :60 reads and prints it under --report. The file is therefore the live checkpoint artifact of a tracked skill that recreates it on demand, not an orphan. The finding's own acceptance criterion 'No document links to it — git grep -n polish-report returns empty outside .gitignore' cannot be satisfied without also editing that skill, which the finding neither mentions nor scopes.

**What it got wrong:** The load-bearing claim, 'nothing links to it', is false, and the evidence entry asserting an empty grep does not reproduce — the command returns five hits in .claude/skills/polish-all/SKILL.md, which both creates and reads the file. Refuted per the brief's rule that an absence claim requires a command whose output is actually empty. Whatever residue is left (a 96-byte stub committed from an April 2026 tool run) is cosmetic and its removal is coupled to a skill definition the finding does not account for.

See `docs/audit/runs/2026-08-08/ADJUDICATIONS.md` for the controller's full reasoning on DCT-007.

