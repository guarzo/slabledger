# CardLadder Pricing `gemRateId` → `profileId` Fix — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore CardLadder pricing, sales comps, and collection-push by reading/sending CardLadder's new `profileId` identifier and `grade` field instead of the removed `gemRateId`/`gemRateCondition`/`condition` fields.

**Architecture:** Centralize the new-contract translation at the single choke point `Client.BuildCollectionCard`: map `profileId`→`GemRateID` and `grade`→`GemRateCondition`, and derive the display-form `Condition` ("PSA 8") from `grade` ("g8") right after unmarshal. Every downstream consumer (scheduler refresh, collection push) then sees a fully-populated struct identical in shape to before, so their code is unchanged. Sales comps switch their filter key and storage identifier to `profileId`. DB columns and Go field names keep the `gemRateId` name (now holding a profileId) — no schema migration.

**Tech Stack:** Go 1.26, hexagonal architecture, `httptest`-based table tests, Postgres (`jackc/pgx/v5`), live CardLadder Firebase Cloud Functions + Cloud Run search.

---

## Background: the identifier split (live-verified 2026-06-21)

| Identifier | Example | Now used by |
|---|---|---|
| **profileId** | `psa-1813135` | `httpbuildcollectioncard` output, `httpcardestimate` input, `salesarchive` filter key |
| **hash gemRateId** | `fb123c6ac87154de3bab52bcf3f90ecacbc760fc` | DH direct-lookup hint; still echoed on CL comp rows as `gemRateId` + `universalGemRateId` |

`httpbuildcollectioncard` no longer returns `gemRateId`, `gemRateCondition`, or `condition`. It returns `profileId` and `grade` (firestore form, e.g. `"g8"`).

**The two condition forms (critical):**
- **Display form** `"PSA 8"` — stored in `cl_card_mappings.cl_condition`; required by the catalog/estimate lookup keying (see `cardladder_sync.go:82-84`).
- **Firestore form** `"g8"` — the new API's `grade`; also what `httpcardestimate` wants as its `condition` input and what `cl_sales_comps.condition` stores.

Helpers already exist in `internal/platform/cardutil/grade.go`:
- `ConditionToAPIFormat("g8") → "PSA 8"` (firestore → display)
- `DisplayConditionToGFormat("PSA 8") → "g8"` (display → firestore)
- `firestoreConditionFor("PSA 8") → "g8"` lives in `cardladder_helpers.go` (used at the estimate call site).

---

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `internal/adapters/clients/cardladder/types.go` | API request/response structs | Re-tag `BuildCardResponse` (`profileId`/`grade`); re-tag `CardEstimateRequest` (`profileId`); add `SaleComp.ProfileID` |
| `internal/adapters/clients/cardladder/functions.go` | Cloud Function callables | Derive display `Condition` from `grade` inside `BuildCollectionCard` |
| `internal/adapters/clients/cardladder/client.go` | Cloud Run search | `FetchSalesComps` filter key `gemRateId:` → `profileId:` |
| `internal/adapters/clients/cardladder/firestore.go` | Collection push | No logic change (works via centralized derivation); verify live |
| `internal/adapters/scheduler/cardladder_gap_fill.go` | Decoupled comp fetch/store | Store `comp.ProfileID`; set filter condition form per empirical gate |
| `internal/adapters/httpserver/handlers/dh_match_handler.go` | DH bulk match | Comment only (hint is now a profileId; DH soft-degrades) |
| `internal/adapters/clients/dh/types_v2.go` | DH request types | Comment only |
| `internal/platform/cardutil/grade_test.go` | Grade-form round-trip | New test |
| `internal/adapters/clients/cardladder/functions_test.go` | Client callable tests | New-contract test + update estimate test |
| `internal/adapters/clients/cardladder/client_test.go` | Search tests | Assert `profileId:` filter; `ProfileID` on comp |
| `docs/runbooks/cl-profileid-backfill.md` | Ops | New: one-time backfill SQL + verification |

**No change needed** (confirmed by centralizing derivation): `cardladder_refresh.go` `resolveGemRate`, `cardladder_catalog.go` `fetchCLEstimate`, `cardladder_helpers.go`, all storage/postgres files, all domain types.

---

### Task 1: New-contract parse for `BuildCardResponse` (Path A core)

**Files:**
- Modify: `internal/adapters/clients/cardladder/types.go:124-148`
- Modify: `internal/adapters/clients/cardladder/functions.go:13-36`
- Test: `internal/adapters/clients/cardladder/functions_test.go`

- [ ] **Step 1: Write the failing test (new-contract raw JSON)**

Add to `functions_test.go`. This simulates the REAL new API: raw JSON with `profileId`+`grade`, and no `gemRateId`/`gemRateCondition`/`condition`.

```go
func TestClient_BuildCollectionCard_NewContract(t *testing.T) {
	// Real httpbuildcollectioncard shape as of the 2026-06 CL migration:
	// profileId + grade present; gemRateId / gemRateCondition / condition absent.
	raw := `{"result":{"profileId":"psa-1813135","grade":"g8","player":"Articuno-Holo",` +
		`"set":"Pokemon Japanese Web","year":"1999","number":"17","category":"Pokemon","pop":42}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(raw)) //nolint:errcheck
	}))
	defer server.Close()

	client := NewClient(WithFunctionsURL(server.URL), WithStaticToken("test-token"))
	resp, err := client.BuildCollectionCard(context.Background(), "158507531", "psa")
	if err != nil {
		t.Fatalf("BuildCollectionCard failed: %v", err)
	}
	if resp.GemRateID != "psa-1813135" {
		t.Errorf("GemRateID = %q, want psa-1813135 (from profileId)", resp.GemRateID)
	}
	if resp.GemRateCondition != "g8" {
		t.Errorf("GemRateCondition = %q, want g8 (from grade)", resp.GemRateCondition)
	}
	if resp.Condition != "PSA 8" {
		t.Errorf("Condition = %q, want PSA 8 (derived display form)", resp.Condition)
	}
	if resp.Set != "Pokemon Japanese Web" {
		t.Errorf("Set = %q, want Pokemon Japanese Web", resp.Set)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/clients/cardladder/ -run TestClient_BuildCollectionCard_NewContract -v`
Expected: FAIL — `GemRateID = ""` (still mapped to the absent `gemRateId`) and `Condition = ""`.

- [ ] **Step 3: Re-tag the response struct**

In `types.go`, change `BuildCardResponse` (lines 124-140). Map `profileId` and `grade`; keep the `Condition` field (now populated in code, not from JSON — the API no longer sends `condition`):

```go
// BuildCardResponse is the result from httpbuildcollectioncard.
//
// As of the 2026-06 CardLadder migration the variant identifier is profileId
// (e.g. "psa-1813135") and the grade arrives in firestore form via "grade"
// (e.g. "g8"). The old gemRateId / gemRateCondition / condition fields are gone.
// GemRateID/GemRateCondition field names are retained to avoid a wide rename;
// GemRateID now carries the profileId. Condition (display form, "PSA 8") is
// derived in BuildCollectionCard, not sent by the API.
type BuildCardResponse struct {
	Pop              int    `json:"pop"`
	Year             string `json:"year"`
	Set              string `json:"set"`
	Category         string `json:"category"`
	Number           string `json:"number"`
	Player           string `json:"player"`
	Variation        string `json:"variation"`
	Condition        string `json:"-"`
	ImageURL         string `json:"imageUrl"`
	ImageBackURL     string `json:"imageBackUrl"`
	GemRateID        string `json:"profileId"`
	GemRateCondition string `json:"grade"`
	SlabSerial       string `json:"slabSerial"`
	GradingCompany   string `json:"gradingCompany"`
}
```

- [ ] **Step 4: Derive display condition in `BuildCollectionCard`**

In `functions.go`, update `BuildCollectionCard` (lines 15-25) to populate `Condition` from `grade`. Add the cardutil import.

Change the import block at the top of `functions.go`:

```go
import (
	"context"
	"encoding/json"
	"fmt"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/platform/cardutil"
)
```

Change the function body:

```go
func (c *Client) BuildCollectionCard(ctx context.Context, cert, grader string) (*BuildCardResponse, error) {
	var resp callableResponse[BuildCardResponse]
	err := c.doCallable(ctx, "httpbuildcollectioncard", BuildCardRequest{
		Cert:   cert,
		Grader: grader,
	}, &resp)
	if err != nil {
		return nil, fmt.Errorf("build collection card for cert %s: %w", cert, err)
	}
	// The API returns grade in firestore form ("g8"); derive the display form
	// ("PSA 8") that cl_condition and the card label require. Centralizing this
	// here keeps every downstream consumer (refresh, push) unchanged.
	if resp.Result.Condition == "" && resp.Result.GemRateCondition != "" {
		resp.Result.Condition = cardutil.ConditionToAPIFormat(resp.Result.GemRateCondition)
	}
	return &resp.Result, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/adapters/clients/cardladder/ -run TestClient_BuildCollectionCard_NewContract -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/clients/cardladder/types.go internal/adapters/clients/cardladder/functions.go internal/adapters/clients/cardladder/functions_test.go
git commit -m "fix(cardladder): parse profileId/grade from buildcollectioncard

CL migrated the variant identifier to profileId and grade. Map them into
the existing GemRateID/GemRateCondition fields and derive the display-form
Condition centrally in BuildCollectionCard so refresh + push are unchanged."
```

---

### Task 2: Re-tag `CardEstimateRequest` to send `profileId` (Path A)

**Files:**
- Modify: `internal/adapters/clients/cardladder/types.go:142-148`
- Test: `internal/adapters/clients/cardladder/functions_test.go:83-140` (existing `TestClient_CardEstimate`)

- [ ] **Step 1: Update the existing test to assert the `profileId` key**

In `functions_test.go`, in `TestClient_CardEstimate`, change the request-body assertion (currently lines ~101-103) from `gemRateId` to `profileId`:

```go
		if dataMap["profileId"] != "psa-1813135" {
			t.Errorf("profileId = %v, want psa-1813135", dataMap["profileId"])
		}
```

And change the request construction (currently lines ~122-127) to use that value:

```go
	resp, err := client.CardEstimate(context.Background(), CardEstimateRequest{
		GemRateID:      "psa-1813135",
		GradingCompany: "psa",
		Condition:      "g8",
		Description:    "Test Card",
	})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/clients/cardladder/ -run TestClient_CardEstimate -v`
Expected: FAIL — `profileId = <nil>` because the field still serializes as `gemRateId`.

- [ ] **Step 3: Re-tag the request struct**

In `types.go`, change `CardEstimateRequest` (lines 142-148):

```go
// CardEstimateRequest is the input for httpcardestimate.
// GemRateID now carries the profileId (e.g. "psa-1813135") and serializes as
// "profileId" — the live API ignores the old "gemRateId" key and returns null.
type CardEstimateRequest struct {
	GemRateID      string `json:"profileId"`
	GradingCompany string `json:"gradingCompany"`
	Condition      string `json:"condition"`
	Description    string `json:"description"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/clients/cardladder/ -run TestClient_CardEstimate -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/clients/cardladder/types.go internal/adapters/clients/cardladder/functions_test.go
git commit -m "fix(cardladder): send profileId to httpcardestimate

The estimate callable ignores the old gemRateId key and returns null;
serialize the identifier as profileId."
```

---

### Task 3: Lock the grade-form round-trip contract (defensive)

**Files:**
- Create: `internal/platform/cardutil/grade_test.go`

This guards the derivation chain `grade("g8") → ConditionToAPIFormat → "PSA 8" → firestoreConditionFor/DisplayConditionToGFormat → "g8"` that Task 1 depends on.

- [ ] **Step 1: Write the round-trip test**

```go
package cardutil

import "testing"

func TestGradeFormRoundTrip(t *testing.T) {
	tests := []struct {
		gForm   string
		display string
	}{
		{"g8", "PSA 8"},
		{"g10", "PSA 10"},
		{"g8_5", "PSA 8.5"},
	}
	for _, tc := range tests {
		t.Run(tc.gForm, func(t *testing.T) {
			gotDisplay := ConditionToAPIFormat(tc.gForm)
			if gotDisplay != tc.display {
				t.Fatalf("ConditionToAPIFormat(%q) = %q, want %q", tc.gForm, gotDisplay, tc.display)
			}
			gotG := DisplayConditionToGFormat(gotDisplay)
			if gotG != tc.gForm {
				t.Fatalf("DisplayConditionToGFormat(%q) = %q, want %q", gotDisplay, gotG, tc.gForm)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./internal/platform/cardutil/ -run TestGradeFormRoundTrip -v`
Expected: PASS (these helpers already exist and are correct; this pins the contract).

- [ ] **Step 3: Commit**

```bash
git add internal/platform/cardutil/grade_test.go
git commit -m "test(cardutil): pin grade-form round-trip used by CL profileId fix"
```

---

### Task 4: Sales-comp filter + identifier (Path B, client side)

**Files:**
- Modify: `internal/adapters/clients/cardladder/types.go:36-50` (`SaleComp`)
- Modify: `internal/adapters/clients/cardladder/client.go:124-139` (`FetchSalesComps`)
- Test: `internal/adapters/clients/cardladder/client_test.go:52-80`

- [ ] **Step 1: Update the failing test**

In `client_test.go`, rewrite `TestClient_FetchSalesComps` to assert the new `profileId:` filter key and the `ProfileID` field on the comp row:

```go
func TestClient_FetchSalesComps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("index") != "salesarchive" {
			t.Fatalf("unexpected index: %s", r.URL.Query().Get("index"))
		}
		filters := r.URL.Query().Get("filters")
		if !strings.Contains(filters, "profileId:psa-1813135") {
			t.Errorf("filters = %q, want it to contain profileId:psa-1813135", filters)
		}
		if strings.Contains(filters, "gemRateId:") {
			t.Errorf("filters = %q, must not use the dead gemRateId key", filters)
		}
		json.NewEncoder(w).Encode(SearchResponse[SaleComp]{ //nolint:errcheck
			Hits: []SaleComp{
				{ItemID: "ebay-123", Price: 135, Platform: "eBay", ListingType: "Auction",
					ProfileID: "psa-1813135", GemRateID: "fb123c6ac8", Condition: "g8"},
			},
			TotalHits: 1,
		})
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL+"/search"),
		WithStaticToken("test-token"),
	)
	comps, err := client.FetchSalesComps(context.Background(), "psa-1813135", "g8", "psa", 0, 100)
	if err != nil {
		t.Fatalf("FetchSalesComps failed: %v", err)
	}
	if len(comps.Hits) != 1 {
		t.Fatalf("got %d hits, want 1", len(comps.Hits))
	}
	if comps.Hits[0].ProfileID != "psa-1813135" {
		t.Errorf("ProfileID = %q, want psa-1813135", comps.Hits[0].ProfileID)
	}
}
```

Add `"strings"` to the `client_test.go` import block if not already present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/clients/cardladder/ -run TestClient_FetchSalesComps -v`
Expected: FAIL — filter still contains `gemRateId:`, and `SaleComp` has no `ProfileID` field (compile error).

- [ ] **Step 3: Add `ProfileID` to `SaleComp`**

In `types.go`, add the field to `SaleComp` (after `GemRateID` at line 47):

```go
// SaleComp represents one sold listing from the salesarchive index.
type SaleComp struct {
	ItemID          string  `json:"itemId"`
	Date            string  `json:"date"`
	Price           float64 `json:"price"`
	Platform        string  `json:"platform"`
	ListingType     string  `json:"listingType"`
	Seller          string  `json:"seller"`
	Feedback        int     `json:"feedback"`
	URL             string  `json:"url"`
	SlabSerial      string  `json:"slabSerial"`
	CardDescription string  `json:"cardDescription"`
	ProfileID       string  `json:"profileId"` // new lookup key (psa-<n>)
	GemRateID       string  `json:"gemRateId"` // legacy hash, retained for debug; NOT the lookup key
	Condition       string  `json:"condition"`
	GradingCompany  string  `json:"gradingCompany"`
}
```

- [ ] **Step 4: Switch the filter key to `profileId`**

In `client.go`, change the `filters` line in `FetchSalesComps` (line 131). The `gemRateID` parameter name is retained but now carries a profileId:

```go
// FetchSalesComps fetches sales comps for a card+grade. The first argument is
// the profileId (the post-2026-06 CL lookup key), passed through the param
// historically named gemRateID.
func (c *Client) FetchSalesComps(ctx context.Context, gemRateID, condition, grader string, page, limit int) (*SearchResponse[SaleComp], error) {
	params := url.Values{
		"index":   {"salesarchive"},
		"query":   {""},
		"page":    {strconv.Itoa(page)},
		"limit":   {strconv.Itoa(limit)},
		"filters": {fmt.Sprintf("condition:%s|profileId:%s|gradingCompany:%s", condition, gemRateID, grader)},
		"sort":    {"date"},
	}
	var resp SearchResponse[SaleComp]
	if err := c.doGet(ctx, params, &resp); err != nil {
		return nil, fmt.Errorf("fetch sales comps for %s: %w", gemRateID, err)
	}
	return &resp, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/adapters/clients/cardladder/ -run TestClient_FetchSalesComps -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/clients/cardladder/types.go internal/adapters/clients/cardladder/client.go internal/adapters/clients/cardladder/client_test.go
git commit -m "fix(cardladder): filter salesarchive by profileId; add SaleComp.ProfileID

The salesarchive index moved the lookup key to profileId (old gemRateId
returns 0 hits). Comp rows still carry a legacy hash gemRateId that differs
from profileId, so add ProfileID as the canonical key."
```

---

### Task 5: Store comps under `profileId` + condition-form gate (Path B, scheduler side)

**Files:**
- Modify: `internal/adapters/scheduler/cardladder_gap_fill.go:48-69`

**Why this matters:** `campaign_purchases.gem_rate_id` will now hold profileIds. The comp-refresh join (`comp_refresh.go`: `sc.gem_rate_id = cp.gem_rate_id`) only matches if `cl_sales_comps.gem_rate_id` also holds the profileId. Storing the comp-row hash would silently match nothing.

- [ ] **Step 1: EMPIRICAL GATE — verify which condition form the filter wants**

The index document stores `condition:"g8"` (g-form). The current gap-fill code converts to display form via `ConditionToAPIFormat`. Confirm which the live filter accepts before editing. Run (creds in `/workspace` env):

```bash
TOKEN=$(curl -s -X POST \
  "https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key=${CL_FIREBASE_API_KEY}" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"${CL_EMAIL}\",\"password\":\"${CL_PASSWORD}\",\"returnSecureToken\":true}" \
  | python3 -c "import sys,json;print(json.load(sys.stdin)['idToken'])")
SEARCH="https://search-zzvl7ri3bq-uc.a.run.app/search"
for COND in "g8" "PSA 8"; do
  printf 'condition:%s -> ' "$COND"
  curl -s -G "$SEARCH" -H "Authorization: Bearer ${TOKEN}" \
    --data-urlencode "index=salesarchive" --data-urlencode "query=" \
    --data-urlencode "page=0" --data-urlencode "limit=5" \
    --data-urlencode "filters=condition:${COND}|profileId:psa-1813135|gradingCompany:psa" \
    --data-urlencode "sort=date" \
    | python3 -c "import sys,json;print('totalHits =', json.load(sys.stdin).get('totalHits'))"
done
```

Known: `condition:g8` returns `totalHits = 4`. Decide from output:
- **Branch A** — `condition:PSA 8` ALSO returns hits → the existing `ConditionToAPIFormat` conversion is fine; do only the `comp.ProfileID` storage change in Step 2.
- **Branch B** — only `condition:g8` returns hits → also pass g-form to the filter (Step 2 alternate).

Evidence to date points to Branch B (the indexed document stores `"g8"`), but confirm before choosing.

- [ ] **Step 2: Apply the comp storage + (gated) condition change**

In `cardladder_gap_fill.go`, the loop currently reads (lines 48-69):

```go
		apiCondition := cardutil.ConditionToAPIFormat(card.Condition)
		if apiCondition == "" {
			continue
		}
		resp, err := client.FetchSalesComps(ctx, card.GemRateID, apiCondition, "psa", 0, 100)
		...
			if err := s.salesStore.UpsertSaleComp(ctx, postgres.CLSaleCompRecord{
				GemRateID:   comp.GemRateID,
				Condition:   card.Condition,
```

**Branch A** (filter wants display form) — change only the stored identifier:

```go
				GemRateID:   comp.ProfileID, // store the profileId so the comp_refresh join matches campaign_purchases.gem_rate_id
				Condition:   card.Condition,
```

**Branch B** (filter wants g-form) — also send g-form to the filter. `card.Condition` is already g-form (`'g' || ...` from `ListUnsoldCardsNeedingComps`), so drop the conversion:

```go
		// card.Condition is already firestore form ("g8"); the salesarchive
		// filter expects firestore form, so pass it through unconverted.
		if card.Condition == "" {
			continue
		}
		resp, err := client.FetchSalesComps(ctx, card.GemRateID, card.Condition, "psa", 0, 100)
```

and in the upsert:

```go
				GemRateID:   comp.ProfileID, // profileId, to match campaign_purchases.gem_rate_id in the comp_refresh join
				Condition:   card.Condition,
```

If Branch B removes the only use of `cardutil` in this file, remove the now-unused import `"github.com/guarzo/slabledger/internal/platform/cardutil"`.

- [ ] **Step 3: Build and vet**

Run: `go build ./... && go vet ./internal/adapters/scheduler/`
Expected: no errors (no unused import, no compile error).

- [ ] **Step 4: Run the scheduler package tests**

Run: `go test ./internal/adapters/scheduler/ -run CardLadder -v`
Expected: PASS (existing tests unaffected).

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/scheduler/cardladder_gap_fill.go
git commit -m "fix(cardladder): store comps under profileId so refresh join matches

campaign_purchases.gem_rate_id now holds profileIds; store comp.ProfileID
(not the legacy hash) in cl_sales_comps.gem_rate_id. [Branch B only: pass
firestore-form condition to the salesarchive filter.]"
```

---

### Task 6: Verify the collection-push path (Path C)

The push path (`firestore.go` `ResolveAndCreateCard`) already works through the centralized derivation: it reads `buildResp.GemRateID` (profileId), `buildResp.GemRateCondition` (`g8`, for the estimate), and `buildResp.Condition` (derived `PSA 8`, for the label/doc). The only behavior change is that the Firestore card document now writes a profileId into its `gemRateId` field (`firestore.go:305`). Confirm that's acceptable.

**Files:**
- Test: `internal/adapters/clients/cardladder/functions_test.go:142-187` (existing `TestClient_CreateCollectionCard`)

- [ ] **Step 1: Add a unit assertion that the doc carries the profileId + display condition**

Append to `TestClient_CreateCollectionCard` (after the existing assertions, before the closing brace), using the input it already builds:

```go
	if v := firestoreString(doc.Fields, "gemRateId"); v != "abc123" {
		t.Errorf("doc gemRateId = %q, want abc123 (now the profileId)", v)
	}
	if v := firestoreString(doc.Fields, "condition"); v != "PSA 9" {
		t.Errorf("doc condition = %q, want PSA 9 (display form)", v)
	}
```

- [ ] **Step 2: Run the test**

Run: `go test ./internal/adapters/clients/cardladder/ -run TestClient_CreateCollectionCard -v`
Expected: PASS (the input in this test sets `GemRateID: "abc123"`, `Condition: "PSA 9"`).

- [ ] **Step 3: Live push verification (manual, creds required)**

Build and exercise one real push, then confirm in the CL UI the created card shows the correct value and grade. Use the existing admin push flow rather than ad-hoc Firestore writes:

```bash
go build -o slabledger ./cmd/slabledger
# Trigger a single-cert push via the existing handler (see docs/API.md for the
# cardladder sync/push endpoint) against a known cert, e.g. 158507531, and
# confirm in the CardLadder collection UI that the card resolves with a value.
```

Expected: card is created with a non-zero value and grade `PSA 8`. If the CL collection UI rejects a `psa-<n>` value in its `gemRateId` doc field, STOP and report — that would mean the push path needs the legacy hash and is out of scope for the pricing fix (open a follow-up; pricing + comps are unaffected).

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/clients/cardladder/functions_test.go
git commit -m "test(cardladder): assert collection-push doc carries profileId + display condition"
```

---

### Task 7: Document the DH hint contract (no behavior change)

DH's `gemrate_id` is only a fuzzy-match-skip hint; live testing showed DH still returns the correct card when sent a profileId, and still echoes its own legacy hash. The shared column now holds a profileId, so DH loses an optimization but does not break. Record this so a future reader doesn't "fix" it.

**Files:**
- Modify: `internal/adapters/httpserver/handlers/dh_match_handler.go:126-134`
- Modify: `internal/adapters/clients/dh/types_v2.go:76`

- [ ] **Step 1: Add the handler comment**

In `dh_match_handler.go`, above the `req := dh.CertResolveRequest{` block (line ~126):

```go
		// Note: p.GemRateID now holds a CardLadder profileId (psa-<n>), not the
		// legacy hash DH uses for direct lookup. DH treats gemrate_id only as a
		// fuzzy-match-skip hint and still resolves correctly by cert_number, so
		// this degrades gracefully (loses the optimization, not the match).
		cardName, variant := dhlisting.CleanCardNameForDH(p.CardName)
```

- [ ] **Step 2: Update the type comment**

In `types_v2.go`, change the comment on line 76:

```go
	GemRateID  string `json:"gemrate_id,omitempty"` // hint to skip fuzzy matching; now receives a CL profileId (psa-<n>) — DH falls back to cert match if it doesn't recognize it
```

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: no errors (comments only).

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/httpserver/handlers/dh_match_handler.go internal/adapters/clients/dh/types_v2.go
git commit -m "docs(dh): note gemrate_id hint now receives a CL profileId"
```

---

### Task 8: Integration test against live CardLadder (creds-gated)

**Files:**
- Create: `internal/integration/cardladder_profileid_test.go`

- [ ] **Step 1: Write the integration test**

Follow the repo's `-tags integration` convention. This resolves a known cert and asserts a non-empty profileId and a plausible estimate.

```go
//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
)

// Cert 158507531 → profileId psa-1813135, grade g8, estimate ~429 (2026-06-21).
func TestCardLadder_ProfileIDContract_Live(t *testing.T) {
	key, email, pass := os.Getenv("CL_FIREBASE_API_KEY"), os.Getenv("CL_EMAIL"), os.Getenv("CL_PASSWORD")
	if key == "" || email == "" || pass == "" {
		t.Skip("CL creds not set; skipping live CardLadder integration test")
	}

	ctx := context.Background()
	auth := cardladder.NewFirebaseAuth(key)
	authResp, err := auth.Login(ctx, email, pass)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	// WithTokenManager refreshes from the refresh token; an already-expired
	// expiry forces an immediate refresh on first call.
	client := cardladder.NewClient(
		cardladder.WithTokenManager(auth, authResp.RefreshToken, time.Now().Add(-time.Hour)),
	)

	build, err := client.BuildCollectionCard(ctx, "158507531", "psa")
	if err != nil {
		t.Fatalf("BuildCollectionCard: %v", err)
	}
	if build.GemRateID != "psa-1813135" {
		t.Errorf("GemRateID = %q, want psa-1813135", build.GemRateID)
	}
	if build.Condition != "PSA 8" {
		t.Errorf("Condition = %q, want PSA 8", build.Condition)
	}

	est, err := client.CardEstimate(ctx, cardladder.CardEstimateRequest{
		GemRateID:      build.GemRateID,
		GradingCompany: "psa",
		Condition:      build.GemRateCondition, // "g8"
		Description:    build.Player,
	})
	if err != nil {
		t.Fatalf("CardEstimate: %v", err)
	}
	if est.EstimatedValue <= 0 {
		t.Errorf("EstimatedValue = %v, want > 0", est.EstimatedValue)
	}

	comps, err := client.FetchSalesComps(ctx, build.GemRateID, build.GemRateCondition, "psa", 0, 100)
	if err != nil {
		t.Fatalf("FetchSalesComps: %v", err)
	}
	if comps.TotalHits == 0 {
		t.Errorf("FetchSalesComps TotalHits = 0, want > 0 for psa-1813135/g8")
	}
}
```

- [ ] **Step 2: Confirm the auth constructor names compile**

Run: `go vet -tags integration ./internal/integration/`
Expected: clean. The test uses the real auth path — `NewFirebaseAuth(key)`, `auth.Login(ctx, email, pass)`, then `NewClient(WithTokenManager(auth, authResp.RefreshToken, <expired>))` — mirroring `internal/adapters/httpserver/handlers/cardladder.go`. If `go vet` reports an unknown symbol, reconcile against that handler before running.

- [ ] **Step 3: Run the integration test**

Run: `go test -tags integration ./internal/integration/ -run TestCardLadder_ProfileIDContract_Live -v`
Expected: PASS — `GemRateID = psa-1813135`, `Condition = PSA 8`, estimate > 0 (~429), comps > 0.

- [ ] **Step 4: Commit**

```bash
git add internal/integration/cardladder_profileid_test.go
git commit -m "test(integration): live CardLadder profileId contract (resolve+estimate+comps)"
```

---

### Task 9: Backfill runbook + full verification

**Files:**
- Create: `docs/runbooks/cl-profileid-backfill.md`

**Decision:** run the backfill as a one-time **runbook SQL step**, not an auto-running migration. Rationale: the repo's migrations are schema-DDL and run automatically on every startup; a data-nulling migration risks re-clearing on a fresh restore. The clear is a one-shot ops action.

- [ ] **Step 1: Write the runbook**

```markdown
# Runbook: CardLadder profileId backfill

After deploying the profileId fix, existing rows hold OLD hash gemRateIds (or
are empty). Clear them so the next CL refresh re-resolves to profileIds.

## When
Once, immediately after the fix is deployed to production.

## SQL (run against the production Postgres)
```sql
-- Clear cached CL mappings so resolveGemRate re-resolves to profileIds.
UPDATE cl_card_mappings   SET cl_gem_rate_id = '', cl_condition = '';

-- Clear stale identifiers on purchases (repopulated by the next refresh).
UPDATE campaign_purchases SET gem_rate_id = '' WHERE gem_rate_id <> '';
```
cl_sales_comps rows keyed by the old hash become orphaned and are superseded by
profileId-keyed rows on the next comp refresh; no manual cleanup required.

## Verify (after the next CL refresh cycle)
Check `/api/admin/cardladder/status`:
- `resolved` > 0
- `certResolveFailed` collapses toward 0
- `updated` > 0
- `cardsMapped` climbs past 34
```

- [ ] **Step 2: Full test suite with race detector**

Run: `go test -race ./...`
Expected: all packages PASS.

- [ ] **Step 3: Quality checks**

Run: `make check && golangci-lint run ./internal/adapters/clients/cardladder/... ./internal/adapters/scheduler/...`
Expected: lint clean, architecture import check passes (adapter→`platform/cardutil` is allowed), file-size check passes.

- [ ] **Step 4: Commit**

```bash
git add docs/runbooks/cl-profileid-backfill.md
git commit -m "docs(runbook): CardLadder profileId backfill + post-deploy verification"
```

- [ ] **Step 5: Push and open the PR**

```bash
git push -u origin fix/cl-pricing-profileid
gh pr create --title "fix(cardladder): migrate gemRateId → profileId pricing contract" \
  --body "$(cat <<'EOF'
## Summary
CardLadder split its card-variant identifier: `profileId` (`psa-<n>`) is now the
pricing key for buildcollectioncard / cardestimate / salesarchive, while the old
hash `gemRateId` remains only DH's direct-lookup hint. Our client read/sent the
now-absent `gemRateId`/`gemRateCondition`/`condition` fields, so every cert failed
resolution and no inventory got a CL price.

## Changes
- Parse `profileId` + `grade` from `httpbuildcollectioncard`; derive display-form
  condition centrally in `BuildCollectionCard` (refresh + push unchanged).
- Send `profileId` to `httpcardestimate`.
- Filter `salesarchive` by `profileId`; store comps under `profileId` so the
  comp-refresh join matches `campaign_purchases.gem_rate_id`.
- DH path: comment only — `gemrate_id` is a hint; DH soft-degrades.
- Reuse existing columns/field names (no schema migration); one-time backfill
  runbook clears stale identifiers for re-resolution.

## Verification
- Unit + integration tests (live resolve→estimate≈429, comps>0).
- Live API contract re-verified 2026-06-21.
- Post-deploy: `/api/admin/cardladder/status` shows resolved>0, certResolveFailed
  collapse, updated>0, cardsMapped past 34.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Self-Review

**Spec coverage:**
- Path A pricing (BuildCardResponse + CardEstimateRequest) → Tasks 1, 2 ✓
- Condition-form display/firestore contract → Tasks 1, 3 ✓
- Path B sales comps (filter + storage identifier + join integrity) → Tasks 4, 5 ✓
- Path C collection push → Task 6 ✓
- DH "investigate, don't touch" → Task 7 (comment only) ✓
- Backfill (clear + re-resolve) → Task 9 ✓
- Testing plan (unit, integration, post-deploy status) → Tasks 1-6, 8, 9 ✓
- Reuse columns / no migration → honored throughout ✓

**Placeholder scan:** No TBD/TODO/"handle errors"/"similar to". The one branch (Task 5) is an explicit empirical decision with the exact command and both concrete code outcomes — not a placeholder.

**Type consistency:** `BuildCardResponse.GemRateID`/`.GemRateCondition`/`.Condition`, `CardEstimateRequest.GemRateID`, `SaleComp.ProfileID`/`.GemRateID`, `CLSaleCompRecord.GemRateID`, `FetchSalesComps(profileId, condition, grader, page, limit)` are used consistently across tasks. `ConditionToAPIFormat`/`DisplayConditionToGFormat`/`firestoreConditionFor` match `grade.go` and `cardladder_helpers.go`.

**Open item flagged honestly:** Task 8 Step 2 verifies the real password-auth constructor name before running (I read the test patterns but not every constructor signature); Task 6 Step 3 has a STOP condition if the CL UI rejects a profileId in the push doc.
