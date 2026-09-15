import { afterEach, expect, it, vi } from 'vitest';
import { act, renderHook, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { api, APIError } from '../../../../js/api';
import { ToastProvider } from '../../../contexts/ToastContext';
import { getShowRefreshCoordinator } from '../../../queries/showRefreshCoordinator';
import { queryKeys } from '../../../queries/queryKeys';
import { ready } from '../../../queries/showReadiness.test-support';
import { useInventoryState } from './useInventoryState';

const items: Parameters<typeof useInventoryState>[0] = [];
function setup() {
  vi.stubGlobal('scrollTo', vi.fn());
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(queryKeys.portfolio.globalInventory, { items: [] });
  const hook = renderHook(() => useInventoryState(items), {
    wrapper: ({ children }) => <QueryClientProvider client={qc}><ToastProvider>{children}</ToastProvider></QueryClientProvider>,
  });
  return { ...hook, qc, coordinator: getShowRefreshCoordinator(qc) };
}
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

it('reports one inline-price API failure, preserves its rejection and holds the write lease through the error body', async () => {
  let body!: ReadableStreamDefaultController<Uint8Array>;
  const fetch = vi.fn(async () => new Response(new ReadableStream({ start(controller) { body = controller; } }), { status: 400 }));
  vi.stubGlobal('fetch', fetch);
  const save = vi.spyOn(api, 'setReviewedPrice'); // Call through the real API client; observe the original rejection.
  const { result, coordinator, qc, unmount } = setup();
  let outcome!: Promise<unknown>;
  act(() => { outcome = result.current.handleInlinePriceSave('purchase-1', 30000).catch(error => error); });
  await waitFor(() => expect(fetch).toHaveBeenCalledOnce());
  expect(coordinator.getSnapshot().blocked).toBe(true);
  expect(screen.queryByRole('button', { name: 'Close notification' })).not.toBeInTheDocument();
  await act(async () => { body.enqueue(new TextEncoder().encode('{"error":"Price rejected","code":"PRICE_CONFLICT"}')); body.close(); await outcome; });
  const rejection = await outcome;
  expect(rejection).toBe(await save.mock.results[0].value.catch((error: unknown) => error));
  expect(rejection).toBeInstanceOf(APIError);
  expect(rejection).toMatchObject({ status: 400, code: 'PRICE_CONFLICT', message: 'Price rejected' });
  expect(screen.getAllByRole('button', { name: 'Close notification' })).toHaveLength(1);
  expect(screen.getByText('Price rejected')).toBeInTheDocument();
  expect(coordinator.getSnapshot().blocked).toBe(false);
  expect(qc.getQueryState(queryKeys.portfolio.globalInventory)?.isInvalidated).toBe(false);
  unmount(); qc.clear();
});

it('saves the original cents payload, notifies once and invalidates inventory after success', async () => {
  const fetch = vi.fn(async () => Response.json({ success: true, reviewedAt: '2026-09-15T12:00:00Z' }));
  vi.stubGlobal('fetch', fetch);
  const { result, coordinator, qc, unmount } = setup();
  await act(async () => { await expect(result.current.handleInlinePriceSave('purchase-1', 30000)).resolves.toBeUndefined(); });
  expect(fetch).toHaveBeenCalledWith('/api/purchases/purchase-1/review-price', expect.objectContaining({
    method: 'PATCH', body: JSON.stringify({ priceCents: 30000, source: 'manual' }),
  }));
  expect(screen.getAllByRole('button', { name: 'Close notification' })).toHaveLength(1);
  expect(screen.getByText('Price saved')).toBeInTheDocument();
  expect(qc.getQueryState(queryKeys.portfolio.globalInventory)?.isInvalidated).toBe(true);
  expect(coordinator.getSnapshot().blocked).toBe(false);
  unmount(); qc.clear();
});

it.each(['inline price', 'resolve flag'])('reports one coordinator refusal without dispatching %s', async action => {
  const fetch = vi.fn(async () => new Response(new ReadableStream()));
  vi.stubGlobal('fetch', fetch);
  const { result, coordinator, qc, unmount } = setup();
  const owner = Symbol('active check'); coordinator.attach(owner);
  let checking!: Promise<void>;
  act(() => { checking = coordinator.check([ready(1)], owner, false); });
  await waitFor(() => expect(fetch).toHaveBeenCalledOnce());
  expect(coordinator.getSnapshot().busy).toBe(true);
  const refusal = 'Comps are being checked. Cancel or wait before changing this data.';
  await act(async () => {
    if (action === 'inline price') await expect(result.current.handleInlinePriceSave('purchase-1', 30000)).rejects.toThrow(refusal);
    else await expect(result.current.handleResolveFlag(12)).resolves.toBeUndefined();
  });
  expect(fetch).toHaveBeenCalledOnce();
  expect(screen.getAllByRole('button', { name: 'Close notification' })).toHaveLength(1);
  expect(screen.getByText(refusal)).toBeInTheDocument();
  expect(coordinator.getSnapshot().blocked).toBe(false);
  await act(async () => { coordinator.cancel(); await checking; });
  coordinator.detach(owner); unmount(); qc.clear();
});

it('keeps a self-reported resolve-flag API failure non-rejecting with one notification', async () => {
  vi.stubGlobal('fetch', async () => Response.json({ error: 'Flag rejected' }, { status: 400 }));
  const { result, coordinator, qc, unmount } = setup();
  await act(async () => { await expect(result.current.handleResolveFlag(12)).resolves.toBeUndefined(); });
  expect(screen.getAllByRole('button', { name: 'Close notification' })).toHaveLength(1);
  expect(screen.getByText('Flag rejected')).toBeInTheDocument();
  expect(coordinator.getSnapshot().blocked).toBe(false);
  unmount(); qc.clear();
});
