# Implementation Notes: PSA Phase-Independent Allocation

## Confirmed facts

- PSA import currently loads all campaigns and then performs a second active-only query for matching.
- `FindMatchingCampaign` is phase-agnostic.
- `filterOutExternal` excludes by `ExternalCampaignID`.
- Scheduler contexts already carry source `scheduler`.
- The production inventory service lacks `WithPendingItemRepository`; the handler-only repository is created after scheduler initialization.

## Decisions

- Reuse the all-campaign query and filter External in the import service.
- Preserve best-effort pending-item writes.
- Create one pending repository in `initializeCampaignsService`, inject it, return it, and reuse it in handlers.

## Verification constraints

- Full tests require reachable PostgreSQL through `POSTGRES_TEST_URL` or host `postgres`.
- The two production certs require actual snapshot rows and campaign definitions for a true replay.

## Verification performed

- RED: `go test ./internal/domain/inventory -run TestService_ImportPSAExportGlobal_MatchesRealCampaignRegardlessOfPhase -count=1 -v` exited 1. The active Umbreon case passed; the pending Mega Charizard and External-guard cases each reported `allocated 0, unmatched 1`.
