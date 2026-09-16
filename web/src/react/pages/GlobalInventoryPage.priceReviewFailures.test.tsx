import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import GlobalInventoryPage from './GlobalInventoryPage';
import { ToastProvider } from '../contexts/ToastContext';
import { evaluation, inventoryItem, purchaseId, sales } from './price-review/fixtures.test-support';
import { showPrepKeys } from '../queries/showPrepKeys';
import type { InventoryEvaluations } from '../../js/api/showprep';
import type { ShowEvaluation, ShowEvidence } from '../../types/showprep';
import unknownPurchase from './price-review/unknown-purchase.test-support.json';

// Only layout and HTTP are controlled; page, transport, validation and observer are real.
vi.mock('@tanstack/react-virtual', () => ({ useVirtualizer: ({ count }: { count: number }) => ({
  getTotalSize: () => count * 160,
  getVirtualItems: () => Array.from({ length: count }, (_, index) => ({ index, start: index * 160 })),
  measureElement() {},
}) }));
let qc: QueryClient;
const flush = async () => { await act(async () => { for (let i = 0; i < 100; i++) { await Promise.resolve(); await vi.advanceTimersByTimeAsync(0); } }); };
const advance = async (ms: number) => { await act(async () => { await vi.advanceTimersByTimeAsync(ms); }); await flush(); };
const click = (name: string) => fireEvent.click(screen.getByRole('button', { name }));
const saved = () => within(screen.getByRole('region', { name: 'Saved price assessment' }));
const selected = () => screen.getByRole('checkbox', { name: 'Select Aurora Dragon' });
function mount() {
  qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><MemoryRouter initialEntries={['/inventory?view=pricing']}><ToastProvider><GlobalInventoryPage /></ToastProvider></MemoryRouter></QueryClientProvider>);
}
beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(new Date('2026-09-16T23:59:50Z')); vi.stubGlobal('scrollTo', vi.fn()); });
afterEach(() => { cleanup(); qc?.clear(); vi.useRealTimers(); vi.unstubAllGlobals(); });

it('does not recertify initially unversioned cached detail after failed midnight observation, and explicitly recovers', async () => {
  let current = evaluation({ status: 'supported', localPriceCents: 240000,
    reason: 'Recent matching sales support the asking price',
    readiness: { state: 'current', refreshEligibility: 'not_needed', identityKey: 'a'.repeat(64), expiresAt: '2026-09-17T00:00:00Z', retryAt: '' },
  });
  let fail = false;
  const requests: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    requests.push(url);
    if (url === '/api/inventory') return Response.json({ items: [inventoryItem(current)], warnings: [] });
    if (url.endsWith('/evaluate')) {
      if (fail) return Response.json({ error: 'Controlled evaluation failure' }, { status: 400 });
      await new Promise(resolve => setTimeout(resolve, 50));
      return Response.json({ evaluations: [current] });
    }
    if (url.includes('/evidence/')) return Response.json(fail ? { error: 'Controlled evidence failure' } : { evaluation: current, sales }, { status: fail ? 400 : 200 });
    throw new Error(`Unexpected ${url}`);
  }));
  mount(); await flush(); await advance(100);
  expect(qc.getQueryData([...showPrepKeys.evidence(purchaseId), ''])).toBeDefined();
  expect(saved().getByText('Supported', { exact: true })).toBeVisible();
  fireEvent.click(selected());
  const input = screen.getByRole('textbox', { name: 'Asking price' });
  fireEvent.change(input, { target: { value: 'unfinished draft' } }); input.focus();
  fail = true; await advance(10000);
  const aggregate = qc.getQueryData<InventoryEvaluations>([...showPrepKeys.evaluations, [purchaseId]])!;
  expect(aggregate.evaluations[purchaseId]).toBeUndefined();
  expect(aggregate.errors[purchaseId]).toContain('Controlled evaluation failure');
  const assertUnavailable = () => {
    expect(saved().queryByText('Supported', { exact: true })).toBeNull();
    expect(saved().queryByText(current.reason, { exact: true })).toBeNull();
    expect(screen.getByRole('region', { name: 'Saved price assessment' })).toHaveTextContent(/unavailable.*stale/i);
    expect(saved().queryByText(/\d.*%.*asking|Below asking|At asking/)).toBeNull();
    expect(screen.getByRole('button', { name: 'Save price' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Add to show (1)' })).toBeDisabled();
    expect(selected()).toBeChecked();
    expect(screen.getByRole('textbox', { name: 'Asking price' })).toHaveValue('unfinished draft');
  };
  assertUnavailable(); expect(input).toHaveFocus();
  expect(screen.getAllByRole('link', { name: /Auction house sale/ })[0]).toBeVisible();
  expect(screen.getByLabelText('Saved asking price')).toHaveTextContent('$2,400.00');
  await advance(60000); assertUnavailable();
  click('Inventory');
  expect(screen.getByRole('link', { name: /Review price 00000001/ })).toHaveAttribute('title', 'SlabLedger asking Unavailable');
  click('Price review'); await flush(); assertUnavailable();
  fail = false;
  current = { ...current, version: 'recovered', readiness: { ...current.readiness!, expiresAt: '2026-09-18T00:00:00Z' } };
  const recoveryStart = requests.length;
  click('Retry evaluation'); await advance(100);
  expect(saved().getByText('Supported', { exact: true })).toBeVisible();
  expect(requests.slice(recoveryStart)).toContain('/api/show-prep/evaluate');
  expect(screen.getByRole('textbox', { name: 'Asking price' })).toHaveValue('unfinished draft');
  expect(selected()).toBeChecked();
  expect(screen.getByRole('button', { name: 'Add to show (1)' })).toBeDisabled();
  fireEvent.click(selected()); fireEvent.click(selected());
  expect(screen.getByRole('button', { name: 'Add to show (1)' })).toBeEnabled();
  expect(requests.some(url => /refresh|review-price|override|preview/.test(url))).toBe(false);
});

it('does not let a recovered detail cache override a repeated unknown aggregate with the same deterministic version', async () => {
  const unknown = unknownPurchase as ShowEvaluation;
  let known = evaluation({ status: 'supported', reason: 'Authoritative known asking', version: 'known-before-failure' });
  let failDetail = true;
  let detailReads = 0;
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    if (url === '/api/inventory') return Response.json({ items: [inventoryItem(known)], warnings: [] });
    if (url.endsWith('/evaluate')) return Response.json({ evaluations: [unknown] });
    if (url.includes('/evidence/')) {
      detailReads++;
      return failDetail ? Response.json({ error: 'Controlled purchase read failure' }, { status: 400 })
        : Response.json({ evaluation: known, sales });
    }
    throw new Error(`Unexpected ${url}`);
  }));
  mount(); await flush(); await advance(100);
  expect(screen.getByLabelText('Saved asking price')).toHaveTextContent('Unknown');
  failDetail = false; click('Retry evidence'); await flush(); await advance(100);
  expect(saved().getByText('Supported', { exact: true })).toBeVisible();
  const unknownKey = [...showPrepKeys.evidence(purchaseId), unknown.version];
  expect(qc.getQueryData<ShowEvidence>(unknownKey)?.evaluation.version).toBe('known-before-failure');
  fireEvent.click(selected());
  fireEvent.change(screen.getByRole('textbox', { name: 'Asking price' }), { target: { value: 'unfinished draft' } });
  failDetail = true;
  const readsBeforeFailure = detailReads;
  await act(async () => { await qc.invalidateQueries({ queryKey: showPrepKeys.evaluations }); }); await advance(100);
  expect(qc.getQueryData<InventoryEvaluations>([...showPrepKeys.evaluations, [purchaseId]])?.evaluations[purchaseId]).toEqual(unknown);
  expect(detailReads).toBe(readsBeforeFailure); // Still-query-fresh known detail under the repeated unknown key.
  expect(saved().queryByText('Supported', { exact: true })).toBeNull();
  expect(saved().queryByText(known.reason, { exact: true })).toBeNull();
  expect(screen.getByLabelText('Saved asking price')).toHaveTextContent('Unknown');
  expect(screen.getByRole('button', { name: 'Unavailable 1' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Save price' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Add to show (1)' })).toBeDisabled();
  expect(screen.getByRole('textbox', { name: 'Asking price' })).toHaveValue('unfinished draft');
  expect(selected()).toBeChecked();
  // Time alone cannot recover. Remounting stale detail performs a genuinely new read.
  await advance(31000);
  expect(saved().queryByText('Supported', { exact: true })).toBeNull();
  failDetail = false; known = { ...known, version: 'known-after-recovery' };
  click('Inventory'); click('Price review'); await flush(); await advance(100);
  expect(detailReads).toBeGreaterThan(readsBeforeFailure);
  expect(saved().getByText('Supported', { exact: true })).toBeVisible();
  expect(qc.getQueryData<InventoryEvaluations>([...showPrepKeys.evaluations, [purchaseId]])?.evaluations[purchaseId].version).toBe('known-after-recovery');
  expect(screen.getByRole('textbox', { name: 'Asking price' })).toHaveValue('unfinished draft');
  expect(selected()).toBeChecked();
  expect(screen.getByRole('button', { name: 'Add to show (1)' })).toBeDisabled();
});

it.each(['snapshot failure', 'healthy changed assessment'] as const)('keeps the latest %s aggregate authoritative over query-fresh recovered detail until a new read publishes', async outcome => {
  const snapshotFailure = outcome === 'snapshot failure';
  const fallback = evaluation(snapshotFailure ? {
    status: 'needs_review', reason: 'Evidence storage unavailable', evidenceNeedsReview: true, evidenceReason: 'Evidence storage unavailable',
    version: 'deterministic-snapshot-failure', evidenceVersion: 'unreadable-snapshot', compCount: 0, medianCents: 0, latestSaleDate: '', refreshedAt: '',
    recent: { ...evaluation().recent, saleIds: [], count: 0, medianCents: 0, latestSaleDate: '', latestSaleCount: 0, latestSaleMinCents: 0, latestSaleMaxCents: 0, gapPct: null },
    readiness: { state: 'unavailable', refreshEligibility: 'unavailable', identityKey: 'a'.repeat(64), expiresAt: '', retryAt: '' },
  } : { version: 'deterministic-above-comps' });
  let detail = fallback;
  let detailReads = 0;
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    if (url === '/api/inventory') return Response.json({ items: [inventoryItem(fallback)], warnings: [] });
    if (url.endsWith('/evaluate')) return Response.json({ evaluations: [fallback] });
    if (url.includes('/evidence/')) {
      detailReads++;
      return Response.json({ evaluation: detail, sales: detail.evidenceNeedsReview ? [] : sales });
    }
    throw new Error(`Unexpected ${url}`);
  }));
  mount(); await flush(); await advance(100);
  expect(saved().getByText(snapshotFailure ? 'Evidence unavailable' : 'Asking above comps', { exact: true })).toBeVisible();
  const fallbackKey = [...showPrepKeys.evidence(purchaseId), fallback.version];
  detail = evaluation({ status: 'supported', reason: 'New read supports asking', version: 'recovered-first', recent: { ...evaluation().recent, gapPct: 0 } });
  await act(async () => { await qc.refetchQueries({ queryKey: fallbackKey, exact: true }); }); await advance(100);
  expect(saved().getByText('Supported', { exact: true })).toBeVisible();
  expect(qc.getQueryData<ShowEvidence>(fallbackKey)?.evaluation.version).toBe('recovered-first');
  fireEvent.click(selected());
  const input = screen.getByRole('textbox', { name: 'Asking price' });
  fireEvent.change(input, { target: { value: 'unfinished draft' } }); input.focus();
  const readsBeforeRepeat = detailReads;
  await act(async () => { await qc.invalidateQueries({ queryKey: showPrepKeys.evaluations }); }); await advance(100);
  expect(qc.getQueryData<InventoryEvaluations>([...showPrepKeys.evaluations, [purchaseId]])?.evaluations[purchaseId]).toEqual(fallback);
  expect(detailReads).toBe(readsBeforeRepeat); // Fresh recovered detail under the repeated fallback key, not a new observation.
  expect(screen.getByRole('button', { name: snapshotFailure ? 'Unavailable 1' : 'Above comps 1' })).toBeVisible();
  expect(saved().queryByText('Supported', { exact: true })).toBeNull();
  expect(saved().queryByText('New read supports asking', { exact: true })).toBeNull();
  expect(saved().getByText(snapshotFailure ? 'Evidence unavailable' : 'Asking above comps', { exact: true })).toBeVisible();
  expect(saved().queryByText('At asking', { exact: true })).toBeNull();
  if (snapshotFailure) expect(saved().queryByText(/\d.*%.*asking|Below asking/)).toBeNull();
  expect(screen.getByLabelText('Saved asking price')).toHaveTextContent('$2,800.00');
  expect(input).toHaveValue('unfinished draft'); expect(input).toHaveFocus(); expect(selected()).toBeChecked();
  expect(screen.getByRole('button', { name: 'Add to show (1)' })).toBeDisabled();
  // Elapsed freshness alone is not recovery. Remounting stale detail actually reads again.
  await advance(31000);
  expect(detailReads).toBe(readsBeforeRepeat);
  expect(saved().queryByText('Supported', { exact: true })).toBeNull();
  detail = { ...detail, version: 'recovered-subsequent' };
  click('Inventory'); click('Price review'); await flush(); await advance(100);
  expect(detailReads).toBeGreaterThan(readsBeforeRepeat);
  expect(qc.getQueryData<InventoryEvaluations>([...showPrepKeys.evaluations, [purchaseId]])?.evaluations[purchaseId].version).toBe('recovered-subsequent');
  expect(saved().getByText('Supported', { exact: true })).toBeVisible();
  expect(screen.getByRole('textbox', { name: 'Asking price' })).toHaveValue('unfinished draft');
  expect(selected()).toBeChecked(); expect(screen.getByRole('button', { name: 'Add to show (1)' })).toBeDisabled();
  fireEvent.click(selected()); fireEvent.click(selected());
  expect(screen.getByRole('button', { name: 'Add to show (1)' })).toBeEnabled();
});

it.each(['current', 'legacy'] as const)('presents the %s failed-purchase-read DTO as unavailable, never a known missing asking', async shape => {
  const unknown = { ...unknownPurchase, ...(shape === 'legacy' ? { status: 'no_listed_price', reason: 'No positive SlabLedger asking price', version: '6207014479ee05a1c6edf8f7e97cf87d78352e2b33299c39ca4873dc46235e6d' } : {}) } as ShowEvaluation;
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    if (url === '/api/inventory') return Response.json({ items: [inventoryItem()], warnings: [] });
    if (url.endsWith('/evaluate')) return Response.json({ evaluations: [unknown] });
    // Old cached fallback detail is also valid transport data, not proof of no asking.
    if (url.includes('/evidence/')) return shape === 'legacy' ? Response.json({ evaluation: unknown, sales: [] })
      : Response.json({ error: 'Controlled purchase read failure' }, { status: 400 });
    throw new Error(`Unexpected ${url}`);
  }));
  mount(); await flush();
  expect(screen.getByRole('button', { name: 'Unpriced 0' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Unavailable 1' })).toBeVisible();
  expect(saved().queryByText('No asking price', { exact: true })).toBeNull();
  expect(screen.getByLabelText('Saved asking price')).toHaveTextContent('Unknown');
  expect(screen.getByRole('button', { name: 'Review Aurora Dragon' })).not.toHaveTextContent(/No asking price|Not set/);
  expect(screen.getByRole('button', { name: 'Save price' })).toBeDisabled();
  fireEvent.click(selected()); expect(screen.getByRole('button', { name: 'Add to show (1)' })).toBeDisabled();
  fireEvent.click(screen.getByText('Source diagnostics'));
  expect(screen.getByText('Stored DH listed price').nextElementSibling).toHaveTextContent('Unknown');
  click('Inventory');
  expect(screen.getByRole('link', { name: /Review price 00000001/ })).toHaveAttribute('title', 'SlabLedger asking Unknown');
});

it.each(['healthy', 'failed snapshot', 'failed detail'] as const)('retains genuinely unpriced semantics with %s evidence', async outcome => {
  const unhealthy = outcome === 'failed snapshot';
  const unpriced = evaluation({ localPriceCents: 0, status: 'no_listed_price', reason: 'No positive SlabLedger asking price',
    evidenceNeedsReview: unhealthy, evidenceReason: unhealthy ? 'Stored source failed' : '', recent: { ...evaluation().recent, gapPct: null } });
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    if (url === '/api/inventory') return Response.json({ items: [inventoryItem(unpriced)], warnings: [] });
    if (url.endsWith('/evaluate')) return Response.json({ evaluations: [unpriced] });
    if (url.includes('/evidence/')) return outcome === 'failed detail' ? Response.json({ error: 'Controlled detail read failure' }, { status: 400 })
      : Response.json({ evaluation: unpriced, sales });
    throw new Error(`Unexpected ${url}`);
  }));
  mount(); await flush(); await advance(100);
  if (outcome === 'failed detail') expect(screen.getByRole('button', { name: 'Retry evidence' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Unpriced 1' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Unavailable 0' })).toBeVisible();
  expect(saved().getByText('No asking price', { exact: true })).toBeVisible();
  expect(screen.getByLabelText('Saved asking price')).toHaveTextContent('Not set');
  expect(screen.getByRole('textbox', { name: 'Asking price' })).toBeEnabled();
  expect(saved().queryByText(/Stored sales may be partial or stale/) !== null).toBe(unhealthy);
  click('Inventory');
  expect(screen.getByRole('link', { name: /Review price 00000001/ })).toHaveAttribute('title', 'SlabLedger asking not set');
});
