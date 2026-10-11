import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { AuthProvider } from '../contexts/AuthContext';
import ShowPreparationPage from './ShowPreparationPage';
import { detail, evaluation, listId, member } from './show-preparation/fixtures.test-support';
import type { Availability, ShowListDetail } from '../../types/showprep';
let saved: ShowListDetail;
let calls: { url: string; method: string; body: Record<string, unknown> }[];
function mount(entry = `/shows?list=${listId}`) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={qc}><AuthProvider><MemoryRouter initialEntries={[entry]}><ShowPreparationPage /></MemoryRouter></AuthProvider></QueryClientProvider>);
}
beforeEach(() => {
  sessionStorage.clear(); saved = detail(); calls = [];
  vi.stubGlobal('fetch', vi.fn(async (url: string, options: RequestInit = {}) => {
    const method = options.method || 'GET'; const body = options.body ? JSON.parse(String(options.body)) : {}; calls.push({ url, method, body });
    if (url.endsWith('/auth/user')) return Response.json({ id: 7, username: 'Operator', email: 'operator@example.test', avatar_url: '', is_admin: false, last_login_at: null });
    let response: unknown = saved;
    if (url.endsWith('/lists')) {
      if (method === 'POST') { saved.list = { ...saved.list, id: body.id, name: body.name }; response = saved.list; }
      else response = { lists: [saved.list] };
    } else if (url.includes('/items/') && method === 'DELETE') { saved = { ...saved, items: [], list: { ...saved.list, withdrawalGeneration: saved.list.withdrawalGeneration + 1 } }; response = { removed: true }; }
    else if (method === 'PUT') { saved.list.name = body.name; response = saved.list; }
    else if (url.includes('/evidence/')) response = { evaluation: saved.items[0].evaluation, sales: [] };
    return Response.json(response);
  }));
});
afterEach(() => vi.unstubAllGlobals());
describe('scan-only saved show preparation', () => {
  it('shows legacy members as unconfirmed with no Pack or Acknowledge bypass', async () => {
    mount(); expect(await screen.findByText('Unconfirmed. Scan the checked slab to confirm.')).toBeVisible();
    expect(screen.queryByRole('checkbox', { name: /Packed/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Add.*show|Acknowledge/i })).not.toBeInTheDocument();
    expect(calls.every(call => call.method === 'GET')).toBe(true);
  });
  it.each<Availability>(['not_received', 'sold', 'refunded', 'campaign_closed', 'removed', 'unknown'])('retains %s history without counting unavailable members, and permits explicit removal', async availability => {
    saved = detail([member({ packedAt: '2026-09-14T11:00:00Z', evaluation: evaluation({ availability, canPack: false, canAdd: availability === 'not_received' }) })]);
    saved.summary.packedCount = 1; mount();
    expect(await screen.findByText(/Confirmed 2026-09-14/)).toBeVisible();
    expect(screen.getByText(/Confirmation history retained; not currently ready/)).toBeVisible();
    expect(screen.getByLabelText('Confirmed ready asking value')).toHaveTextContent('$0.00');
    fireEvent.click(screen.getByRole('button', { name: 'Remove 12345678 from show' }));
    await waitFor(() => expect(screen.queryByRole('article', { name: 'Show member 12345678' })).not.toBeInTheDocument());
    expect(calls.filter(call => call.method !== 'GET').map(call => call.method)).toEqual(['DELETE']);
  });
  it('warns about corrected identity without hiding confirmation history or mutating inventory', async () => {
    const corrected = { ...member({ packedAt: '2026-09-14T11:00:00Z', evaluation: evaluation({ grade: 9 }) }), identityChanged: true };
    saved = detail([corrected]);
    saved.summary = { ...saved.summary, packedCount: 1, confirmedReadyCount: 0, confirmedKnownValueCents: 0 };
    mount();
    expect(await screen.findByText(/Identity changed.*Confirmation history retained/)).toBeVisible();
    expect(screen.getByText(/Check inventory identity before scanning/)).toBeVisible();
    expect(screen.getByText(/Confirmed 2026-09-14/)).toBeVisible();
    expect(screen.getByLabelText('Confirmed ready asking value')).toHaveTextContent('$0.00');
    expect(calls.every(call => call.method === 'GET')).toBe(true);
    expect(screen.getByRole('button', { name: 'Remove 12345678 from show' })).toBeEnabled();
  });
  it('uses confirmed-ready totals, not historical packed or legacy known value', async () => {
    saved = detail([member({ packedAt: '2026-09-14T11:00:00Z', evaluation: evaluation({ priceAssociationUnclear: true }) })]);
    saved.summary = { ...saved.summary, packedCount: 4, totalCount: 9, knownValueCents: 90000, confirmedReadyCount: 1, confirmedKnownValueCents: 30000, missingPriceCount: 2, ambiguousPriceCount: 1 };
    mount(); expect(await screen.findByLabelText('Confirmed ready asking value')).toHaveTextContent('$300.00');
    expect(screen.getByLabelText('DH association warnings')).toHaveTextContent('1'); expect(screen.getByLabelText('Missing asking prices')).toHaveTextContent('2');
  });
  it('keeps sticker drift and policy-neutral original acknowledgments until a physical rescan', async () => {
    saved = detail([member({ priceChanged: true, supportChanged: true, acknowledgedStatus: 'no_listed_price', acknowledgedPriceCents: 0 })]);
    mount(); expect(await screen.findByText('Check sticker and rescan to confirm.')).toBeVisible();
    expect(screen.getByText('Support changed. Previously recorded status: no_listed_price.')).toBeVisible();
    expect(screen.getByText('Price changed. Previously acknowledged no price.')).toBeVisible();
    expect(calls.every(call => call.method === 'GET')).toBe(true);
  });
  it('rechecking reopened availability never automatically confirms legacy members', async () => {
    saved.items[0].evaluation = evaluation({ availability: 'campaign_closed', canPack: false, canAdd: false }); mount();
    await screen.findByText(/Confirmation history retained/); saved.items[0].evaluation = evaluation({ version: 'reopened' });
    fireEvent.click(screen.getByRole('button', { name: 'Update list status' }));
    await waitFor(() => expect(screen.queryByText(/Confirmation history retained/)).not.toBeInTheDocument());
    expect(screen.getByText(/Unconfirmed. Scan/)).toBeVisible(); expect(calls.every(call => call.method === 'GET')).toBe(true);
  });
  it('creates and renames named shows without a bulk approval path', async () => {
    mount(); await screen.findByText(/Unconfirmed. Scan/);
    fireEvent.click(screen.getByRole('button', { name: 'New list' }));
    fireEvent.change(screen.getByLabelText('New show name'), { target: { value: '  October show  ' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create show list' }));
    await waitFor(() => expect(screen.getByLabelText('Show list')).not.toHaveValue(listId)); expect(calls.find(call => call.method === 'POST')?.body.name).toBe('October show');
    fireEvent.click(await screen.findByRole('button', { name: 'Rename show list' }));
    fireEvent.change(screen.getByLabelText('Show name'), { target: { value: '  October packing  ' } }); fireEvent.click(screen.getByRole('button', { name: 'Save name' }));
    await waitFor(() => expect(within(screen.getByLabelText('Show list')).getByRole('option', { name: 'October packing' })).toBeInTheDocument());
  });
});
