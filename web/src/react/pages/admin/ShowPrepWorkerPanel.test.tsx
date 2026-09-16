import { afterEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ShowPrepWorkerPanel } from './ShowPrepWorkerPanel';

const initial = { enabled: true, configured: true, state: 'auth_hold', eligibleIdentities: 2, currentIdentities: 1,
  missingIdentities: 1, staleIdentities: 0, failedIdentities: 0, eligibleCards: 4, currentCards: 2, unresolvedCards: 1,
  lastSweepAt: '2026-09-15T12:00:00Z', retryAt: '', error: 'CardLadder authentication needs attention' };
afterEach(() => vi.unstubAllGlobals());

function mount(status = initial, post?: () => Promise<Response>) {
  const requests: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    requests.push(`${init?.method ?? 'GET'} ${url}`);
    if (init?.method === 'POST') return post ? post() : new Response(JSON.stringify({ status: 'accepted' }), { status: 202 });
    return new Response(JSON.stringify(status));
  }));
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const view = render(<QueryClientProvider client={qc}><ShowPrepWorkerPanel /></QueryClientProvider>);
  return { requests, close: () => { view.unmount(); qc.clear(); } };
}

it('shows explicit identity/card units and resumes an auth hold with zero failed identities', async () => {
  const view = mount();
  await screen.findByText('Authentication hold');
  expect(screen.getByText('1/2 identities current')).toBeVisible();
  expect(screen.getByText('2/4 cards with current evidence')).toBeVisible();
  expect(screen.getByText('1 unresolved card')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Retry failed' })).toBeEnabled();
  fireEvent.click(screen.getByRole('button', { name: 'Retry failed' }));
  await screen.findByText('Request accepted. Background coverage will update separately.');
  expect(view.requests).toContain('POST /api/admin/show-prep/worker/retry');
  expect(screen.queryByText(/refresh complete|all evidence ready/i)).not.toBeInTheDocument();
  view.close();
});

it('locks both actions while enqueue is pending, not while waiting for fleet completion', async () => {
  let accept!: (value: Response) => void;
  const pending = new Promise<Response>(resolve => { accept = resolve; });
  const view = mount(initial, () => pending);
  await screen.findByText('Authentication hold');
  fireEvent.click(screen.getByRole('button', { name: 'Run now' }));
  await waitFor(() => expect(screen.getByRole('button', { name: /Requesting/ })).toBeDisabled());
  expect(screen.getByRole('button', { name: 'Retry failed' })).toBeDisabled();
  accept(new Response(JSON.stringify({ status: 'accepted' }), { status: 202 }));
  await screen.findByText('Request accepted. Background coverage will update separately.');
  expect(screen.getByRole('button', { name: 'Run now' })).toBeEnabled();
  expect(view.requests.filter(r => r.startsWith('POST'))).toEqual(['POST /api/admin/show-prep/worker/run']);
  view.close();
});

it.each([
  [{ ...initial, enabled: false, state: 'disabled' }, 'Disabled'],
  [{ ...initial, configured: false, state: 'unconfigured' }, 'Unconfigured'],
  [{ ...initial, state: 'failed', failedIdentities: 1 }, 'Completed with errors'],
  [{ ...initial, state: 'idle', failedIdentities: 0 }, 'Idle, coverage incomplete'],
  [{ ...initial, state: 'running' }, 'Running'],
  [{ ...initial, state: 'idle', currentIdentities: 2, missingIdentities: 0, currentCards: 4, unresolvedCards: 0 }, 'Current coverage'],
  [{ ...initial, state: 'idle', currentIdentities: 0, eligibleIdentities: 0, missingIdentities: 0, currentCards: 0, eligibleCards: 0, unresolvedCards: 0 }, 'No inventory'],
])('reports %s honestly', async (status, label) => {
  const view = mount(status);
  await screen.findByText(label);
  if (!status.enabled || !status.configured) {
    expect(screen.getByRole('button', { name: 'Run now' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Retry failed' })).toBeDisabled();
  }
  expect(view.requests.every(r => r.startsWith('GET'))).toBe(true);
  view.close();
});

it('does not report current coverage for a malformed status response', async () => {
  const view = mount({} as typeof initial);
  await screen.findByText('Evidence worker status unavailable. Coverage below may be out of date.');
  expect(screen.queryByText('Current coverage')).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Run now' })).toBeDisabled();
  view.close();
});

it('reports rejected intent without silently retrying an epoch reset', async () => {
  const view = mount(initial, async () => new Response(JSON.stringify({ error: 'private detail' }), { status: 503 }));
  await screen.findByText('Authentication hold');
  fireEvent.click(screen.getByRole('button', { name: 'Retry failed' }));
  await screen.findByText('Request was not accepted. Try again after checking service availability.');
  expect(view.requests.filter(r => r.startsWith('POST'))).toHaveLength(1);
  expect(screen.queryByText(/private detail/)).not.toBeInTheDocument();
  view.close();
});
