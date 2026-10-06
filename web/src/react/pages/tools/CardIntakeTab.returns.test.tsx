import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../../../js/api';
import type { ConfirmedReturnState, Purchase, Sale } from '../../../types/campaigns';
import CardIntakeTab from './CardIntakeTab';
import { returnActionLabel, rowIsListable } from './cardIntakeTypes';
import { loadQueue, saveQueue } from './cardIntakeStorage';

const cards = [
  { cert: '160944741', inventoryId: 147840, externalSale: 443, name: 'M Charizard EX', grade: 3 },
  { cert: '162787413', inventoryId: 364577, externalSale: 848, name: 'Spheal', grade: 6 },
];
const stamp = '2026-10-03T12:00:00Z';

function purchase(card = cards[1]): Purchase {
  return {
    id: `p-${card.inventoryId}`, campaignId: 'external', certNumber: card.cert,
    cardName: card.name, gradeValue: card.grade, buyCostCents: 1000, clValueCents: 5000, receivedAt: stamp,
    psaSourcingFeeCents: 0, purchaseDate: '2026-09-01', createdAt: stamp, updatedAt: stamp,
    dhInventoryId: card.inventoryId, dhCardId: 100, dhStatus: 'in_stock', dhPushStatus: 'pending',
  };
}

function sale(p: Purchase, id = 'old-sale'): Sale {
  return {
    id, purchaseId: p.id, saleChannel: 'ebay', salePriceCents: 4000, saleFeeCents: 0,
    saleDate: '2026-09-10', daysToSell: 9, netProfitCents: 3000,
    forcedLiquidation: false, createdAt: stamp, updatedAt: stamp,
  };
}

function state(p = purchase()): ConfirmedReturnState {
  return { operation: null, expectedSaleId: null, awaitingListing: false, precedingAttempt: null, purchase: p, sale: null };
}

function soldState(p: Purchase): ConfirmedReturnState {
  return { ...state(p), sale: sale(p), expectedSaleId: 'old-sale' };
}

function completed(initial: ConfirmedReturnState, externalSale = 848): ConfirmedReturnState {
  const p = initial.purchase ?? purchase();
  return {
    ...initial, expectedSaleId: null, sale: null, awaitingListing: true, outcome: 'completed',
    operation: {
      id: 'return-operation', purchaseId: p.id, capturedPurchaseId: p.id,
      dhInventoryId: p.dhInventoryId ?? 0, certNumber: p.certNumber, grader: 'PSA',
      expectedSaleId: initial.expectedSaleId, capturedOrderId: '', returnedOrderId: `ext-${externalSale}`,
      state: 'completed', createdAt: stamp, completedAt: stamp,
    },
  };
}

function seed(p: Purchase, status: 'existing' | 'sold' = 'existing') {
  saveQueue(new Map([[p.certNumber, {
    certNumber: p.certNumber, purchaseId: p.id, campaignId: p.campaignId,
    cardName: p.cardName, status, dhInventoryId: p.dhInventoryId,
    dhCardId: p.dhCardId, dhStatus: p.dhStatus,
    market: { lastSoldCents: 4000, gradePriceCents: 4000, clValueCents: 5000 },
  }]]));
  expect(loadQueue().get(p.certNumber)?.purchaseId).toBe(p.id);
}

function transport(initial: ConfirmedReturnState, externalSale = 848) {
  let durable = initial;
  vi.spyOn(api, 'fetchWithRetry').mockRejectedValue(new Error('Unexpected API request'));
  const read = vi.spyOn(api, 'getConfirmedReturnState').mockImplementation(async () => durable);
  const confirm = vi.spyOn(api, 'confirmPurchaseReturn').mockImplementation(async () => {
    durable = completed(durable, externalSale);
    return durable;
  });
  const scan = vi.spyOn(api, 'scanCert').mockImplementation(async () => {
    const p = durable.purchase ?? purchase();
    return {
      status: durable.sale ? 'sold' : 'existing', purchaseId: p.id, campaignId: p.campaignId,
      cardName: p.cardName, dhInventoryId: p.dhInventoryId, dhCardId: p.dhCardId,
      dhStatus: p.dhStatus, market: { lastSoldCents: 4000, gradePriceCents: 4000, clValueCents: 5000 },
    };
  });
  const remove = vi.spyOn(api, 'deleteSale').mockResolvedValue(undefined);
  const list = vi.spyOn(api, 'listPurchaseOnDH').mockImplementation(async () => {
    durable = { ...durable, awaitingListing: false,
      purchase: durable.purchase ? { ...durable.purchase, dhStatus: 'listed' } : null };
    return { listed: 1, synced: 1, skipped: 0, total: 1 };
  });
  vi.spyOn(api, 'setReviewedPrice').mockResolvedValue({ success: true, reviewedAt: stamp });
  return { read, confirm, scan, remove, list, setState: (s: ConfirmedReturnState) => { durable = s; } };
}

beforeEach(() => {
  const saved = new Map<string, string>();
  vi.spyOn(localStorage, 'getItem').mockImplementation(key => saved.get(key) ?? null);
  vi.spyOn(localStorage, 'setItem').mockImplementation((key, value) => { saved.set(key, value); });
  vi.spyOn(localStorage, 'removeItem').mockImplementation(key => { saved.delete(key); });
  vi.spyOn(localStorage, 'clear').mockImplementation(() => saved.clear());
});
afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); });

describe('confirmed Cert Intake returns', () => {
  it('does not authorize listing or a return from a different purchase projection', () => {
    const p = purchase();
    const row = {
      certNumber: p.certNumber, purchaseId: 'replacement-purchase', status: 'existing' as const,
      dhInventoryId: p.dhInventoryId, dhStatus: p.dhStatus,
      market: { clValueCents: 5000, lastSoldCents: 4000, gradePriceCents: 4000 },
      returnState: completed(state(p)),
    };
    expect(rowIsListable(row)).toBe(false);
    expect(returnActionLabel(row)).toBeNull();
  });
  it.each(cards)('returns recorded sale $cert only after confirmation, then explicitly lists', async card => {
    const p = purchase(card);
    const mocks = transport(soldState(p), card.externalSale);
    seed(p, 'sold');
    const user = userEvent.setup();
    render(<CardIntakeTab />);
    await user.click(await screen.findByRole('button', { name: 'Return' }));
    const dialog = await screen.findByRole('alertdialog');
    expect(dialog).toHaveTextContent(card.cert);
    expect(dialog).toHaveTextContent(/physically back/i);
    expect(dialog).toHaveTextContent(/refund.*resolved/i);
    expect(mocks.confirm).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: 'Confirm return' }));
    await waitFor(() => expect(mocks.confirm).toHaveBeenCalledWith(p.id, { returnConfirmed: true, expectedSaleId: 'old-sale' }));
    await screen.findByText(/Returned; review the price and list explicitly/i);
    expect(mocks.scan).toHaveBeenCalledWith(card.cert);
    expect(mocks.remove).not.toHaveBeenCalled();
    expect(mocks.list).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'List on DH' }));
    await waitFor(() => expect(mocks.list).toHaveBeenCalledWith(p.id));
    expect(api.setReviewedPrice).toHaveBeenCalledWith(p.id, 4000, 'market');
    await waitFor(() => expect(screen.queryByText(/Returned; review the price and list explicitly/i)).not.toBeInTheDocument());
  });

  it('does not request another List when a successful listing follow-up read fails', async () => {
    const p = purchase();
    const mocks = transport(soldState(p));
    seed(p, 'sold');
    const user = userEvent.setup();
    render(<CardIntakeTab />);
    await user.click(await screen.findByRole('button', { name: 'Return' }));
    await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Confirm return' }));
    await screen.findByText(/Returned; review the price and list explicitly/i);
    mocks.read.mockRejectedValueOnce(new Error('Return state refresh failed'));
    await user.click(screen.getByRole('button', { name: 'List on DH' }));
    await screen.findByText('Return state refresh failed');
    expect(mocks.list).toHaveBeenCalledTimes(1);
    expect(screen.queryByText(/Returned; review the price and list explicitly/i)).not.toBeInTheDocument();
  });

  it('cancelling confirmation never deletes a sale or returns remotely', async () => {
    const p = purchase();
    const mocks = transport(soldState(p));
    seed(p, 'sold');
    const user = userEvent.setup();
    render(<CardIntakeTab />);
    await user.click(await screen.findByRole('button', { name: 'Return' }));
    const dialog = await screen.findByRole('alertdialog');
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(mocks.confirm).not.toHaveBeenCalled();
    expect(mocks.remove).not.toHaveBeenCalled();
  });

  it('reloads pending completion from the server and reuses its captured sale and operation', async () => {
    const p = purchase();
    const initial = state(p);
    const pending = completed({ ...initial, expectedSaleId: 'original-sale' });
    pending.operation = {
      ...pending.operation!, state: 'pending', completedAt: undefined,
      observedReceipt: { dhInventoryId: 364577, itemStatus: 'in_stock', externalSaleId: 848, restored: true },
      lastError: { code: 'local_completion_failed', message: 'Retry completion', phase: 'completion' },
    };
    pending.awaitingListing = false;
    const mocks = transport(pending);
    seed(p);
    const user = userEvent.setup();
    render(<CardIntakeTab />);
    await user.click(await screen.findByRole('button', { name: 'Retry completion' }));
    await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Confirm return' }));
    await waitFor(() => expect(mocks.confirm).toHaveBeenCalledWith(p.id, {
      returnConfirmed: true, expectedSaleId: 'original-sale', operationId: 'return-operation',
    }));
    expect(mocks.list).not.toHaveBeenCalled();
  });

  it('recognizes completion after a lost response without sending a fresh return', async () => {
    const p = purchase();
    const initial = soldState(p);
    const mocks = transport(initial);
    mocks.confirm.mockImplementation(async () => {
      mocks.setState(completed(initial));
      throw new Error('Response lost');
    });
    seed(p, 'sold');
    const user = userEvent.setup();
    render(<CardIntakeTab />);
    await user.click(await screen.findByRole('button', { name: 'Return' }));
    await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Confirm return' }));
    await screen.findByText(/Returned; review the price and list explicitly/i);
    expect(mocks.confirm).toHaveBeenCalledTimes(1);
    expect(screen.queryByText('Response lost')).not.toBeInTheDocument();
    expect(mocks.list).not.toHaveBeenCalled();
  });

  it('rechecks an open intake listing automatically and restores List only after durable settlement', async () => {
    const p = purchase();
    const initial = state(p);
    initial.precedingAttempt = {
      id: 'intake-list', capturedPurchaseId: p.id, dhInventoryId: p.dhInventoryId ?? 0,
      certNumber: p.certNumber, grader: 'PSA', kind: 'list', phase: 'intake_patch_sync', startedAt: stamp, outcome: 'open',
    };
    const mocks = transport(initial);
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-10-03T12:00:10Z'));
    seed(p);
    await act(async () => { render(<CardIntakeTab />); });
    expect(screen.getByText(/DH listing sync in progress/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Return|Refresh return state|List on DH/i })).not.toBeInTheDocument();
    expect(screen.queryByText(/return/i)).not.toBeInTheDocument();
    mocks.setState(state(p));
    await act(async () => { await vi.advanceTimersByTimeAsync(4000); });
    expect(screen.queryByText(/DH listing sync in progress/i)).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'List on DH' })).toBeEnabled();
    expect(screen.queryByRole('button', { name: /Return|Refresh return state/i })).not.toBeInTheDocument();
    expect(mocks.read).toHaveBeenCalledTimes(2);
    expect(mocks.scan).not.toHaveBeenCalled();
    expect(mocks.confirm).not.toHaveBeenCalled();
    expect(mocks.list).not.toHaveBeenCalled();
  });

  it('marks a persistent intake listing attempt as an unresolved DH hold, not a return', async () => {
    const p = purchase();
    const initial = state(p);
    initial.precedingAttempt = {
      id: 'old-intake-list', capturedPurchaseId: p.id, dhInventoryId: p.dhInventoryId ?? 0,
      certNumber: p.certNumber, grader: 'PSA', kind: 'list', phase: 'intake_patch_sync', startedAt: stamp, outcome: 'open',
    };
    const mocks = transport(initial);
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-10-03T12:01:10Z'));
    seed(p);
    await act(async () => { render(<CardIntakeTab />); });
    expect(screen.getByText(/DH mutation unresolved \(list: intake_patch_sync\)/i)).toBeInTheDocument();
    await act(async () => { await vi.advanceTimersByTimeAsync(4000); });
    expect(screen.getByText(/DH mutation unresolved \(list: intake_patch_sync\)/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Return|Refresh return state|List on DH/i })).not.toBeInTheDocument();
    expect(mocks.read).toHaveBeenCalledTimes(2);
    expect(mocks.scan).not.toHaveBeenCalled();
  });

  it('keeps an unresolved listing hold after read failure without branding it a return', async () => {
    const p = purchase();
    const initial = state(p);
    initial.precedingAttempt = {
      id: 'uncertain-patch', capturedPurchaseId: p.id, dhInventoryId: p.dhInventoryId ?? 0,
      certNumber: p.certNumber, grader: 'PSA', kind: 'list', phase: 'patch', startedAt: stamp, outcome: 'open',
    };
    const mocks = transport(initial);
    seed(p);
    vi.useFakeTimers();
    await act(async () => { render(<CardIntakeTab />); });
    expect(screen.getByText(/DH mutation unresolved/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Return|Refresh return state|List on DH/i })).not.toBeInTheDocument();
    mocks.read.mockRejectedValueOnce(new Error('State read failed'));
    await act(async () => { await vi.advanceTimersByTimeAsync(4000); });
    expect(screen.getByText('State read failed')).toBeInTheDocument();
    expect(screen.getByText(/DH state read failed/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Return|Refresh return state|List on DH/i })).not.toBeInTheDocument();
    expect(mocks.scan).not.toHaveBeenCalled();
    expect(mocks.confirm).not.toHaveBeenCalled();
    expect(mocks.list).not.toHaveBeenCalled();
    mocks.setState(state(p));
    await act(async () => { await vi.advanceTimersByTimeAsync(4000); });
    expect(screen.getByRole('button', { name: 'List on DH' })).toBeEnabled();
    expect(screen.queryByText(/DH state read failed/i)).not.toBeInTheDocument();
  });

  it('does not overlap state reads, then stops polling after settlement and unmount', async () => {
    const p = purchase();
    const open = state(p);
    open.precedingAttempt = {
      id: 'intake-list', capturedPurchaseId: p.id, dhInventoryId: p.dhInventoryId ?? 0,
      certNumber: p.certNumber, grader: 'PSA', kind: 'list', phase: 'intake_patch_sync', startedAt: stamp, outcome: 'open',
    };
    const mocks = transport(open);
    let finishRead: ((value: ConfirmedReturnState) => void) | undefined;
    const slowRead = new Promise<ConfirmedReturnState>(resolve => { finishRead = resolve; });
    mocks.read.mockResolvedValueOnce(open).mockImplementationOnce(() => slowRead).mockResolvedValue(state(p));
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-10-03T12:00:10Z'));
    seed(p);
    let unmount = () => {};
    await act(async () => { ({ unmount } = render(<CardIntakeTab />)); });
    await act(async () => { await vi.advanceTimersByTimeAsync(8000); });
    expect(mocks.read).toHaveBeenCalledTimes(2);
    expect(screen.queryByRole('button', { name: 'List on DH' })).not.toBeInTheDocument();
    await act(async () => { finishRead?.(open); });
    await act(async () => { await vi.advanceTimersByTimeAsync(4000); });
    expect(mocks.read).toHaveBeenCalledTimes(3);
    expect(screen.getByRole('button', { name: 'List on DH' })).toBeEnabled();
    await act(async () => { await vi.advanceTimersByTimeAsync(8000); });
    expect(mocks.read).toHaveBeenCalledTimes(3);
    unmount();
    await act(async () => { await vi.advanceTimersByTimeAsync(4000); });
    expect(mocks.read).toHaveBeenCalledTimes(3);
    expect(mocks.scan).not.toHaveBeenCalled();
  });

  it('drops a slow old-purchase state read after the cert is dismissed and rescanned', async () => {
    const p = purchase();
    const replacement = { ...p, id: 'replacement-purchase', cardName: 'Replacement slab', dhInventoryId: 999 };
    const mocks = transport(state(p));
    let releaseOld: ((value: ConfirmedReturnState) => void) | undefined;
    const slowOld = new Promise<ConfirmedReturnState>(resolve => { releaseOld = resolve; });
    mocks.read.mockImplementation(id => id === p.id ? slowOld : Promise.resolve(state(replacement)));
    mocks.scan.mockResolvedValue({
      status: 'existing', purchaseId: replacement.id, campaignId: replacement.campaignId,
      cardName: replacement.cardName, dhInventoryId: replacement.dhInventoryId,
      dhCardId: replacement.dhCardId, dhStatus: replacement.dhStatus,
      market: { lastSoldCents: 4000, gradePriceCents: 4000, clValueCents: 5000 },
    });
    seed(p);
    const user = userEvent.setup();
    render(<CardIntakeTab />);
    await waitFor(() => expect(mocks.read).toHaveBeenCalledWith(p.id));
    await user.click(screen.getByRole('button', { name: 'Dismiss' }));
    const input = screen.getByPlaceholderText('Scan or type cert number…');
    fireEvent.change(input, { target: { value: p.certNumber } });
    fireEvent.keyDown(input, { key: 'Enter' });
    await waitFor(() => expect(mocks.read).toHaveBeenCalledWith(replacement.id));
    expect(await screen.findByRole('button', { name: 'List on DH' })).toBeEnabled();
    await act(async () => { releaseOld?.(state(p)); });
    expect(screen.getByText('Replacement slab')).toBeInTheDocument();
    expect(screen.queryByText(p.cardName)).not.toBeInTheDocument();
    expect(mocks.scan).toHaveBeenCalledTimes(1);
  });

  it('keeps a retained awaiting-listing hold on a mismatched target in diagnosis', async () => {
    const p = purchase();
    const held = completed(state(p));
    held.purchase = { ...p, dhInventoryId: 999 };
    transport(held);
    seed(p);
    render(<CardIntakeTab />);
    await screen.findByText(/Return identity conflict/i);
    expect(screen.queryByRole('button', { name: 'List on DH' })).not.toBeInTheDocument();
    expect(screen.queryByText(/Returned; review the price/i)).not.toBeInTheDocument();
  });

  it('refreshes a failed state lookup without inventing a sale precondition', async () => {
    const p = purchase();
    const mocks = transport(state(p));
    mocks.read.mockRejectedValue(new Error('DH state unavailable'));
    seed(p);
    render(<CardIntakeTab />);
    await screen.findByText(/DH state unavailable/i);
    expect(screen.getByText(/DH state read failed/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Return|Refresh return state/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'List on DH' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Fix DH Match' })).not.toBeInTheDocument();
    expect(mocks.list).not.toHaveBeenCalled();
    expect(mocks.confirm).not.toHaveBeenCalled();
    expect(mocks.remove).not.toHaveBeenCalled();
  });

  it('rehydrates a queue saved while a return was in flight instead of staying busy', async () => {
    const p = purchase();
    const initial = state(p);
    const pending = completed(initial);
    pending.operation = { ...pending.operation!, state: 'pending', completedAt: undefined };
    pending.awaitingListing = false;
    const mocks = transport(pending);
    saveQueue(new Map([[p.certNumber, {
      certNumber: p.certNumber, purchaseId: p.id, status: 'existing',
      dhInventoryId: p.dhInventoryId, returnBusy: true, returnLoading: true,
      returnState: completed(initial),
    }]]));
    render(<CardIntakeTab />);
    const retry = await screen.findByRole('button', { name: 'Retry return' });
    expect(retry).toBeEnabled();
    expect(mocks.read).toHaveBeenCalledWith(p.id);
    expect(mocks.confirm).not.toHaveBeenCalled();
  });

  it('keeps a permanent attribution conflict actionable and diagnosis-only', async () => {
    const p = purchase(cards[0]);
    const initial = soldState(p);
    const mocks = transport(initial, 443);
    mocks.confirm.mockImplementationOnce(async () => {
      const pending = completed(initial, 443);
      pending.operation = { ...pending.operation!, state: 'conflicted', completedAt: undefined,
        lastError: { code: 'sale_attribution_missing', message: 'This item has no attributable external sale.', phase: 'dispatch' } };
      pending.awaitingListing = false;
      mocks.setState(pending);
      throw new Error('This item has no attributable external sale.');
    });
    seed(p, 'sold');
    const user = userEvent.setup();
    render(<CardIntakeTab />);
    await user.click(await screen.findByRole('button', { name: 'Return' }));
    await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Confirm return' }));
    await screen.findByText('This item has no attributable external sale.');
    expect(screen.queryByText(/push pending/i)).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Refresh return state' }));
    expect(mocks.confirm).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('button', { name: 'Retry return' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'List on DH' })).not.toBeInTheDocument();
    expect(mocks.remove).not.toHaveBeenCalled();
  });

  it('does not transfer the old return busy flag to a purchase recreated during scan', async () => {
    const p = purchase();
    const initial = soldState(p);
    const newPurchase = { ...p, id: 'recreated-purchase' };
    const returned = completed(initial);
    const recreated = { ...returned, purchase: newPurchase,
      operation: returned.operation ? { ...returned.operation, purchaseId: null } : null };
    const mocks = transport(initial);
    let releaseOld: ((value: ConfirmedReturnState) => void) | undefined;
    const delayedOldRead = new Promise<ConfirmedReturnState>(resolve => { releaseOld = resolve; });
    let oldReads = 0;
    mocks.read.mockImplementation(async id => {
      if (id === newPurchase.id) return recreated;
      return ++oldReads > 2 ? delayedOldRead : initial;
    });
    mocks.scan.mockResolvedValue({
      status: 'existing', purchaseId: newPurchase.id, campaignId: p.campaignId,
      cardName: p.cardName, dhInventoryId: p.dhInventoryId, dhCardId: p.dhCardId, dhStatus: 'in_stock',
      market: { lastSoldCents: 4000, gradePriceCents: 4000, clValueCents: 5000 },
    });
    seed(p, 'sold');
    const user = userEvent.setup();
    render(<CardIntakeTab />);
    await user.click(await screen.findByRole('button', { name: 'Return' }));
    await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Confirm return' }));
    // Let the replacement render and hydrate before releasing the old-owner
    // read, matching network latency rather than React's same-tick batching.
    await waitFor(() => expect(mocks.read).toHaveBeenCalledWith(newPurchase.id));
    releaseOld?.(initial);
    const list = await screen.findByRole('button', { name: 'List on DH' });
    expect(list).toBeEnabled();
    expect(screen.queryByRole('button', { name: 'Returning…' })).not.toBeInTheDocument();
    expect(mocks.list).not.toHaveBeenCalled();
    await user.click(list);
    await waitFor(() => expect(mocks.list).toHaveBeenCalledWith(newPurchase.id));
  });

  it('does not mistake historical completion for success when the later sale return fails', async () => {
    const p = purchase();
    const old = completed(state(p));
    const newer = { ...old, sale: sale(p, 'new-sale'), expectedSaleId: 'new-sale', awaitingListing: false };
    const mocks = transport(newer);
    mocks.confirm.mockRejectedValue(new Error('Sale precondition failed; rescan before retrying.'));
    seed(p);
    const user = userEvent.setup();
    render(<CardIntakeTab />);
    await user.click(await screen.findByRole('button', { name: 'Return' }));
    await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Confirm return' }));
    await screen.findByText('Sale precondition failed; rescan before retrying.');
    expect(screen.getByText('⚠ Sold')).toBeInTheDocument();
    expect(mocks.list).not.toHaveBeenCalled();
  });

  it('does not reuse a historical completed operation to return a later sale', async () => {
    const p = purchase();
    const old = completed(state(p));
    const newer = { ...old, sale: sale(p, 'new-sale'), expectedSaleId: 'new-sale', awaitingListing: false };
    const mocks = transport(newer);
    seed(p);
    const user = userEvent.setup();
    render(<CardIntakeTab />);
    await user.click(await screen.findByRole('button', { name: 'Return' }));
    await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Confirm return' }));
    await waitFor(() => expect(mocks.confirm).toHaveBeenCalledWith(p.id, { returnConfirmed: true, expectedSaleId: 'new-sale' }));
  });
});
