import { useState, type ReactNode } from 'react';
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { showPrepAPI } from '../../../js/api/showprep';
import { priceReviewAPI } from '../../../js/api/priceReview';
import type { ShowEvaluation } from '../../../types/showprep';
import { PriceReviewPanel } from './PriceReviewPanel';
import { usePriceReviewState } from './usePriceReviewState';
import type { PriceDraft } from './priceReviewModel';
import { evaluation, inventoryItem, preview, purchaseId, sales } from './fixtures.test-support';

const clients: QueryClient[] = [];
function provider() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } }); clients.push(qc);
  return { qc, wrapper: ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider> };
}
let saved: ShowEvaluation;
beforeEach(() => {
  saved = evaluation();
  vi.spyOn(showPrepAPI, 'evidence').mockImplementation(async () => ({ evaluation: saved, sales }));
});
afterEach(() => { cleanup(); clients.forEach(qc => qc.clear()); clients.length = 0; vi.restoreAllMocks(); vi.unstubAllGlobals(); });
function setup(initialDraft?: PriceDraft, onSavePrice = vi.fn(async (_id: string, _cents: number) => {})) {
  const { qc, wrapper } = provider();
  function Harness({ e = saved }: { e?: ShowEvaluation }) {
    const [draft, setDraft] = useState(initialDraft);
    const review = usePriceReviewState([inventoryItem(e)], { [purchaseId]: e }, '');
    return <PriceReviewPanel purchaseId={purchaseId} item={inventoryItem(e)} evaluation={e} draft={draft}
      onDraftChange={setDraft} onClearDraft={() => setDraft(undefined)} onSavePrice={onSavePrice}
      save={review.saves[purchaseId]} onSaveResultChange={result => review.setSaveResult(purchaseId, result)}
      onSaveRechecked={rechecked => review.markSaveRechecked(purchaseId, rechecked)} />;
  }
  return { ...render(<Harness />, { wrapper }), qc, Harness, onSavePrice, user: userEvent.setup() };
}

it('uses the server trial outcome and only persists after explicit Save; saved badge and amount are never hypothetical', async () => {
  vi.spyOn(priceReviewAPI, 'preview').mockImplementation(async (_id, cents) => preview(cents, saved));
  const save = vi.fn(async (_id: string, cents: number) => {
    const assessed = preview(cents, saved);
    saved = { ...saved, version: 'saved-b', localPriceCents: cents, status: assessed.status, reason: assessed.reason, recent: assessed.recent };
  });
  const { user } = setup(undefined, save);
  await user.clear(screen.getByLabelText('Asking price')); await user.type(screen.getByLabelText('Asking price'), '2400');
  expect(save).not.toHaveBeenCalled();
  await within(screen.getByRole('region', { name: 'Trial price assessment' })).findByText('Supported', { exact: true });
  expect(screen.getByLabelText('Saved asking price')).toHaveTextContent('$2,800.00');
  expect(within(screen.getByRole('region', { name: 'Saved price assessment' })).getByText('Asking above comps')).toBeInTheDocument();
  expect(screen.getByText(/Saving syncs DH and can list eligible inventory/)).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Save price' }));
  expect(save).toHaveBeenCalledExactlyOnceWith(purchaseId, 240000);
  await waitFor(() => expect(screen.getByLabelText('Saved asking price')).toHaveTextContent('$2,400.00'));
  expect(screen.getByText(/Price saved locally/)).toBeInTheDocument();
});
it('invalidates the displayed trial immediately during debounce and cannot show a late 254000 result over current 240000', async () => {
  const requests: { cents: number; release: (data: ReturnType<typeof preview>) => void }[] = [];
  // Keep the real preview hook, API validation, transport and QueryClient. Only HTTP is controlled.
  vi.stubGlobal('fetch', vi.fn((_url: string, opts: RequestInit) => new Promise<Response>(resolve => {
    const { priceCents } = JSON.parse(String(opts.body));
    requests.push({ cents: priceCents, release: data => resolve(new Response(JSON.stringify(data))) });
  })));
  const { user } = setup();
  const input = screen.getByLabelText('Asking price');
  await user.clear(input); await user.type(input, '2540');
  await waitFor(() => expect(requests.map(r => r.cents)).toEqual([254000]));
  await user.clear(input); await user.type(input, '2400');
  expect(screen.getByRole('region', { name: 'Trial price assessment' })).toHaveTextContent(/Waiting|Checking/);
  await waitFor(() => expect(requests.map(r => r.cents)).toEqual([254000, 240000]));
  await act(async () => requests[1].release(preview(240000)));
  await within(screen.getByRole('region', { name: 'Trial price assessment' })).findByText('Supported', { exact: true });
  await act(async () => requests[0].release(preview(254000)));
  expect(screen.queryByText('Mixed evidence')).not.toBeInTheDocument();
  await user.type(input, '1');
  expect(within(screen.getByRole('region', { name: 'Trial price assessment' })).queryByText('Supported', { exact: true })).not.toBeInTheDocument();
});
it('retains the input on preview failure, allows an explicit read retry, and does not autosave', async () => {
  const read = vi.spyOn(priceReviewAPI, 'preview').mockRejectedValueOnce(new Error('Preview offline')).mockImplementation(async (_id, cents) => preview(cents));
  const { user, onSavePrice } = setup({ value: '2400', baselinePriceCents: 280000 });
  expect(await screen.findByText(/Preview offline/)).toBeInTheDocument();
  expect(screen.getByLabelText('Asking price')).toHaveValue('2400');
  expect(onSavePrice).not.toHaveBeenCalled(); expect(read).toHaveBeenCalledTimes(1);
  await user.click(screen.getByRole('button', { name: 'Retry preview' }));
  await within(screen.getByRole('region', { name: 'Trial price assessment' })).findByText('Supported', { exact: true });
});
it('keeps a changed saved baseline visible while preserving dirty input', async () => {
  vi.spyOn(priceReviewAPI, 'preview').mockImplementation(async (_id, cents) => preview(cents, saved));
  const { Harness, rerender } = setup({ value: '2400', baselinePriceCents: 280000 });
  await within(screen.getByRole('region', { name: 'Trial price assessment' })).findByText('Supported', { exact: true });
  saved = { ...saved, localPriceCents: 270000, version: 'changed', recent: { ...saved.recent, gapPct: 11.111111 } }; rerender(<Harness e={saved} />);
  expect(screen.getByLabelText('Asking price')).toHaveValue('2400');
  expect(await screen.findByText(/Saved asking changed from \$2,800.00 to \$2,700.00/)).toBeInTheDocument();
});
it('requires rechecking when the preview observes a different saved price, without publishing it as saved', async () => {
  vi.spyOn(priceReviewAPI, 'preview').mockResolvedValue({ ...preview(), currentPriceCents: 270000 });
  const { user, onSavePrice } = setup({ value: '2400', baselinePriceCents: 280000 });
  await screen.findByText(/Preview observed a different saved asking price/);
  expect(screen.getByLabelText('Saved asking price')).toHaveTextContent('$2,800.00');
  expect(screen.getByRole('button', { name: 'Save price' })).toBeDisabled();
  saved = { ...saved, localPriceCents: 270000, version: 'changed', recent: { ...saved.recent, gapPct: 11.111111 } };
  await user.click(screen.getByRole('button', { name: 'Recheck saved state' }));
  await waitFor(() => expect(screen.getByLabelText('Saved asking price')).toHaveTextContent('$2,700.00'));
  expect(onSavePrice).not.toHaveBeenCalled();
});
it('disables duplicate saves and retains text after uncertain failure; rechecks rather than replaying a write', async () => {
  vi.spyOn(priceReviewAPI, 'preview').mockImplementation(async (_id, cents) => preview(cents));
  let fail!: (error: Error) => void;
  const save = vi.fn((_id: string, _cents: number) => new Promise<void>((_resolve, reject) => { fail = reject; }));
  const { user } = setup({ value: '2400', baselinePriceCents: 280000 }, save);
  await within(screen.getByRole('region', { name: 'Trial price assessment' })).findByText('Supported', { exact: true });
  await user.dblClick(screen.getByRole('button', { name: 'Save price' }));
  expect(save).toHaveBeenCalledTimes(1); expect(screen.getByLabelText('Asking price')).toBeDisabled();
  await act(async () => fail(new Error('Connection lost')));
  await screen.findByText(/Save not confirmed: Connection lost/);
  expect(screen.getByLabelText('Asking price')).toHaveValue('2400');
  await waitFor(() => expect(showPrepAPI.evidence).toHaveBeenCalledTimes(2));
  expect(save).toHaveBeenCalledTimes(1);
});
it.each(['0', '12oops', '1.001', '90071992547409.92'])('never previews or saves invalid input %s', async value => {
  const read = vi.spyOn(priceReviewAPI, 'preview');
  const { user, onSavePrice } = setup({ value, baselinePriceCents: 280000 });
  expect(screen.getByRole('button', { name: 'Save price' })).toBeDisabled();
  await user.click(screen.getByRole('button', { name: 'Save price' }));
  expect(read).not.toHaveBeenCalled(); expect(onSavePrice).not.toHaveBeenCalled();
});
it('shows low single-sale facts even for Limited, safe matching-sale links and progressive older context', async () => {
  saved = evaluation({ status: 'thin_evidence', reason: 'One recent sale is not enough to establish support.',
    recent: { ...evaluation().recent, count: 1, saleIds: ['sale-a'], medianCents: 200000, gapPct: 28.571429 } });
  vi.mocked(showPrepAPI.evidence).mockImplementation(async () => ({ evaluation: saved,
    sales: sales.map(sale => sale.id === 'sale-b' ? { ...sale, date: '2026-08-30' } : sale) }));
  const { user, onSavePrice } = setup();
  const matchingLinks = await screen.findAllByRole('link', { name: /Auction house sale/ });
  expect(matchingLinks[0]).toBeVisible();
  expect(within(screen.getByRole('region', { name: 'Saved price assessment' })).getByText('Limited evidence', { exact: true })).toBeInTheDocument();
  expect(screen.getByLabelText('Recent single sale')).toHaveTextContent('$2,000.00');
  expect(screen.getByLabelText('Recent single sale')).toHaveClass('price-review-low');
  expect(screen.queryByRole('link', { name: /Marketplace/ })).not.toBeInTheDocument();
  expect(screen.getByRole('link', { name: /Older auction/ })).not.toBeVisible();
  await user.click(screen.getByText('30-day sale history'));
  expect(screen.getByRole('link', { name: /Older auction/ })).toHaveAttribute('rel', 'noopener noreferrer');
  expect(document.querySelector('a[href^="javascript:"]')).toBeNull();
  await user.click(screen.getByRole('button', { name: /Try recent reference/ }));
  expect(screen.getByLabelText('Asking price')).toHaveValue('2000.00');
  expect(onSavePrice).not.toHaveBeenCalled();
});
it('keeps physical readiness in diagnostics rather than ahead of the price assessment', async () => {
  const { user } = setup();
  expect(screen.getByText('Ready to pack')).not.toBeVisible();
  await user.click(screen.getByText('Source diagnostics'));
  expect(screen.getByText('Ready to pack')).toBeVisible();
});
it('labels stored partial facts as unverified and puts diagnostics after evidence, not as healthy no-sales evidence', async () => {
  saved = evaluation({ status: 'needs_review', reason: 'Stored snapshot is incomplete.', evidenceNeedsReview: true, evidenceReason: 'Snapshot incomplete', recent: { ...evaluation().recent, gapPct: null } });
  setup(); await screen.findAllByRole('link', { name: /Auction house sale/ });
  expect(screen.getByText(/Stored sales may be partial or stale/)).toBeInTheDocument();
  expect(screen.getByText('Evidence unavailable', { exact: true })).toBeInTheDocument();
  const evidence = screen.getByRole('heading', { name: 'Recent matching sales' });
  const diagnostics = screen.getByText('Source diagnostics');
  expect(evidence.compareDocumentPosition(diagnostics) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
});
