import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query';
import { Link, useLocation } from 'react-router-dom';
import PriceHintDialog from './PriceHintDialog';
import App from './App';
import { getShowRefreshCoordinator } from './queries/showRefreshCoordinator';
import { ready } from './queries/showReadiness.test-support';

const probe = vi.hoisted(() => ({ client: undefined as QueryClient | undefined, fail: false, dialog: false }));
function Page() {
  const qc = useQueryClient(); probe.client = qc;
  const location = useLocation();
  const query = useQuery({ queryKey: ['ordinary-page'], queryFn: async () => (await fetch('/ordinary')).json() });
  if (probe.fail) throw new Error('route failed');
  return <><Link to="/inventory">Inventory fixture</Link><Link to="/shows">Shows fixture</Link>
    <span>{query.data?.value}</span>
    {probe.dialog && location.pathname === '/inventory' && <PriceHintDialog cardName="Slab" setName="Set" cardNumber="1" onClose={() => {}} onSaved={() => {}} />}</>;
}
vi.mock('./pages/GlobalInventoryPage', () => ({ default: () => <Page /> }));
vi.mock('./pages/ShowPreparationPage', () => ({ default: () => <Page /> }));
vi.mock('./components/Header', () => ({ default: () => <Link to="/shows">Recover route</Link> }));
vi.mock('./components/KeyboardShortcuts', () => ({ default: () => null }));
vi.mock('./components/PageTransition', () => ({ default: ({ children }: { children: React.ReactNode }) => children }));
let identity = 1;
let ordinary = 'first read';
beforeEach(() => {
  identity = 1; ordinary = 'first read'; probe.fail = false; probe.dialog = false;
  window.history.replaceState(null, '', '/inventory');
  vi.stubGlobal('fetch', vi.fn(async (url: string, options?: RequestInit) => {
    if (url.endsWith('/auth/user')) return new Response(JSON.stringify({ id: identity, username: `user-${identity}`, is_admin: false }));
    if (url === '/ordinary') return new Response(JSON.stringify({ value: ordinary }));
    const ids: string[] = JSON.parse(String(options?.body)).purchaseIds;
    return new Response(JSON.stringify({ evaluations: ids.map(id => ready(Number(id.slice(3)), 'current')) }));
  }));
});
afterEach(() => { probe.client?.clear(); vi.unstubAllGlobals(); vi.restoreAllMocks(); });
async function navigate(name: string) {
  fireEvent.click(screen.getByRole('link', { name }));
  await screen.findByRole('link', { name: 'Inventory fixture' });
}
it('preserves automatic budget across actual App pathname navigation, but rereads ordinary pages', async () => {
  render(<App />); await screen.findByText('first read');
  const qc = probe.client!; const coordinator = getShowRefreshCoordinator(qc); const owner = Symbol(); coordinator.attach(owner);
  await act(async () => coordinator.check(Array.from({ length: 201 }, (_, i) => ready(i)), owner, true));
  expect(coordinator.getSnapshot().phase).toBe('budget');
  ordinary = 'new read'; await navigate('Shows fixture');
  await screen.findByText('new read');
  expect(probe.client).toBe(qc);
  const next = getShowRefreshCoordinator(probe.client!);
  await act(async () => next.check([ready(999)], owner, true));
  expect(next.getSnapshot()).toMatchObject({ phase: 'budget', done: 200 });
});
it('preserves Cancel and a pending write lease across navigation without starting another check', async () => {
  render(<App />); await screen.findByText('first read');
  const qc = probe.client!; const c = getShowRefreshCoordinator(qc); const owner = Symbol(); c.attach(owner);
  const fetcher = vi.mocked(fetch); fetcher.mockImplementationOnce(async () => new Response(new ReadableStream({ start(s) { s.enqueue(new TextEncoder().encode('{')); } })));
  let run!: Promise<void>;
  act(() => { run = c.check([ready(1)], owner, true); });
  await act(async () => { c.cancel(); await run; });
  let release!: () => void; let write!: Promise<void>;
  act(() => { write = c.write(() => new Promise<void>(resolve => { release = resolve; })); });
  await navigate('Shows fixture');
  const next = getShowRefreshCoordinator(probe.client!);
  expect(next.getSnapshot()).toMatchObject({ phase: 'cancelled', blocked: true });
  const nextOwner = Symbol(); next.attach(nextOwner);
  await act(async () => next.check([ready(2)], nextOwner, false));
  expect(next.getSnapshot().phase).toBe('cancelled');
  await act(async () => { release(); await write; });
  expect(next.getSnapshot().blocked).toBe(false);
  await act(async () => next.check([ready(2)], nextOwner, true));
  expect(next.getSnapshot().phase).toBe('cancelled');
});
it('never exposes a previous identity cache after route authentication returns another user', async () => {
  render(<App />); await screen.findByText('first read');
  const first = probe.client!; first.setQueryData(['private-user'], 'secret');
  identity = 2; ordinary = 'second user'; await navigate('Shows fixture');
  await screen.findByText('second user');
  expect(probe.client).not.toBe(first);
  expect(probe.client!.getQueryData(['private-user'])).toBeUndefined();
  await waitFor(() => expect(first.getQueryData(['private-user'])).toBeUndefined());
});
it('ignores authentication completing from an abandoned route', async () => {
  const fetcher = vi.mocked(fetch); const normal = fetcher.getMockImplementation()!;
  let release!: (response: Response) => void;
  fetcher.mockImplementationOnce(() => new Promise<Response>(resolve => { release = resolve; })).mockImplementation(normal);
  render(<App />);
  await waitFor(() => expect(release).toBeTypeOf('function'));
  identity = 2; fireEvent.click(screen.getByRole('link', { name: 'Recover route' }));
  await screen.findByText('first read');
  const current = probe.client!; current.setQueryData(['private-user'], 'user-two');
  await act(async () => release(new Response(JSON.stringify({ id: 1, username: 'user-one' }))));
  expect(probe.client).toBe(current);
  expect(probe.client!.getQueryData(['private-user'])).toBe('user-two');
});
it.each([200, 400])('holds an actual dialog write across App SPA navigation through %s body settlement', async status => {
  probe.dialog = true;
  const fetcher = vi.mocked(fetch); const normal = fetcher.getMockImplementation()!;
  let body!: ReadableStreamDefaultController<Uint8Array>;
  fetcher.mockImplementation((url, options) => String(url).endsWith('/price-hints')
    ? Promise.resolve(new Response(new ReadableStream({ start(s) { body = s; s.enqueue(new TextEncoder().encode('{')); } }), { status })) : normal(url, options));
  render(<App />); await screen.findByText('first read');
  fireEvent.change(screen.getByLabelText('External ID'), { target: { value: '1234' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save Hint' }));
  await waitFor(() => expect(body).toBeDefined());
  await act(async () => { window.history.pushState(null, '', '/shows'); window.dispatchEvent(new PopStateEvent('popstate')); });
  await screen.findByText('first read');
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  const c = getShowRefreshCoordinator(probe.client!);
  expect(c.getSnapshot().blocked).toBe(true);
  const owner = Symbol(); c.attach(owner);
  await act(async () => c.check([ready(2)], owner, true));
  expect(c.getSnapshot().phase).toBe('idle');
  await act(async () => { body.enqueue(new TextEncoder().encode(status === 200 ? '"status":"ok"}' : '"error":"rejected"}')); body.close(); });
  await waitFor(() => expect(c.getSnapshot().blocked).toBe(false));
});
it('still resets a route error on pathname navigation', async () => {
  const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
  probe.fail = true; render(<App />); await screen.findByText(/Something went wrong/i);
  probe.fail = false;
  await act(async () => { window.history.pushState(null, '', '/shows'); window.dispatchEvent(new PopStateEvent('popstate')); });
  await screen.findByRole('link', { name: 'Inventory fixture' });
  consoleError.mockRestore();
});
