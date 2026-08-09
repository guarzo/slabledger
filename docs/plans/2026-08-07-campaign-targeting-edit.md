# Campaign Targeting Edit Path Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make an existing campaign's targeting (languages, subject filter, subjects) editable from the SlabLedger UI, so the PSA portal is never the only way to change it.

**Architecture:** A second `useForm` in `CampaignsPage` seeds from the campaign being edited and saves through `PUT /api/campaigns/{id}`, sending the full fresh campaign with form values layered on top (the endpoint is a full-row replace). `CampaignsTab` gains a per-row **Edit** button and renders the edit card above the list, reusing the existing `CampaignFormFields`. Saving is local only — publishing stays the existing separate `PSAPublishModal` propose/diff flow. One Go line fixes `updated_at` so the client-side staleness guard can actually observe a harvester baseline write.

**Tech Stack:** Go 1.26 (`cmd/psa-harvest`), React 18 + TypeScript, TanStack Query, Vitest + Testing Library, Tailwind with CSS custom properties.

**Spec:** `docs/superpowers/specs/2026-08-07-campaign-targeting-edit-design.md`

## Global Constraints

- **Portal ids are copied verbatim and never re-resolved.** An existing `SubjectRef.id` must round-trip through the form byte-for-byte. Only id `0` (operator-typed name) is resolved by name at push time.
- **`-1` (`LEGACY_UNRECONCILED_SUBJECT_ID`) cannot be repaired by hand.** A real portal id may only arrive via the harvester baseline pull. Removal is allowed behind confirmation; re-adding the removed name in the same editor session is blocked.
- **`PUT /api/campaigns/{id}` is a full-row replace.** `HandleUpdateCampaign` decodes a whole `inventory.Campaign` and `campaign_store.go:295-329` sets every column. Any omitted field is written as its zero value. Always send `{ ...freshCampaign, ...formValues }` — never form values alone.
- **Do not widen the bulk-paste update format** (`CampaignsPage.tsx:43-54`) to carry targeting. Only the sentence claiming "there is currently no edit surface for it after that" changes.
- **`deniedSpecs` stays read-only in the form.** It is portal-managed per `docs/psa-harvester.md:385-390`.
- **`npm run build` does NOT type-check.** The only type gate is `npm run typecheck` (`tsc --noEmit`). Run `npm test` too.
- **Backend scope is exactly one line plus its test.** No new endpoint, no schema change, no migration.
- `web/src/types/` are manually kept in sync with Go JSON tags.
- Go: table-driven tests, `errors.Is` for sentinel errors, `go test -race` before committing.

## File Structure

| File | Created / Modified | Responsibility |
| --- | --- | --- |
| `cmd/psa-harvest/baseline.go` | Modify (`buildBaselineCampaign`) | Stamp `UpdatedAt` so a baseline write moves the row's timestamp. |
| `cmd/psa-harvest/baseline_test.go` | Modify | Pin the new timestamp behavior. |
| `web/src/js/api/campaigns.ts` | Modify | Add `getCampaign(id)` for the staleness refetch (`GET /api/campaigns/{id}` already exists at `routes.go:112`). |
| `web/src/react/queries/useCampaignQueries.ts` | Modify | Add `useUpdateCampaign()`. |
| `web/src/react/utils/campaignFormValues.ts` | **Create** | `toFormValues(c: Campaign): CreateCampaignInput` — the seed mapping, isolated so it is unit-testable and keeps `CampaignsPage` from growing. |
| `web/src/react/utils/campaignFormValues.test.ts` | **Create** | Unit tests for the seed mapping. |
| `web/src/react/ui/SubjectListEditor.tsx` | Modify | Confirm before removing a `-1` chip; block the removed name for the editor session. |
| `web/src/react/ui/SubjectListEditor.test.tsx` | Modify | New confirmation/blocking tests; update the one existing test that removes a `-1` chip. |
| `web/src/react/pages/campaigns/CampaignsTab.tsx` | Modify | Per-row **Edit** button; edit `CardShell` above the list. |
| `web/src/react/pages/campaigns/CampaignsTab.test.tsx` | Modify | New props in the harness; Edit-button and edit-card tests. |
| `web/src/react/pages/CampaignsPage.tsx` | Modify | `editing` state, second `useForm`, save handler with staleness guard, paste-comment update. |
| `web/src/react/pages/CampaignsPage.edit.test.tsx` | **Create** | Save-body, staleness-guard, and refetch-failure tests. |

`CampaignFormFields.tsx` needs **no** change: its props already include `showPhase` and `showFees`, and `EconomicsSection` already re-syncs its local inputs from props.

---

### Task 1: Harvester stamps `updated_at`

**Why first:** it is independent of the UI, and the client-side staleness guard in Task 5 is pointless without it. `runBaselinePull` (`baseline.go:206`) takes an `inventory.CampaignRepository`, not the service, so it never reaches `service_crud.go:39` where `UpdateCampaign` stamps `c.UpdatedAt = time.Now()`. `updated := existing` carries the old timestamp forward, and `campaign_store.go:314` binds it verbatim to `$16` — so a baseline write changes targeting without moving `updated_at`.

**Files:**
- Modify: `cmd/psa-harvest/baseline.go` (imports block at `:3-15`; `buildBaselineCampaign` body around `:157-161`)
- Test: `cmd/psa-harvest/baseline_test.go` (append a new top-level test function)

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces: no new exported symbols. `buildBaselineCampaign(existing inventory.Campaign, pc psacampaign.PortalCampaign) (inventory.Campaign, error)` keeps its signature.

- [ ] **Step 1: Write the failing test**

Append to `cmd/psa-harvest/baseline_test.go`:

```go
func TestBuildBaselineCampaignStampsUpdatedAt(t *testing.T) {
	// runBaselinePull writes through CampaignRepository.UpdateCampaign, which
	// does not stamp UpdatedAt the way inventory.Service.UpdateCampaign does
	// (service_crud.go:39). Without this assignment a baseline write changes a
	// row's targeting while leaving updated_at at its pre-baseline value, and
	// the UI's optimistic-concurrency check cannot see the write it exists to
	// catch.
	stale := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	existing := inventory.Campaign{
		ID: "camp-1", Name: "Vintage Core", PSACampaignRequestID: "req-1",
		UpdatedAt: stale,
	}
	pc := psacampaign.PortalCampaign{
		CampaignRequestID: "req-1",
		SpecListIDs:       []string{"uuid-en"},
		SpecListNames:     []string{"Pokemon - English Language Only"},
		SubjectFilter: psacampaign.CampaignFilter{
			Type:     "Target",
			Subjects: []psacampaign.SubjectRef{{ID: 22210, Name: "Machamp"}},
		},
	}

	before := time.Now()
	got, err := buildBaselineCampaign(existing, pc)
	if err != nil {
		t.Fatalf("buildBaselineCampaign(): unexpected error: %v", err)
	}
	if !got.UpdatedAt.After(stale) {
		t.Errorf("UpdatedAt = %v, want a time after the pre-baseline %v", got.UpdatedAt, stale)
	}
	if got.UpdatedAt.Before(before) {
		t.Errorf("UpdatedAt = %v, want at or after the call time %v", got.UpdatedAt, before)
	}
}
```

If `baseline_test.go` does not already import `time`, add it to that file's import block.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/psa-harvest/ -run TestBuildBaselineCampaignStampsUpdatedAt -v`
Expected: FAIL — `UpdatedAt = 2020-01-01 00:00:00 +0000 UTC, want a time after the pre-baseline ...`

- [ ] **Step 3: Add `time` to the imports**

In `cmd/psa-harvest/baseline.go`, the import block currently reads:

```go
import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
	"github.com/guarzo/slabledger/internal/platform/cardutil"
)
```

Add `"time"` after `"strings"`:

```go
import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
	"github.com/guarzo/slabledger/internal/platform/cardutil"
)
```

- [ ] **Step 4: Stamp the timestamp**

In `buildBaselineCampaign`, this block:

```go
	updated := existing
	updated.TargetLanguages = langs
	updated.SubjectFilterMode = pc.SubjectFilter.Type
	updated.Subjects = toTargetSubjects(pc.SubjectFilter.Subjects)
	updated.DeniedSpecs = toTargetSubjects(pc.DeniedSpecs)
```

becomes:

```go
	updated := existing
	updated.TargetLanguages = langs
	updated.SubjectFilterMode = pc.SubjectFilter.Type
	updated.Subjects = toTargetSubjects(pc.SubjectFilter.Subjects)
	updated.DeniedSpecs = toTargetSubjects(pc.DeniedSpecs)
	// This writer bypasses inventory.Service.UpdateCampaign (service_crud.go:39),
	// which is where UpdatedAt is normally stamped, and campaign_store.go:314
	// binds whatever it is given. Without this, a baseline write changes the
	// row's targeting while leaving updated_at at its pre-baseline value — and
	// updated_at is what the UI's edit form compares against to detect that
	// this pull landed underneath an open form.
	updated.UpdatedAt = time.Now()
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test -race ./cmd/psa-harvest/...`
Expected: PASS, including the pre-existing `TestBuildBaselineCampaign` table (it does not assert on `UpdatedAt`, so it is unaffected).

- [ ] **Step 6: Run the full backend suite**

Run: `go build ./... && go test ./...`
Expected: exit 0, no failures.

- [ ] **Step 7: Commit**

```bash
git add cmd/psa-harvest/baseline.go cmd/psa-harvest/baseline_test.go
git commit -m "fix(psa-harvest): stamp UpdatedAt on baseline campaign writes"
```

---

### Task 2: Data layer — `getCampaign` and `useUpdateCampaign`

**Files:**
- Modify: `web/src/js/api/campaigns.ts` (declaration-merging block at `:22-30`, prototype block at `:39-56`)
- Modify: `web/src/react/queries/useCampaignQueries.ts` (after `useCreateCampaign` at `:127-135`)

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces:
  - `api.getCampaign(id: string): Promise<Campaign>` — used by Task 5's staleness guard.
  - `useUpdateCampaign(): UseMutationResult<Campaign, unknown, { id: string; data: Partial<Campaign> }>` — used by Task 5. Call as `mutateAsync({ id, data })`.

- [ ] **Step 1: Add `getCampaign` to the API client**

In `web/src/js/api/campaigns.ts`, the declaration-merging block currently reads:

```ts
declare module './client' {
  interface APIClient {
    // Campaign CRUD
    listCampaigns(activeOnly?: boolean): Promise<Campaign[]>;
    deleteCampaign(id: string): Promise<void>;
    createCampaign(input: CreateCampaignInput): Promise<Campaign>;
    updateCampaign(id: string, data: Partial<Campaign>): Promise<Campaign>;
  }
}
```

Add one line:

```ts
declare module './client' {
  interface APIClient {
    // Campaign CRUD
    listCampaigns(activeOnly?: boolean): Promise<Campaign[]>;
    getCampaign(id: string): Promise<Campaign>;
    deleteCampaign(id: string): Promise<void>;
    createCampaign(input: CreateCampaignInput): Promise<Campaign>;
    updateCampaign(id: string, data: Partial<Campaign>): Promise<Campaign>;
  }
}
```

Then, in the prototype block, after `proto.listCampaigns`:

```ts
// Single-campaign read, deliberately uncached: the edit form uses it to check
// whether the row changed underneath an open form, and a cache read cannot
// observe a write made by the psa-harvest process.
proto.getCampaign = async function (this: APIClient, id: string): Promise<Campaign> {
  return this.get<Campaign>(`/campaigns/${id}`);
};
```

The route already exists: `internal/adapters/httpserver/routes.go:112` → `HandleGetCampaign`, which returns the full campaign JSON.

- [ ] **Step 2: Add `useUpdateCampaign`**

In `web/src/react/queries/useCampaignQueries.ts`, immediately after `useCreateCampaign` (`:127-135`), add:

```tsx
export function useUpdateCampaign() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<Campaign> }) =>
      api.updateCampaign(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.all });
    },
  });
}
```

If `Campaign` is not already imported as a type in this file, add it to the existing `import type { ... } from '../../types/campaigns';` line.

- [ ] **Step 3: Type-check**

Run: `cd web && npm run typecheck`
Expected: exit 0. (`npm run build` does not type-check — do not substitute it.)

- [ ] **Step 4: Run tests to confirm no regression**

Run: `cd web && npm test`
Expected: 450 passed | 2 skipped, 46 files — unchanged from the baseline.

- [ ] **Step 5: Commit**

```bash
git add web/src/js/api/campaigns.ts web/src/react/queries/useCampaignQueries.ts
git commit -m "feat(web): add getCampaign and useUpdateCampaign"
```

---

### Task 3: `SubjectListEditor` — guard the legacy-subject removal path

Removing a `-1` chip is allowed (it drops targeting; it does not manufacture an id) but it is the one path that can turn into a silent hand-repair: remove `-1 Machamp`, retype "Machamp", and `SubjectID` (`resolver.go:143-151`) resolves it case-insensitively to the *different* portal subject `22210 Machamp` and pushes wrong targeting. Confirmation makes the removal deliberate; the session-scoped block closes the retype path.

**Files:**
- Modify: `web/src/react/ui/SubjectListEditor.tsx`
- Test: `web/src/react/ui/SubjectListEditor.test.tsx`

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces: no prop changes. `SubjectListEditorProps` stays `{ label, value, onChange, inputSize? }`. Behavior change only.

- [ ] **Step 1: Write the failing tests**

Append these three tests inside the existing `describe('SubjectListEditor', ...)` block in `web/src/react/ui/SubjectListEditor.test.tsx`, before its closing `});`:

```tsx
  it('asks for confirmation before removing a legacy (-1) chip, and keeps it when declined', async () => {
    vi.mocked(api.listPSASubjects).mockResolvedValue({
      subjects: [{ id: 22210, name: 'Machamp' }],
      fetchedAt: '2026-08-01T00:00:00Z',
    });
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);
    const onChange = renderEditor([{ id: -1, name: 'Machamp' }]);

    fireEvent.click(screen.getByRole('button', { name: /remove machamp/i }));

    expect(confirmSpy).toHaveBeenCalled();
    expect(onChange).not.toHaveBeenCalled();
    confirmSpy.mockRestore();
  });

  it('blocks re-adding a confirmed-removed legacy name, in the dropdown and on Enter', async () => {
    // Remove-then-retype is the hand repair the design forbids: SubjectID
    // matches names with strings.EqualFold (resolver.go:146), so retyping
    // "Machamp" would resolve to the unrelated portal subject 22210 and push
    // wrong targeting — the one failure mode that is silent rather than a 400.
    vi.mocked(api.listPSASubjects).mockResolvedValue({
      subjects: [{ id: 22210, name: 'Machamp' }, { id: 4807, name: 'Charizard' }],
      fetchedAt: '2026-08-01T00:00:00Z',
    });
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
    const onChange = renderEditor([{ id: -1, name: 'Machamp' }]);

    fireEvent.click(screen.getByRole('button', { name: /remove machamp/i }));
    expect(onChange).toHaveBeenCalledWith([]);
    onChange.mockClear();

    const input = await screen.findByPlaceholderText(/add a subject/i);
    fireEvent.change(input, { target: { value: 'cha' } });
    // Charizard proves the catalog loaded and the dropdown opened, so the
    // Machamp assertion below cannot pass vacuously.
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Charizard' })).toBeInTheDocument();
    });

    fireEvent.change(input, { target: { value: 'Machamp' } });
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: 'Machamp' })).not.toBeInTheDocument();
    });
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(onChange).not.toHaveBeenCalled();
    confirmSpy.mockRestore();
  });

  it('does not confirm when removing a normal chip', () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
    const onChange = renderEditor([{ id: 4807, name: 'Charizard' }]);

    fireEvent.click(screen.getByRole('button', { name: /remove charizard/i }));

    expect(confirmSpy).not.toHaveBeenCalled();
    expect(onChange).toHaveBeenCalledWith([]);
    confirmSpy.mockRestore();
  });
```

**Note:** the editor's `value` prop is controlled by the parent, and `renderEditor` does not re-render on `onChange`. That is fine — the block lives in the editor's own state, and the second test asserts the block after the removal call, not a re-rendered chip list.

- [ ] **Step 2: Update the one existing test that removes a `-1` chip**

The existing test `'flags legacy unreconciled subjects (id -1) without disturbing operator-typed ones (id 0)'` ends with an unconfirmed removal (`SubjectListEditor.test.tsx:192-194`):

```tsx
    // Legacy chips stay removable — removing one is a deliberate operator edit.
    fireEvent.click(screen.getByRole('button', { name: /remove blastoise/i }));
    expect(onChange).toHaveBeenCalledWith([{ id: 0, name: 'Mewtwo' }, { id: 4807, name: 'Charizard' }]);
```

Replace those three lines with:

```tsx
    // Legacy chips stay removable — removing one is a deliberate operator edit,
    // now behind a confirmation (see the dedicated tests below).
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
    fireEvent.click(screen.getByRole('button', { name: /remove blastoise/i }));
    expect(onChange).toHaveBeenCalledWith([{ id: 0, name: 'Mewtwo' }, { id: 4807, name: 'Charizard' }]);
    confirmSpy.mockRestore();
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `cd web && npm test -- SubjectListEditor`
Expected: the three new tests FAIL (`window.confirm` never called; "Machamp" still offered after removal). The updated existing test passes either way — it is a compatibility update, not a new assertion.

- [ ] **Step 4: Implement the confirmation and the session block**

In `web/src/react/ui/SubjectListEditor.tsx`:

**4a.** Add state next to the existing `const [query, setQuery] = useState('');`:

```tsx
  // Names of legacy (-1) subjects the operator confirmed removing, lower-cased.
  // Blocking re-entry is what keeps "remove then retype" from becoming a hand
  // repair: a retyped name resolves case-insensitively at push time
  // (resolver.go:146) and can land on a *different* portal subject that happens
  // to share the name. Session-scoped by design — see the design doc's
  // "Known limit".
  const [removedLegacyNames, setRemovedLegacyNames] = useState<Set<string>>(new Set());
```

**4b.** In the `matches` useMemo, the filter currently reads:

```tsx
    return catalog
      .filter(s => s.name.toLowerCase().includes(q)
        && !selectedIds.has(s.id)
        && !selectedNames.has(s.name.toLowerCase()))
      .slice(0, 20);
  }, [query, catalog, value]);
```

Change it to:

```tsx
    return catalog
      .filter(s => s.name.toLowerCase().includes(q)
        && !selectedIds.has(s.id)
        && !selectedNames.has(s.name.toLowerCase())
        && !removedLegacyNames.has(s.name.toLowerCase()))
      .slice(0, 20);
  }, [query, catalog, value, removedLegacyNames]);
```

**4c.** In `addSubject`, add the block check at the top:

```tsx
  function addSubject(subject: SubjectRef) {
    // Enter-to-add bypasses the dropdown, so the removed-legacy block needs
    // enforcing here as well as in `matches`.
    if (removedLegacyNames.has(subject.name.toLowerCase())) {
      setQuery('');
      setOpen(false);
      return;
    }
    // Same two-sided check as `matches`: Enter-to-add can reach this with a
    // typed name that duplicates an already-selected catalog subject, which
    // an id-only comparison would let through.
    const alreadyPresent = value.some(s =>
      (subject.id !== 0 && s.id === subject.id) ||
      s.name.toLowerCase() === subject.name.toLowerCase(),
    );
    if (!alreadyPresent) onChange([...value, subject]);
    setQuery('');
    setOpen(false);
  }
```

**4d.** Replace `removeSubject`:

```tsx
  function removeSubject(index: number) {
    const subject = value[index];
    if (subject?.id === LEGACY_UNRECONCILED_SUBJECT_ID) {
      const ok = window.confirm(
        `Remove "${subject.name}"?\n\n` +
        'This subject has no portal id yet, so removing it drops that targeting — ' +
        'it does not repair it. A real portal id can only come from the harvester ' +
        'baseline pull, and this name cannot be added back on this form.',
      );
      if (!ok) return;
      setRemovedLegacyNames(prev => new Set(prev).add(subject.name.toLowerCase()));
    }
    onChange(value.filter((_, i) => i !== index));
  }
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd web && npm test -- SubjectListEditor`
Expected: PASS, all tests in the file.

- [ ] **Step 6: Type-check and run the full suite**

Run: `cd web && npm run typecheck && npm test`
Expected: exit 0; test count is baseline + 3.

- [ ] **Step 7: Commit**

```bash
git add web/src/react/ui/SubjectListEditor.tsx web/src/react/ui/SubjectListEditor.test.tsx
git commit -m "feat(web): confirm and block re-entry when removing a legacy subject"
```

---

### Task 4: `toFormValues` — the seed mapping

A dedicated module rather than a helper inside `CampaignsPage`: it is the single place the `Campaign` → form shape conversion happens, it is where the null-slice and `phase` hazards live, and it is far cheaper to test directly than through a rendered page.

**Files:**
- Create: `web/src/react/utils/campaignFormValues.ts`
- Test: `web/src/react/utils/campaignFormValues.test.ts`

**Interfaces:**
- Consumes: `Campaign`, `CreateCampaignInput` from `web/src/types/campaigns`.
- Produces: `toFormValues(c: Campaign): CreateCampaignInput` — used by Task 5.

- [ ] **Step 1: Write the failing test**

Create `web/src/react/utils/campaignFormValues.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { toFormValues } from './campaignFormValues';
import type { Campaign } from '../../types/campaigns';

function makeCampaign(overrides: Partial<Campaign> = {}): Campaign {
  return {
    id: 'c1',
    name: 'Vintage Core',
    sport: 'Pokemon',
    yearRange: '1999-2003',
    gradeRange: '8-10',
    priceRange: '50-500',
    clConfidence: 'high',
    buyTermsCLPct: 0.78,
    dailySpendCapCents: 50000,
    targetLanguages: ['english', 'japanese'],
    subjectFilterMode: 'Exclude',
    subjects: [{ id: 22210, name: 'Machamp' }, { id: 0, name: 'Mewtwo' }],
    deniedSpecs: [{ id: 4807, name: 'Charizard' }],
    phase: 'closed',
    psaSourcingFeeCents: 300,
    ebayFeePct: 0.1235,
    expectedFillRate: 0.42,
    psaCampaignRequestId: 'req-1',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-02-02T00:00:00Z',
    ...overrides,
  } as Campaign;
}

describe('toFormValues', () => {
  it('seeds all three targeting axes from the campaign', () => {
    const v = toFormValues(makeCampaign());
    expect(v.targetLanguages).toEqual(['english', 'japanese']);
    expect(v.subjectFilterMode).toBe('Exclude');
    expect(v.subjects).toEqual([{ id: 22210, name: 'Machamp' }, { id: 0, name: 'Mewtwo' }]);
    expect(v.deniedSpecs).toEqual([{ id: 4807, name: 'Charizard' }]);
  });

  it('preserves portal-issued subject ids exactly', () => {
    // Ids span 4xxx/8xxx/22xxx portal generations and are never re-derived from
    // a name. A seed that dropped or rewrote one would push wrong targeting.
    const v = toFormValues(makeCampaign({
      subjects: [{ id: 4807, name: 'Charizard' }, { id: 8123, name: 'Blastoise' }, { id: -1, name: 'Venusaur' }],
    }));
    expect(v.subjects).toEqual([
      { id: 4807, name: 'Charizard' },
      { id: 8123, name: 'Blastoise' },
      { id: -1, name: 'Venusaur' },
    ]);
  });

  it('always sets phase, so a later spread cannot blank it', () => {
    // CampaignsPage saves `{ ...freshCampaign, ...formValues }`. `phase` is
    // optional on CreateCampaignInput, and an explicit `phase: undefined` in
    // the spread would override the campaign's real phase with undefined.
    const v = toFormValues(makeCampaign({ phase: 'pending' }));
    expect(v.phase).toBe('pending');
  });

  it('copies arrays rather than aliasing the campaign', () => {
    const c = makeCampaign();
    const v = toFormValues(c);
    (v.subjects as { id: number; name: string }[]).push({ id: 1, name: 'X' });
    v.targetLanguages.push('klingon');
    expect(c.subjects).toHaveLength(2);
    expect(c.targetLanguages).toEqual(['english', 'japanese']);
  });

  it('tolerates null slices from the server', () => {
    // Go marshals a nil slice as JSON null, so a campaign that has never had
    // targeting set arrives with targetLanguages: null, not [].
    const c = makeCampaign({
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- simulating the real null the server sends, which the non-nullable TS type disallows
      targetLanguages: null as any,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- ditto
      subjects: null as any,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- ditto
      deniedSpecs: null as any,
    });
    const v = toFormValues(c);
    expect(v.targetLanguages).toEqual([]);
    expect(v.subjects).toEqual([]);
    expect(v.deniedSpecs).toEqual([]);
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd web && npm test -- campaignFormValues`
Expected: FAIL — cannot resolve `./campaignFormValues`.

- [ ] **Step 3: Write the implementation**

Create `web/src/react/utils/campaignFormValues.ts`:

```ts
import type { Campaign, CreateCampaignInput } from '../../types/campaigns';

/**
 * Seeds the campaign edit form from an existing campaign.
 *
 * Every key of CreateCampaignInput is assigned explicitly, `phase` included.
 * The save path sends `{ ...freshCampaign, ...formValues }` to a full-row PUT,
 * so a key that is present-but-undefined here would override the campaign's
 * real value with undefined — which is why this does not spread the campaign.
 *
 * Subject ids ride through verbatim: portal ids span 4xxx/8xxx/22xxx
 * generations and are never re-derived from a name. -1
 * (LegacyUnreconciledSubjectID) and 0 (operator-typed, resolved at push time)
 * are preserved as-is too; SubjectListEditor is what treats them differently.
 *
 * Arrays are copied, not aliased, so editing the form cannot mutate the cached
 * campaign object React Query is holding.
 *
 * The `?? []` fallbacks are load-bearing: Go marshals a nil slice as JSON null,
 * so a campaign that never had targeting set arrives with null, not [].
 */
export function toFormValues(c: Campaign): CreateCampaignInput {
  return {
    name: c.name,
    sport: c.sport,
    yearRange: c.yearRange,
    gradeRange: c.gradeRange,
    priceRange: c.priceRange,
    clConfidence: c.clConfidence,
    buyTermsCLPct: c.buyTermsCLPct,
    dailySpendCapCents: c.dailySpendCapCents,
    targetLanguages: [...(c.targetLanguages ?? [])],
    subjectFilterMode: c.subjectFilterMode,
    subjects: (c.subjects ?? []).map(s => ({ ...s })),
    deniedSpecs: (c.deniedSpecs ?? []).map(s => ({ ...s })),
    psaSourcingFeeCents: c.psaSourcingFeeCents,
    ebayFeePct: c.ebayFeePct,
    phase: c.phase,
  };
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd web && npm test -- campaignFormValues`
Expected: PASS, 5 tests.

- [ ] **Step 5: Type-check**

Run: `cd web && npm run typecheck`
Expected: exit 0.

- [ ] **Step 6: Commit**

```bash
git add web/src/react/utils/campaignFormValues.ts web/src/react/utils/campaignFormValues.test.ts
git commit -m "feat(web): add toFormValues seed mapping for the campaign edit form"
```

---

### Task 5: Wire the edit form (`CampaignsPage` + `CampaignsTab`)

These land together. `CampaignsTab`'s new props are required, so adding them without updating the only caller would break `npm run typecheck` mid-tree.

**Files:**
- Modify: `web/src/react/pages/campaigns/CampaignsTab.tsx`
- Modify: `web/src/react/pages/campaigns/CampaignsTab.test.tsx` (harness needs the new props)
- Modify: `web/src/react/pages/CampaignsPage.tsx`
- Create: `web/src/react/pages/CampaignsPage.edit.test.tsx`

**Interfaces:**
- Consumes: `toFormValues` (Task 4), `useUpdateCampaign` (Task 2), `api.getCampaign` (Task 2).
- Produces: `CampaignsTab` props gain `editingCampaign: Campaign | null`, `editForm: UseFormReturn<CreateCampaignInput>`, `updateMutation: { isPending: boolean }`, `onEdit: (c: Campaign) => void`, `onCancelEdit: () => void`.

- [ ] **Step 1: Add the new props to `CampaignsTab`**

In `web/src/react/pages/campaigns/CampaignsTab.tsx`, the component signature at `:47-65` currently reads:

```tsx
export default function CampaignsTab({
  campaigns, pnlMap, healthMap, psaPushMap, showCreate, form, createMutation, onToggleCreate,
}: {
  campaigns: Campaign[];
  pnlMap: Record<string, CampaignPNL>;
  healthMap: Record<string, string>;
  psaPushMap: Record<string, PSAPushRow>;
  showCreate: boolean;
  form: UseFormReturn<CreateCampaignInput>;
  createMutation: { isPending: boolean };
  onToggleCreate: () => void;
}) {
```

Change it to:

```tsx
export default function CampaignsTab({
  campaigns, pnlMap, healthMap, psaPushMap, showCreate, form, createMutation, onToggleCreate,
  editingCampaign, editForm, updateMutation, onEdit, onCancelEdit,
}: {
  campaigns: Campaign[];
  pnlMap: Record<string, CampaignPNL>;
  healthMap: Record<string, string>;
  psaPushMap: Record<string, PSAPushRow>;
  showCreate: boolean;
  form: UseFormReturn<CreateCampaignInput>;
  createMutation: { isPending: boolean };
  onToggleCreate: () => void;
  /** The campaign currently being edited, or null. The parent owns this state
      and the seeded form; the tab only renders and reports intent. */
  editingCampaign: Campaign | null;
  editForm: UseFormReturn<CreateCampaignInput>;
  updateMutation: { isPending: boolean };
  onEdit: (c: Campaign) => void;
  onCancelEdit: () => void;
}) {
```

- [ ] **Step 2: Render the edit card above the list**

Immediately after the closing `)}` of the `{showCreate && ( ... )}` block (which ends just before `{campaigns.length === 0 ? (`), insert:

```tsx
      {editingCampaign && (
        <div className="mb-6">
          <CardShell variant="elevated" padding="lg">
            <form onSubmit={editForm.handleSubmit}>
              <div className="mb-5">
                <h2 className="text-lg font-semibold text-[var(--text)]">
                  Edit {editingCampaign.name}
                </h2>
                <p className="text-sm text-[var(--text-muted)] mt-1">
                  Saves to SlabLedger only. Publish the change to PSA from the campaign&apos;s PSA button.
                </p>
              </div>
              <CampaignFormFields
                values={editForm.values}
                onChange={(field, value) => editForm.handleChange(field as keyof CreateCampaignInput, value)}
                nameError={editForm.touched.name ? editForm.errors.name : undefined}
                onNameBlur={() => editForm.handleBlur('name')}
                showPhase
                showFees
              />
              <div className="mt-5 flex justify-end gap-2">
                <Button type="button" variant="ghost" onClick={onCancelEdit}>Cancel</Button>
                <Button type="submit" loading={editForm.isSubmitting || updateMutation.isPending}>
                  Save Changes
                </Button>
              </div>
            </form>
          </CardShell>
        </div>
      )}
```

- [ ] **Step 3: Add the per-row Edit button**

In the row's right-hand action cluster, the PSA block currently reads:

```tsx
                      <div className="flex items-center gap-2 flex-shrink-0">
                        <StatusPill tone={SYNC_TONES[sync]} size="xs" title={`PSA sync: ${SYNC_LABELS[sync]}`}>
                          {SYNC_LABELS[sync]}
                        </StatusPill>
                        <Button
                          size="sm"
                          variant="ghost"
                          aria-label={`Publish to PSA for ${c.name} — currently ${SYNC_LABELS[sync]}`}
                          onClick={() => setPsaModalCampaignId(c.id)}
                        >
                          PSA
                        </Button>
                      </div>
```

Add an Edit button before the PSA one. It appears on every phase — a closed campaign's targeting is still worth correcting before it is reopened:

```tsx
                      <div className="flex items-center gap-2 flex-shrink-0">
                        <StatusPill tone={SYNC_TONES[sync]} size="xs" title={`PSA sync: ${SYNC_LABELS[sync]}`}>
                          {SYNC_LABELS[sync]}
                        </StatusPill>
                        <Button
                          size="sm"
                          variant="ghost"
                          aria-label={`Edit ${c.name}`}
                          onClick={() => onEdit(c)}
                        >
                          Edit
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          aria-label={`Publish to PSA for ${c.name} — currently ${SYNC_LABELS[sync]}`}
                          onClick={() => setPsaModalCampaignId(c.id)}
                        >
                          PSA
                        </Button>
                      </div>
```

- [ ] **Step 4: Update the `CampaignsTab` test harness and add tab tests**

In `web/src/react/pages/campaigns/CampaignsTab.test.tsx`, the `renderTab` helper must pass the new props. Add these to the `<CampaignsTab ... />` element, after `onToggleCreate={vi.fn()}`:

```tsx
            editingCampaign={editingCampaign}
            editForm={editFormArg ?? fakeForm}
            updateMutation={{ isPending: false }}
            onEdit={onEdit}
            onCancelEdit={vi.fn()}
```

and widen the helper's signature so the existing call sites keep working:

```tsx
function renderTab(
  campaigns: Campaign[],
  psaPushMap: Record<string, PSAPushRow>,
  opts: {
    editingCampaign?: Campaign | null;
    editForm?: UseFormReturn<CreateCampaignInput>;
    onEdit?: (c: Campaign) => void;
  } = {},
) {
  const { editingCampaign = null, editForm: editFormArg, onEdit = vi.fn() } = opts;
```

Then append these tests to the file:

```tsx
it('reports the campaign to edit when Edit is clicked', () => {
  const onEdit = vi.fn();
  const campaign = makeCampaign();
  renderTab([campaign], {}, { onEdit });

  fireEvent.click(screen.getByRole('button', { name: /edit test campaign/i }));

  expect(onEdit).toHaveBeenCalledWith(campaign);
});

it('offers Edit on a closed campaign', () => {
  // A closed campaign's targeting is still worth correcting before it is
  // reopened, so the button is not phase-gated.
  renderTab([makeCampaign({ phase: 'closed' })], {});
  expect(screen.getByRole('button', { name: /edit test campaign/i })).toBeInTheDocument();
});

it('renders the edit card with the seeded form values when a campaign is being edited', () => {
  const campaign = makeCampaign({ name: 'Vintage Core' });
  const editForm = {
    ...fakeForm,
    values: toFormValues(campaign),
  } as unknown as UseFormReturn<CreateCampaignInput>;

  renderTab([campaign], {}, { editingCampaign: campaign, editForm });

  expect(screen.getByText('Edit Vintage Core')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /save changes/i })).toBeInTheDocument();
});
```

Add the imports these need at the top of the file: `fireEvent` to the existing `@testing-library/react` import, and `import { toFormValues } from '../../utils/campaignFormValues';`.

Note the existing `fakeForm` has `values: {}` — that is why the third test supplies a real seeded `values`; `CampaignFormFields` reads them.

- [ ] **Step 5: Run the tab tests**

Run: `cd web && npm test -- CampaignsTab`
Expected: PASS, including the pre-existing tests.

- [ ] **Step 6: Add the edit state and save handler to `CampaignsPage`**

In `web/src/react/pages/CampaignsPage.tsx`, the component body at `:167-188` currently starts:

```tsx
export default function CampaignsPage() {
  const [showCreate, setShowCreate] = useState(false);
  const toast = useToast();
  const queryClient = useQueryClient();
  const { data: allCampaigns = [], isLoading } = useCampaigns(false);
  const createMutation = useCreateCampaign();
```

Add the edit state and mutation:

```tsx
export default function CampaignsPage() {
  const [showCreate, setShowCreate] = useState(false);
  // The campaign under edit, plus the updatedAt captured when the form opened.
  // That timestamp is the optimistic-concurrency token: the psa-harvest
  // baseline pull writes the same targeting fields from a separate process,
  // and a stale form would put -1 placeholders back over reconciled portal ids.
  const [editing, setEditing] = useState<{ id: string; updatedAt: string } | null>(null);
  const toast = useToast();
  const queryClient = useQueryClient();
  const { data: allCampaigns = [], isLoading } = useCampaigns(false);
  const createMutation = useCreateCampaign();
  const updateMutation = useUpdateCampaign();
```

Immediately after the existing create `useForm` block (`:175-188`, ending with its closing `});`), add the edit form. It reuses the module-level `validateCampaignForm` at `:34`:

```tsx
  const editForm = useForm<CreateCampaignInput>({
    initialValues: { ...defaultCampaignInput },
    validate: validateCampaignForm,
    onSubmit: async (values) => {
      if (!editing) return;

      // Re-read over the network, not from the React Query cache: useCampaigns
      // holds data fresh for CAMPAIGN_STALE_TIME (30s), and the racing writer
      // is the psa-harvest process, whose write the cache cannot observe.
      let fresh: Campaign;
      try {
        fresh = await api.getCampaign(editing.id);
      } catch (err) {
        // Fail closed. Saving anyway would reintroduce exactly the race this
        // check exists to close.
        toast.error(getErrorMessage(err, 'Could not confirm the campaign is unchanged — nothing was saved'));
        return;
      }

      if (fresh.updatedAt !== editing.updatedAt) {
        toast.error(
          'This campaign changed since you opened the form — most likely the harvester baseline pull. ' +
          'Nothing was saved. Close and re-open Edit to start from current data.',
        );
        return;
      }

      try {
        // Full-row PUT: HandleUpdateCampaign decodes a whole inventory.Campaign
        // and the UPDATE sets every column, so an omitted field is written as
        // its zero value. Spreading `fresh` first is what keeps
        // psaCampaignRequestId and expectedFillRate intact. This is deliberately
        // NOT the pattern used by the bulk-paste path below, which strips
        // expectedFillRate — correct there, wrong here.
        await updateMutation.mutateAsync({ id: editing.id, data: { ...fresh, ...values } });
        setEditing(null);
        toast.success(
          fresh.psaCampaignRequestId
            ? 'Campaign updated — open PSA on the row to publish the change'
            : 'Campaign updated',
        );
      } catch (err) {
        toast.error(getErrorMessage(err, 'Failed to update campaign'));
      }
    },
  });

  function handleEdit(c: Campaign) {
    setShowCreate(false);
    setEditing({ id: c.id, updatedAt: c.updatedAt });
    editForm.reset(toFormValues(c));
  }

  function handleCancelEdit() {
    setEditing(null);
  }

  const editingCampaign = editing ? allCampaigns.find(c => c.id === editing.id) ?? null : null;
```

Imports: `CampaignsPage.tsx` already has everything this needs except two things. Verified present at the top of the file — `api` (`:8`), `getErrorMessage` (`:13`), `useToast` (`:14`), `useForm` (`:15`), `defaultCampaignInput` (`:16`), and the `Campaign` / `CreateCampaignInput` types (`:10`). `validateCampaignForm` is a module-level function in this same file at `:34`, so the edit form reuses it directly.

Add exactly these two:
- `useUpdateCampaign` to the existing `import { useCampaigns, useCreateCampaign, usePortfolioHealth, campaignPNLQueryOptions } from '../queries/useCampaignQueries';` (`:18`).
- A new line: `import { toFormValues } from '../utils/campaignFormValues';`

- [ ] **Step 7: Pass the new props to `CampaignsTab`**

Find the `<CampaignsTab ... />` element in `CampaignsPage.tsx` and add:

```tsx
          editingCampaign={editingCampaign}
          editForm={editForm}
          updateMutation={updateMutation}
          onEdit={handleEdit}
          onCancelEdit={handleCancelEdit}
```

- [ ] **Step 8: Update the bulk-paste comment**

The comment at `CampaignsPage.tsx:43-54` ends with a claim this change makes false. Replace this sentence:

```
// set once at campaign creation (CampaignFormFields' subject editor) or by the
// harvester's baseline pull — there is currently no edit surface for it after
// that; this paste format stays scoped to scalar economics/range fields, which
// round-trip safely.
```

with:

```
// set at campaign creation, edited through the per-row Edit form (which carries
// SubjectRef ids through untouched), or replaced by the harvester's baseline
// pull. This paste format stays scoped to scalar economics/range fields, which
// round-trip safely.
```

Leave the rest of the comment — the reasoning about ids being re-derived from names — exactly as it is.

- [ ] **Step 9: Write the page-level tests**

Create `web/src/react/pages/CampaignsPage.edit.test.tsx`:

```tsx
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import CampaignsPage from './CampaignsPage';
import { ToastProvider } from '../contexts/ToastContext';
import type { Campaign } from '../../types/campaigns';

const campaign: Campaign = {
  id: 'c1',
  name: 'Vintage Core',
  sport: 'Pokemon',
  yearRange: '1999-2003',
  gradeRange: '8-10',
  priceRange: '50-500',
  clConfidence: 'high',
  buyTermsCLPct: 0.78,
  dailySpendCapCents: 50000,
  targetLanguages: ['english'],
  subjectFilterMode: 'Target',
  subjects: [{ id: 22210, name: 'Machamp' }, { id: 4807, name: 'Charizard' }],
  deniedSpecs: [],
  phase: 'active',
  psaSourcingFeeCents: 300,
  ebayFeePct: 0.1235,
  expectedFillRate: 0.42,
  psaCampaignRequestId: 'req-1',
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-02-02T00:00:00Z',
} as Campaign;

const updateMutateAsync = vi.fn().mockResolvedValue(campaign);

vi.mock('../queries/useCampaignQueries', async (orig) => {
  const mod = await orig<typeof import('../queries/useCampaignQueries')>();
  return {
    ...mod,
    useCampaigns: () => ({ data: [campaign], isLoading: false }),
    usePortfolioHealth: () => ({ data: undefined }),
    useCreateCampaign: () => ({ mutateAsync: vi.fn(), isPending: false }),
    useUpdateCampaign: () => ({ mutateAsync: updateMutateAsync, isPending: false }),
  };
});

const getCampaign = vi.fn();

vi.mock('../../js/api', async (orig) => {
  const mod = await orig<typeof import('../../js/api')>();
  return {
    ...mod,
    api: {
      ...mod.api,
      listPSAPushes: vi.fn().mockResolvedValue({ pushes: [] }),
      listPSASubjects: vi.fn().mockResolvedValue({ subjects: [], fetchedAt: '2026-08-01T00:00:00Z' }),
      getCampaign: (...args: unknown[]) => getCampaign(...args),
    },
  };
});

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <MemoryRouter>
      <QueryClientProvider client={qc}>
        <ToastProvider>
          <CampaignsPage />
        </ToastProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  updateMutateAsync.mockClear();
  getCampaign.mockReset();
});

async function openEditAndSave() {
  const user = userEvent.setup();
  renderPage();
  await user.click(screen.getByRole('button', { name: /edit vintage core/i }));
  await user.click(await screen.findByRole('button', { name: /save changes/i }));
  return user;
}

it('sends the full campaign so a full-row PUT cannot blank server-owned fields', async () => {
  getCampaign.mockResolvedValue(campaign);
  await openEditAndSave();

  await waitFor(() => expect(updateMutateAsync).toHaveBeenCalled());
  const { id, data } = updateMutateAsync.mock.calls[0][0];
  expect(id).toBe('c1');
  // Omitting either of these from a full-row UPDATE writes its zero value:
  // psaCampaignRequestId silently unlinks the campaign from the portal, and
  // expectedFillRate zeroes an analytics input.
  expect(data.psaCampaignRequestId).toBe('req-1');
  expect(data.expectedFillRate).toBe(0.42);
});

it('round-trips existing portal subject ids byte-for-byte', async () => {
  getCampaign.mockResolvedValue(campaign);
  await openEditAndSave();

  await waitFor(() => expect(updateMutateAsync).toHaveBeenCalled());
  const { data } = updateMutateAsync.mock.calls[0][0];
  expect(data.subjects).toEqual([{ id: 22210, name: 'Machamp' }, { id: 4807, name: 'Charizard' }]);
  expect(data.targetLanguages).toEqual(['english']);
  expect(data.subjectFilterMode).toBe('Target');
});

it('checks staleness over the network rather than from cached campaign data', async () => {
  // useCampaigns holds data fresh for 30s and cannot observe a write made by
  // the psa-harvest process, so the guard must actually hit the server.
  getCampaign.mockResolvedValue(campaign);
  await openEditAndSave();

  await waitFor(() => expect(getCampaign).toHaveBeenCalledWith('c1'));
});

it('aborts the save when the campaign changed since the form opened', async () => {
  getCampaign.mockResolvedValue({ ...campaign, updatedAt: '2026-03-03T00:00:00Z' });
  await openEditAndSave();

  await waitFor(() => expect(getCampaign).toHaveBeenCalled());
  expect(updateMutateAsync).not.toHaveBeenCalled();
  expect(await screen.findByText(/harvester baseline pull/i)).toBeInTheDocument();
  // The form stays open so the operator does not lose their edits.
  expect(screen.getByRole('button', { name: /save changes/i })).toBeInTheDocument();
});

it('aborts the save when the staleness check itself fails', async () => {
  // Fail closed: saving anyway would reintroduce the race the check closes.
  getCampaign.mockRejectedValue(new Error('network down'));
  await openEditAndSave();

  await waitFor(() => expect(getCampaign).toHaveBeenCalled());
  expect(updateMutateAsync).not.toHaveBeenCalled();
  expect(await screen.findByText(/could not confirm/i)).toBeInTheDocument();
});
```

- [ ] **Step 10: Run the page tests**

Run: `cd web && npm test -- CampaignsPage`
Expected: PASS — the new edit tests plus the existing `CampaignsPage.psaLink.test.tsx` and `web/tests/pages/CampaignsPage.test.tsx`.

If the existing `web/tests/pages/CampaignsPage.test.tsx` fails because the new Edit buttons changed a query that expected a single match, narrow that test's query rather than removing the button — report the change in the commit message.

- [ ] **Step 11: Full gates**

Run: `cd web && npm run typecheck && npm test && npm run lint`
Expected: exit 0 on each. (`npm run build` does not type-check; `npm run typecheck` is the gate.)

- [ ] **Step 12: Commit**

```bash
git add web/src/react/pages/CampaignsPage.tsx web/src/react/pages/CampaignsPage.edit.test.tsx \
        web/src/react/pages/campaigns/CampaignsTab.tsx web/src/react/pages/campaigns/CampaignsTab.test.tsx
git commit -m "feat(web): add a campaign edit form with targeting axes"
```

---

### Task 6: Final verification

**Files:** none modified — this is the gate before claiming completion.

- [ ] **Step 1: Inspect the whole diff**

Run: `git diff main...HEAD --stat && git diff main...HEAD`
Check: no debug output, no placeholders, no scope beyond the file list above. In particular confirm the bulk-paste format itself is unchanged — only its comment.

- [ ] **Step 2: Backend gates**

Run: `go build ./... && go test -race ./... && make check`
Expected: exit 0. `make check` covers the hexagonal import rule and the file-size budget.

- [ ] **Step 3: Frontend gates**

Run: `cd web && npm run typecheck && npm test && npm run lint && npm run build`
Expected: exit 0 on each. Test count should be the 452 baseline plus the tests added in Tasks 3, 4, and 5.

- [ ] **Step 4: Confirm the spec's assertions are all covered**

Walk the spec's Testing table and point each row at a test that now exists. Report any row that does not have one.

- [ ] **Step 5: Manual smoke (optional, requires a running stack)**

`go build -o slabledger ./cmd/slabledger && ./slabledger`, then `cd web && npm run dev`. On the Campaigns page: click **Edit** on a campaign with subjects, change a language checkbox, save, and confirm the row's filter summary updates and the PSA sync pill still reads as linked (i.e. `psaCampaignRequestId` survived).

- [ ] **Step 6: Commit anything the gates required**

```bash
git add -A
git commit -m "chore: verification fixes for campaign targeting edit"
```

(Skip if the gates were clean.)
