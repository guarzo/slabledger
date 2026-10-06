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
  const check = vi.spyOn(api, 'getDHSaleCheck').mockResolvedValue({
    status: 'sold', resolvable: true, reason: '',
    target: { dhInventoryId: initial.purchase?.dhInventoryId ?? 0, certNumber: initial.purchase?.certNumber ?? '', grader: 'PSA' },
  });
  const remove = vi.spyOn(api, 'deleteSale').mockResolvedValue(undefined);
  const list = vi.spyOn(api, 'listPurchaseOnDH').mockImplementation(async () => {
    durable = { ...durable, awaitingListing: false,
      purchase: durable.purchase ? { ...durable.purchase, dhStatus: 'listed' } : null };
    return { listed: 1, synced: 1, skipped: 0, total: 1 };
  });
  vi.spyOn(api, 'setReviewedPrice').mockResolvedValue({ success: true, reviewedAt: stamp });
  return { read, check, confirm, scan, remove, list, setState: (s: ConfirmedReturnState) => { durable = s; } };
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
  it.each(cards)('returns recorded sale $cert in one click, then explicitly lists', async card => {
    const p = purchase(card);
    const mocks = transport(soldState(p), card.externalSale);
    seed(p, 'sold');
    const user = userEvent.setup();
    render(<CardIntakeTab />);
    const button = await screen.findByRole('button', { name: 'Return' });
    expect(screen.getByText('Return confirms slab in hand and refund resolved')).toBeVisible();
    await user.click(button);
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
    await waitFor(() => expect(mocks.confirm).toHaveBeenCalledWith(p.id, {
      returnConfirmed: true, expectedSaleId: 'old-sale',
      expectedTarget: { dhInventoryId: card.inventoryId, certNumber: card.cert, grader: 'PSA' },
    }));
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
    await screen.findByText(/Returned; review the price and list explicitly/i);
    mocks.read.mockRejectedValueOnce(new Error('Return state refresh failed'));
    await user.click(screen.getByRole('button', { name: 'List on DH' }));
    await screen.findByText('Return state refresh failed');
    expect(mocks.list).toHaveBeenCalledTimes(1);
    expect(screen.queryByText(/Returned; review the price and list explicitly/i)).not.toBeInTheDocument();
  });

  it.each([
    { name: 'sale', change: (s: ConfirmedReturnState) => ({ ...s, sale: sale(s.purchase!, 'sale-B'), expectedSaleId: 'sale-B' }) },
    { name: 'target', change: (s: ConfirmedReturnState) => ({ ...s, purchase: { ...s.purchase!, dhInventoryId: 43 } }) },
  ])('does not retarget a Return when the fresh $name differs', async ({ change }) => {
    const p = purchase();
    const initial = soldState(p);
    const mocks = transport(initial);
    seed(p, 'sold');
    render(<CardIntakeTab />);
    await screen.findByRole('button', { name: 'Return' });
    mocks.setState(change(initial));
    await userEvent.click(screen.getByRole('button', { name: 'Return' }));
    await screen.findByText(/Return state changed.*rescan/i);
    expect(mocks.confirm).not.toHaveBeenCalled();
  });

  it.each([
    { name: 'sale', change: (s: ConfirmedReturnState) => ({ ...s, sale: sale(s.purchase!, 'sale-B'), expectedSaleId: 'sale-B' }), newSale: 'sale-B' },
    { name: 'target', change: (s: ConfirmedReturnState) => ({ ...s, purchase: { ...s.purchase!, dhInventoryId: 43 } }), newSale: 'old-sale' },
  ])('recovers a stale $name only after dismissing and physically rescanning', async ({ change, newSale }) => {
    const p = purchase();
    const initial = soldState(p);
    const changed = change(initial);
    const mocks = transport(initial);
    seed(p, 'sold');
    const user = userEvent.setup();
    render(<CardIntakeTab />);
    await screen.findByRole('button', { name: 'Return' });
    mocks.setState(changed);
    await user.click(screen.getByRole('button', { name: 'Return' }));
    expect(await screen.findByRole('button', { name: 'Dismiss to rescan' })).toBeVisible();
    expect(screen.queryByRole('button', { name: 'Return' })).not.toBeInTheDocument();
    // Entering the cert again must not quietly recapture a new sale or target.
    const input = screen.getByPlaceholderText('Scan or type cert number…');
    await user.type(input, `${p.certNumber}{Enter}`);
    expect(mocks.scan).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Dismiss to rescan' })).toBeVisible();
    expect(mocks.confirm).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Dismiss to rescan' }));
    expect(screen.queryByRole('button', { name: 'Return' })).not.toBeInTheDocument();
    await user.type(input, `${p.certNumber}{Enter}`);
    expect(await screen.findByRole('button', { name: 'Return' })).toBeEnabled();
    expect(mocks.scan).toHaveBeenCalledTimes(1);
    await user.click(screen.getByRole('button', { name: 'Return' }));
    await waitFor(() => expect(mocks.confirm).toHaveBeenCalledWith(p.id, {
      returnConfirmed: true, expectedSaleId: newSale,
      expectedTarget: { dhInventoryId: changed.purchase!.dhInventoryId!, certNumber: p.certNumber, grader: 'PSA' },
    }));
  });

  it('does not apply an old return response to a dismissed and re-added same-purchase row', async () => {
    const p = purchase();
    const initial = soldState(p);
    const newer = { ...initial, sale: sale(p, 'sale-B'), expectedSaleId: 'sale-B' };
    const mocks = transport(initial);
    let finish: ((value: ConfirmedReturnState) => void) | undefined;
    mocks.confirm.mockImplementationOnce(() => new Promise(resolve => { finish = resolve; }));
    seed(p, 'sold');
    const user = userEvent.setup();
    render(<CardIntakeTab />);
    await user.click(await screen.findByRole('button', { name: 'Return' }));
    await waitFor(() => expect(mocks.confirm).toHaveBeenCalledTimes(1));
    await user.click(screen.getByRole('button', { name: 'Clear all' }));
    await user.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Clear all' }));
    mocks.setState(newer);
    const input = screen.getByPlaceholderText('Scan or type cert number…');
    await user.type(input, `${p.certNumber}{Enter}`);
    expect(await screen.findByRole('button', { name: 'Return' })).toBeEnabled();
    expect(mocks.scan).toHaveBeenCalledTimes(1);
    await act(async () => { finish?.(completed(initial)); });
    expect(mocks.scan).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('button', { name: 'Return' })).toBeEnabled();
    await user.click(screen.getByRole('button', { name: 'Return' }));
    await waitFor(() => expect(mocks.confirm).toHaveBeenLastCalledWith(p.id, {
      returnConfirmed: true, expectedSaleId: 'sale-B',
      expectedTarget: { dhInventoryId: p.dhInventoryId!, certNumber: p.certNumber, grader: 'PSA' },
    }));
  });

  it('hides Return when a manual state refresh discovers a changed sale', async () => {
    const p = purchase();
    const initial = soldState(p);
    const mocks = transport(initial);
    seed(p, 'sold');
    render(<CardIntakeTab />);
    await screen.findByRole('button', { name: 'Return' });
    mocks.read.mockRejectedValueOnce(new Error('State read failed'));
    await userEvent.click(screen.getByRole('button', { name: 'Return' }));
    await screen.findByRole('button', { name: 'Refresh return state' });
    mocks.setState({ ...initial, sale: sale(p, 'sale-B'), expectedSaleId: 'sale-B' });
    await userEvent.click(screen.getByRole('button', { name: 'Refresh return state' }));
    expect(await screen.findByRole('button', { name: 'Dismiss to rescan' })).toBeVisible();
    expect(screen.queryByRole('button', { name: 'Return' })).not.toBeInTheDocument();
    expect(mocks.confirm).not.toHaveBeenCalled();
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
    await waitFor(() => expect(mocks.confirm).toHaveBeenCalledWith(p.id, {
      returnConfirmed: true, expectedSaleId: 'original-sale', operationId: 'return-operation',
      expectedTarget: { dhInventoryId: p.dhInventoryId!, certNumber: p.certNumber, grader: 'PSA' },
    }));
    expect(mocks.list).not.toHaveBeenCalled();
  });

  it('retries only the recorded pending operation while its own attempt is open', async () => {
    const p = purchase();
    const pending = completed({ ...state(p), expectedSaleId: 'sale-A' });
    pending.operation = { ...pending.operation!, state: 'pending', completedAt: undefined };
    pending.awaitingListing = false;
    pending.precedingAttempt = { id: 'attempt-A', capturedPurchaseId: p.id, dhInventoryId: p.dhInventoryId!,
      certNumber: p.certNumber, grader: 'PSA', kind: 'return', phase: 'dispatch', operationId: 'return-operation',
      startedAt: stamp, outcome: 'open' };
    const mocks = transport(pending);
    seed(p);
    render(<CardIntakeTab />);
    await userEvent.click(await screen.findByRole('button', { name: 'Retry return' }));
    await waitFor(() => expect(mocks.confirm).toHaveBeenCalledWith(p.id, {
      returnConfirmed: true, expectedSaleId: 'sale-A', operationId: 'return-operation',
      expectedTarget: { dhInventoryId: p.dhInventoryId!, certNumber: p.certNumber, grader: 'PSA' },
    }));
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
    await screen.findByText(/Returned; review the price and list explicitly/i);
    expect(mocks.confirm).toHaveBeenCalledTimes(1);
    expect(screen.queryByText('Response lost')).not.toBeInTheDocument();
    expect(mocks.list).not.toHaveBeenCalled();
  });

  it('does not issue an unkeyed second POST when a lost response has no durable resolution yet', async () => {
    const p = purchase();
    const mocks = transport(soldState(p));
    mocks.confirm.mockRejectedValue(new Error('Response lost'));
    seed(p, 'sold');
    render(<CardIntakeTab />);
    await userEvent.click(await screen.findByRole('button', { name: 'Return' }));
    await screen.findByText('Response lost');
    expect(screen.queryByRole('button', { name: 'Return' })).not.toBeInTheDocument();
    expect(mocks.confirm).toHaveBeenCalledTimes(1);
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
    // Let the replacement render and hydrate before releasing the old-owner
    // read, matching network latency rather than React's same-tick batching.
    await waitFor(() => expect(mocks.read).toHaveBeenCalledWith(newPurchase.id));
    releaseOld?.(initial);
    const list = await screen.findByRole('button', { name: 'List on DH' });
    await waitFor(() => expect(list).toBeEnabled());
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
    await screen.findByText('Sale precondition failed; rescan before retrying.');
    expect(screen.getByText('⚠ Sold')).toBeInTheDocument();
    expect(mocks.list).not.toHaveBeenCalled();
  });

  it('checks DH only after detail expansion and explicit click, then resolves with captured null sale and target', async () => {
    const p = purchase();
    const mocks = transport(state(p));
    seed(p);
    render(<CardIntakeTab />);
    await screen.findByRole('button', { name: 'List on DH' });
    expect(screen.queryByRole('button', { name: 'Return' })).not.toBeInTheDocument();
    expect(mocks.check).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', { name: 'Show card details' }));
    expect(mocks.check).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', { name: 'Check DH sale' }));
    await screen.findByRole('button', { name: 'Resolve DH sale' });
    expect(screen.getByText('Resolve confirms slab in hand and refund resolved')).toBeVisible();
    await userEvent.click(screen.getByRole('button', { name: 'Resolve DH sale' }));
    await waitFor(() => expect(mocks.confirm).toHaveBeenCalledWith(p.id, {
      returnConfirmed: true, expectedSaleId: null,
      expectedTarget: { dhInventoryId: p.dhInventoryId!, certNumber: p.certNumber, grader: 'PSA' },
    }));
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
    expect(mocks.list).not.toHaveBeenCalled();
  });

  it('does not offer Check DH sale for a purchase with no DH inventory link', async () => {
    const p = { ...purchase(), dhInventoryId: 0 };
    const mocks = transport(state(p));
    seed(p);
    render(<CardIntakeTab />);
    await screen.findByRole('button', { name: 'Show card details' });
    await userEvent.click(screen.getByRole('button', { name: 'Show card details' }));
    expect(screen.queryByRole('button', { name: 'Check DH sale' })).not.toBeInTheDocument();
    expect(mocks.check).not.toHaveBeenCalled();
  });

  it('does not offer a second unkeyed Resolve after a lost response with no durable episode', async () => {
    const p = purchase();
    const mocks = transport(state(p));
    mocks.confirm.mockRejectedValue(new Error('Response lost'));
    seed(p);
    render(<CardIntakeTab />);
    await screen.findByRole('button', { name: 'Show card details' });
    await userEvent.click(screen.getByRole('button', { name: 'Show card details' }));
    await userEvent.click(screen.getByRole('button', { name: 'Check DH sale' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Resolve DH sale' }));
    await screen.findByText('Response lost');
    expect(mocks.read).toHaveBeenCalledTimes(3); // hydration, pre-POST guard, lost-response recovery
    expect(mocks.confirm).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('button', { name: 'Resolve DH sale' })).not.toBeInTheDocument();
    // A second read-only observation does not clear the uncertain mutation fence.
    await userEvent.click(screen.getByRole('button', { name: 'Check DH sale' }));
    await waitFor(() => expect(mocks.check).toHaveBeenCalledTimes(2));
    expect(screen.queryByRole('button', { name: 'Resolve DH sale' })).not.toBeInTheDocument();
    expect(mocks.confirm).toHaveBeenCalledTimes(1);
  });

  it.each([
    { label: 'in_stock', check: { status: 'in_stock', resolvable: false, reason: 'not_sold' } },
    { label: 'open attempt', check: { status: '', resolvable: false, reason: 'mutation_pending' } },
    { label: 'missing target', check: { status: '', resolvable: false, reason: 'target_unavailable' } },
    { label: 'completed null', check: { status: '', resolvable: false, reason: 'return_episode_exists' } },
    { label: 'completed sale', check: { status: '', resolvable: false, reason: 'return_episode_exists' } },
  ])('does not offer Resolve for $label', async ({ check, label }) => {
    const p = purchase();
    const initial = state(p);
    if (check.reason === 'mutation_pending') initial.precedingAttempt = {
      id: 'list-open', capturedPurchaseId: p.id, dhInventoryId: p.dhInventoryId!, certNumber: p.certNumber,
      grader: 'PSA', kind: 'list', phase: 'intake_patch_sync', startedAt: stamp, outcome: 'open',
    };
    if (check.reason === 'return_episode_exists') {
      const historical = completed({ ...initial, expectedSaleId: label === 'completed sale' ? 'sale-A' : null });
      initial.operation = historical.operation;
      initial.awaitingListing = false;
    }
    const mocks = transport(initial);
    mocks.check.mockResolvedValue({ ...check, target: { dhInventoryId: p.dhInventoryId!, certNumber: p.certNumber, grader: 'PSA' } });
    seed(p);
    render(<CardIntakeTab />);
    await screen.findByRole('button', { name: 'Show card details' });
    await userEvent.click(screen.getByRole('button', { name: 'Show card details' }));
    await userEvent.click(screen.getByRole('button', { name: 'Check DH sale' }));
    await screen.findByText(new RegExp(check.reason));
    expect(screen.queryByRole('button', { name: 'Resolve DH sale' })).not.toBeInTheDocument();
    expect(mocks.confirm).not.toHaveBeenCalled();
  });

  it.each(['open attempt', 'completed episode'])('does not trust a stale-positive DH check against an %s', async hold => {
    const p = purchase();
    const initial = state(p);
    if (hold === 'open attempt') initial.precedingAttempt = {
      id: 'list-open', capturedPurchaseId: p.id, dhInventoryId: p.dhInventoryId!,
      certNumber: p.certNumber, grader: 'PSA', kind: 'list', phase: 'patch', startedAt: stamp, outcome: 'open',
    };
    else initial.operation = completed(initial).operation;
    const mocks = transport(initial); // Deliberately returns sold/resolvable: true despite the hold.
    seed(p);
    render(<CardIntakeTab />);
    await screen.findByRole('button', { name: 'Show card details' });
    await userEvent.click(screen.getByRole('button', { name: 'Show card details' }));
    await userEvent.click(screen.getByRole('button', { name: 'Check DH sale' }));
    await screen.findByText('DH sale confirmed for this slab.');
    expect(screen.queryByRole('button', { name: 'Resolve DH sale' })).not.toBeInTheDocument();
    expect(mocks.confirm).not.toHaveBeenCalled();
  });

  it('does not enable Resolve after a failed DH read', async () => {
    const p = purchase();
    const mocks = transport(state(p));
    mocks.check.mockRejectedValue(new Error('DH unavailable'));
    seed(p);
    render(<CardIntakeTab />);
    await screen.findByRole('button', { name: 'Show card details' });
    await userEvent.click(screen.getByRole('button', { name: 'Show card details' }));
    await userEvent.click(screen.getByRole('button', { name: 'Check DH sale' }));
    await screen.findByText(/DH sale check failed: DH unavailable/);
    expect(screen.queryByRole('button', { name: 'Resolve DH sale' })).not.toBeInTheDocument();
  });

  it('discards a delayed DH check when the cert is rescanned as another purchase', async () => {
    const p = purchase();
    const replacement = { ...p, id: 'replacement-purchase', dhInventoryId: 43 };
    const mocks = transport(state(p));
    let finish: ((value: Awaited<ReturnType<typeof api.getDHSaleCheck>>) => void) | undefined;
    mocks.check.mockImplementation(() => new Promise(resolve => { finish = resolve; }));
    mocks.read.mockImplementation(async id => state(id === p.id ? p : replacement));
    mocks.scan.mockResolvedValue({ status: 'existing', purchaseId: replacement.id, campaignId: p.campaignId,
      cardName: p.cardName, dhInventoryId: 43, dhStatus: 'in_stock' });
    seed(p);
    render(<CardIntakeTab />);
    await screen.findByRole('button', { name: 'Show card details' });
    await userEvent.click(screen.getByRole('button', { name: 'Show card details' }));
    await userEvent.click(screen.getByRole('button', { name: 'Check DH sale' }));
    await userEvent.click(screen.getByRole('button', { name: 'Clear all' }));
    await userEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Clear all' }));
    const input = screen.getByPlaceholderText('Scan or type cert number…');
    fireEvent.change(input, { target: { value: p.certNumber } });
    fireEvent.keyDown(input, { key: 'Enter' });
    await waitFor(() => expect(mocks.read).toHaveBeenCalledWith(replacement.id));
    await act(async () => { finish?.({ status: 'sold', resolvable: true, reason: '',
      target: { dhInventoryId: p.dhInventoryId!, certNumber: p.certNumber, grader: 'PSA' } }); });
    await userEvent.click(screen.getByRole('button', { name: 'Show card details' }));
    expect(screen.queryByRole('button', { name: 'Resolve DH sale' })).not.toBeInTheDocument();
    expect(mocks.confirm).not.toHaveBeenCalled();
  });

  it.each(['sale', 'target', 'completed episode'])('blocks Resolve when fresh state gains a %s', async change => {
    const p = purchase();
    const initial = state(p);
    const mocks = transport(initial);
    seed(p);
    render(<CardIntakeTab />);
    await screen.findByRole('button', { name: 'Show card details' });
    await userEvent.click(screen.getByRole('button', { name: 'Show card details' }));
    await userEvent.click(screen.getByRole('button', { name: 'Check DH sale' }));
    const button = await screen.findByRole('button', { name: 'Resolve DH sale' });
    mocks.setState(change === 'sale' ? soldState(p) : change === 'target'
      ? { ...initial, purchase: { ...p, dhInventoryId: 43 } }
      : { ...initial, operation: completed(initial).operation });
    await userEvent.click(button);
    await screen.findByText(/Return state changed.*rescan/i);
    expect(mocks.confirm).not.toHaveBeenCalled();
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
    await waitFor(() => expect(mocks.confirm).toHaveBeenCalledWith(p.id, { returnConfirmed: true, expectedSaleId: 'new-sale',
      expectedTarget: { dhInventoryId: p.dhInventoryId!, certNumber: p.certNumber, grader: 'PSA' } }));
  });
});
