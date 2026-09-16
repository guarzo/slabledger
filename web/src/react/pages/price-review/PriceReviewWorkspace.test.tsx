import { useState, type ReactNode } from 'react';
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import type { AgingItem } from '../../../types/campaigns';
import type { ShowEvaluation } from '../../../types/showprep';
import { showPrepAPI } from '../../../js/api/showprep';
import { priceReviewAPI } from '../../../js/api/priceReview';
import { PriceReviewWorkspace } from './PriceReviewWorkspace';
import { usePriceReviewState } from './usePriceReviewState';
import { evaluation, inventoryItem, otherId, preview, purchaseId, sales } from './fixtures.test-support';

const a = evaluation();
const supported = { status: 'supported' as const, localPriceCents: 240000, reason: 'Recent sales support asking.', recent: { ...a.recent, gapPct: 0 } };
const b = evaluation({ ...supported, purchaseId: otherId, cardName: 'Orbit Fox', certNumber: '00000002' });
const initialItems = [inventoryItem(a), inventoryItem(b)];
const initialEvaluations = { [purchaseId]: a, [otherId]: b };
let liveEvaluations: Record<string, ShowEvaluation>;
let qc: QueryClient;
beforeEach(() => {
  liveEvaluations = initialEvaluations;
  save.mockReset();
  qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  vi.spyOn(showPrepAPI, 'evidence').mockImplementation(async id => ({ evaluation: liveEvaluations[id], sales }));
  vi.spyOn(priceReviewAPI, 'preview').mockImplementation(async (id, cents) => preview(cents, liveEvaluations[id]));
});
afterEach(() => { cleanup(); qc.clear(); vi.restoreAllMocks(); });
const save = vi.fn(async (_id: string, _cents: number) => {});
function Harness({ items = initialItems, evaluations = initialEvaluations }: { items?: AgingItem[]; evaluations?: Record<string, ShowEvaluation> }) {
  const review = usePriceReviewState(items, evaluations, '');
  const [selected, setSelected] = useState(new Set<string>());
  const [visible, setVisible] = useState(true);
  return <><button onClick={() => setVisible(!visible)}>Switch view</button>{visible && <PriceReviewWorkspace items={items} evaluations={evaluations}
    review={review} selected={selected} onToggleSelected={id => setSelected(old => {
      const next = new Set(old); if (next.has(id)) next.delete(id); else next.add(id); return next;
    })} onSavePrice={save} />}</>;
}
function setup() {
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  return { ...render(<Harness />, { wrapper }), user: userEvent.setup() };
}
it('separates focused card from bulk checkbox intent and retains both through view unmount/remount', async () => {
  const { user } = setup();
  const workspace = screen.getByRole('region', { name: 'Price review' });
  await user.click(within(workspace).getByRole('checkbox', { name: 'Select Orbit Fox' }));
  expect(within(screen.getByRole('region', { name: 'Price details' })).getByRole('heading', { name: 'Aurora Dragon' })).toBeInTheDocument();
  await user.clear(screen.getByLabelText('Asking price')); await user.type(screen.getByLabelText('Asking price'), '2400');
  await user.click(screen.getByRole('button', { name: 'Review Orbit Fox' }));
  await user.clear(screen.getByLabelText('Asking price')); await user.type(screen.getByLabelText('Asking price'), '1900');
  await user.click(screen.getByRole('button', { name: 'Switch view' }));
  expect(screen.queryByRole('region', { name: 'Price review' })).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Switch view' }));
  expect(screen.getByLabelText('Asking price')).toHaveValue('1900');
  expect(screen.getByRole('checkbox', { name: 'Select Orbit Fox' })).toBeChecked();
  await user.click(screen.getByRole('button', { name: 'Review Aurora Dragon' }));
  expect(screen.getByLabelText('Asking price')).toHaveValue('2400');
});
it('keeps the focused panel after live status leaves the filter until explicit navigation', async () => {
  const { user, rerender } = setup();
  await user.click(screen.getByRole('button', { name: 'Above comps 1' }));
  liveEvaluations = { ...initialEvaluations, [purchaseId]: { ...a, ...supported, version: 'new' } };
  rerender(<Harness evaluations={liveEvaluations} />);
  expect(screen.getByText(/Outside the current filter/)).toBeInTheDocument();
  expect(within(screen.getByRole('region', { name: 'Price details' })).getByRole('heading', { name: 'Aurora Dragon' })).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'All 2' }));
  await user.click(screen.getByRole('button', { name: 'Next card' }));
  expect(within(screen.getByRole('region', { name: 'Price details' })).getByRole('heading', { name: 'Orbit Fox' })).toBeInTheDocument();
});
it('keeps removed-card draft visible and disables unsafe saves rather than retaining stale inventory', async () => {
  const { user, rerender } = setup();
  await user.clear(screen.getByLabelText('Asking price')); await user.type(screen.getByLabelText('Asking price'), '2400');
  rerender(<Harness items={[initialItems[1]]} />);
  expect(screen.getByText(/Card unavailable in current inventory/)).toBeInTheDocument();
  expect(screen.getByLabelText('Asking price')).toHaveValue('2400');
  expect(screen.getByRole('button', { name: 'Save price' })).toBeDisabled();
});
it('cannot turn supported but sold/not-packable live inventory into show eligibility', async () => {
  const { user, rerender } = setup();
  await user.clear(screen.getByLabelText('Asking price')); await user.type(screen.getByLabelText('Asking price'), '2300');
  expect(screen.getByRole('button', { name: 'Save price' })).toBeEnabled();
  liveEvaluations = { ...initialEvaluations, [purchaseId]: { ...a, ...supported, availability: 'sold', canAdd: false, canPack: false, version: 'sold' } };
  rerender(<Harness evaluations={liveEvaluations} />);
  await waitFor(() => expect(screen.getByRole('button', { name: 'Save price' })).toBeDisabled());
  expect(screen.getByText('Unavailable: sold', { exact: true })).toBeVisible();
  expect(screen.queryByRole('button', { name: /Pack|Add to show/ })).not.toBeInTheDocument();
  expect(liveEvaluations[purchaseId].canPack).toBe(false);
});
it('offers mobile return to queue and restores focus to its review control', async () => {
  const { user } = setup();
  await user.click(screen.getByRole('button', { name: 'Review Orbit Fox' }));
  await user.click(screen.getByRole('button', { name: 'Back to inventory list' }));
  expect(screen.getByRole('button', { name: 'Review Orbit Fox' })).toHaveFocus();
});
it('blocks duplicate saves across view unmount/remount while preserving the submitted draft', async () => {
  let complete!: () => void;
  save.mockImplementation(() => new Promise<void>(resolve => { complete = resolve; }));
  const { user } = setup();
  await user.clear(screen.getByLabelText('Asking price')); await user.type(screen.getByLabelText('Asking price'), '2400');
  await user.click(screen.getByRole('button', { name: 'Save price' }));
  await user.click(screen.getByRole('button', { name: 'Switch view' }));
  await user.click(screen.getByRole('button', { name: 'Switch view' }));
  expect(screen.getByLabelText('Asking price')).toBeDisabled();
  expect(screen.getByLabelText('Asking price')).toHaveValue('2400');
  await act(async () => complete());
  expect(save).toHaveBeenCalledTimes(1);
});
it('rechecks the submitted card, not the newly focused card, when a save finishes after navigation', async () => {
  let complete!: () => void;
  save.mockImplementation(() => new Promise<void>(resolve => { complete = resolve; }));
  const { user } = setup();
  await user.clear(screen.getByLabelText('Asking price')); await user.type(screen.getByLabelText('Asking price'), '2400');
  await user.click(screen.getByRole('button', { name: 'Save price' }));
  await user.click(screen.getByRole('button', { name: 'Review Orbit Fox' }));
  await user.clear(screen.getByLabelText('Asking price')); await user.type(screen.getByLabelText('Asking price'), '1900');
  await waitFor(() => expect(showPrepAPI.evidence).toHaveBeenCalledTimes(2));
  await act(async () => complete());
  await waitFor(() => expect(vi.mocked(showPrepAPI.evidence).mock.calls.map(call => call[0])).toEqual([purchaseId, otherId, purchaseId]));
  expect(screen.getByLabelText('Asking price')).toHaveValue('1900');
  expect(screen.queryByText(/Price saved locally/)).not.toBeInTheDocument();
});
it('handles initially empty inventory without price actions or evidence requests', () => {
  render(<QueryClientProvider client={qc}><Harness items={[]} evaluations={{}} /></QueryClientProvider>);
  expect(screen.getByText('No inventory to review')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Save price' })).not.toBeInTheDocument();
  expect(showPrepAPI.evidence).not.toHaveBeenCalled();
});
