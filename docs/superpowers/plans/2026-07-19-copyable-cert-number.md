# Click-to-copy Certificate Number Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the PSA certificate number on the inventory list copy to the clipboard in a single click, on both desktop rows and mobile cards.

**Architecture:** A single focused, reusable React component (`CopyableCert`) owns the clipboard call, a ~1s inline "copied" flash, keyboard accessibility, and click-event isolation (`stopPropagation`). It is dropped into `DesktopRow.tsx` and `MobileCard.tsx` in place of the plain cert text.

**Tech Stack:** React + TypeScript, Vitest + @testing-library/react + @testing-library/user-event, Tailwind CSS variables.

## Global Constraints

- Frontend commands run from `web/` (e.g. `cd web && npm test`).
- No toast on copy — feedback is an inline ~1s flash only.
- Component writes the **raw** `certNumber` digits to the clipboard, regardless of displayed label.
- Clicking the cert must NOT trigger the row's own click handler (`stopPropagation`).
- Falsy `certNumber` renders nothing.
- Spec: `docs/superpowers/specs/2026-07-19-copyable-cert-number-design.md`.
- Branch: `feat/copyable-cert-number` (already checked out).

---

### Task 1: `CopyableCert` component + tests

**Files:**
- Create: `web/src/react/pages/campaign-detail/inventory/CopyableCert.tsx`
- Test: `web/src/react/pages/campaign-detail/inventory/CopyableCert.test.tsx`

**Interfaces:**
- Consumes: `navigator.clipboard.writeText` (browser API).
- Produces: `export default function CopyableCert(props: { certNumber: string; children?: React.ReactNode }): JSX.Element | null`

- [ ] **Step 1: Write the failing tests**

Create `web/src/react/pages/campaign-detail/inventory/CopyableCert.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import CopyableCert from './CopyableCert';

describe('CopyableCert', () => {
  const writeText = vi.fn().mockResolvedValue(undefined);

  beforeEach(() => {
    writeText.mockClear();
    Object.assign(navigator, { clipboard: { writeText } });
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('writes the raw cert number to the clipboard on click', async () => {
    const user = userEvent.setup();
    render(<CopyableCert certNumber="12345678">Cert #12345678</CopyableCert>);
    await user.click(screen.getByRole('button'));
    expect(writeText).toHaveBeenCalledWith('12345678');
  });

  it('stops propagation so a parent click handler does not fire', async () => {
    const user = userEvent.setup();
    const parentClick = vi.fn();
    render(
      <div onClick={parentClick}>
        <CopyableCert certNumber="12345678" />
      </div>,
    );
    await user.click(screen.getByRole('button'));
    expect(parentClick).not.toHaveBeenCalled();
  });

  it('renders nothing when certNumber is empty', () => {
    const { container } = render(<CopyableCert certNumber="" />);
    expect(container).toBeEmptyDOMElement();
  });

  it('does not throw when the clipboard write rejects', async () => {
    writeText.mockRejectedValueOnce(new Error('denied'));
    const user = userEvent.setup();
    render(<CopyableCert certNumber="12345678" />);
    await user.click(screen.getByRole('button'));
    expect(writeText).toHaveBeenCalled();
  });

  it('shows a copied flash after a successful copy and clears it', async () => {
    vi.useFakeTimers();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<CopyableCert certNumber="12345678">Cert #12345678</CopyableCert>);
    await user.click(screen.getByRole('button'));
    expect(screen.getByText(/copied/i)).toBeInTheDocument();
    act(() => { vi.advanceTimersByTime(1100); });
    expect(screen.queryByText(/copied/i)).not.toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && npx vitest run src/react/pages/campaign-detail/inventory/CopyableCert.test.tsx`
Expected: FAIL — cannot resolve `./CopyableCert`.

- [ ] **Step 3: Write the component**

Create `web/src/react/pages/campaign-detail/inventory/CopyableCert.tsx`:

```tsx
import { useEffect, useRef, useState } from 'react';

interface CopyableCertProps {
  certNumber: string;
  children?: React.ReactNode;
}

export default function CopyableCert({ certNumber, children }: CopyableCertProps) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => () => {
    if (timer.current) clearTimeout(timer.current);
  }, []);

  if (!certNumber) return null;

  const handleClick = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await navigator.clipboard.writeText(certNumber);
      setCopied(true);
      if (timer.current) clearTimeout(timer.current);
      timer.current = setTimeout(() => setCopied(false), 1000);
    } catch {
      // Clipboard rejection is rare and non-destructive; no flash signals failure.
    }
  };

  return (
    <button
      type="button"
      onClick={handleClick}
      title="Copy cert number"
      aria-label={`Copy cert number ${certNumber}`}
      className={`inline cursor-pointer bg-transparent border-0 p-0 font-inherit text-inherit hover:text-[var(--text)] hover:underline ${
        copied ? 'text-[var(--success,#16a34a)]' : ''
      }`}
    >
      {copied ? 'Copied ✓' : (children ?? certNumber)}
    </button>
  );
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd web && npx vitest run src/react/pages/campaign-detail/inventory/CopyableCert.test.tsx`
Expected: PASS (5 tests).

- [ ] **Step 5: Typecheck and lint**

Run: `cd web && npx tsc --noEmit && npx eslint src/react/pages/campaign-detail/inventory/CopyableCert.tsx`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/react/pages/campaign-detail/inventory/CopyableCert.tsx web/src/react/pages/campaign-detail/inventory/CopyableCert.test.tsx
git commit -m "feat: add CopyableCert click-to-copy component"
```

---

### Task 2: Integrate into DesktopRow and MobileCard

**Files:**
- Modify: `web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx:208`
- Modify: `web/src/react/pages/campaign-detail/inventory/MobileCard.tsx:108`

**Interfaces:**
- Consumes: `CopyableCert` from Task 1.

- [ ] **Step 1: Add the import to `DesktopRow.tsx`**

Add near the other local imports at the top of the file:

```tsx
import CopyableCert from './CopyableCert';
```

- [ ] **Step 2: Replace the cert render in `DesktopRow.tsx`**

Line 208, change:

```tsx
{item.purchase.certNumber && <> &middot; {item.purchase.certNumber}</>}
```
to:
```tsx
{item.purchase.certNumber && <> &middot; <CopyableCert certNumber={item.purchase.certNumber} /></>}
```

- [ ] **Step 3: Add the import to `MobileCard.tsx`**

Add near the other local imports at the top of the file:

```tsx
import CopyableCert from './CopyableCert';
```

- [ ] **Step 4: Replace the cert render in `MobileCard.tsx`**

Line 108, change:

```tsx
Cert #{item.purchase.certNumber} &middot; <GradeBadge grader={item.purchase.grader || 'PSA'} grade={item.purchase.gradeValue} size="sm" />
```
to:
```tsx
<CopyableCert certNumber={item.purchase.certNumber}>Cert #{item.purchase.certNumber}</CopyableCert> &middot; <GradeBadge grader={item.purchase.grader || 'PSA'} grade={item.purchase.gradeValue} size="sm" />
```

- [ ] **Step 5: Typecheck, lint, and run the inventory test suite**

Run: `cd web && npx tsc --noEmit && npx eslint src/react/pages/campaign-detail/inventory/DesktopRow.tsx src/react/pages/campaign-detail/inventory/MobileCard.tsx && npx vitest run src/react/pages/campaign-detail/inventory`
Expected: no type/lint errors; all existing inventory tests still PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/react/pages/campaign-detail/inventory/DesktopRow.tsx web/src/react/pages/campaign-detail/inventory/MobileCard.tsx
git commit -m "feat: make cert number click-to-copy on inventory rows and cards"
```

---

## Self-Review

- **Spec coverage:** clickable cert (Task 1 component) ✓; raw-digit copy ✓; desktop + mobile integration (Task 2) ✓; minimal affordance / hover cue (className) ✓; inline ~1s flash, no toast (component + test 5) ✓; stopPropagation (test 2) ✓; falsy guard (test 3) ✓; Fix-DH dialog explicitly out of scope — not touched ✓.
- **Placeholder scan:** none — every step shows full code or an exact command.
- **Type consistency:** `CopyableCert({ certNumber, children })` signature is identical across the component, both call sites, and all tests.
