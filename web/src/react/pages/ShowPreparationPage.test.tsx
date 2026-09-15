import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import ShowPreparationPage from './ShowPreparationPage';
import { detail, evaluation, listId, member } from './show-preparation/fixtures.test-support';
import type { Availability, ShowListDetail } from '../../types/showprep';

let saved: ShowListDetail;
let calls: { url: string; method: string; body: Record<string, unknown> }[];
let conflict: boolean;
function mount(entry = `/shows?list=${listId}`) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={qc}><MemoryRouter initialEntries={[entry]}><ShowPreparationPage /></MemoryRouter></QueryClientProvider>);
}
beforeEach(() => {
  saved = detail(); calls = []; conflict = false;
  vi.stubGlobal('fetch', vi.fn().mockImplementation(async (url: string, options: RequestInit = {}) => {
    const method = options.method || 'GET';
    const body = options.body ? JSON.parse(String(options.body)) : {};
    calls.push({ url, method, body });
    let response: unknown = saved; let status = 200;
    if (url.endsWith('/lists')) {
      if (method === 'POST') { saved.list = { ...saved.list, id: body.id, name: body.name }; response = saved.list; }
      else response = { lists: [saved.list] };
    } else if (url.includes('/items/')) {
      if (method === 'DELETE') { saved.items = []; saved.summary.totalCount = 0; response = { removed: true }; }
      else if (conflict) { response = { error: 'Stale item. Review current values.' }; status = 409; }
      else {
        if ('packed' in body) saved.items[0].packedAt = body.packed ? '2026-09-14T11:00:00Z' : '';
        if (body.acknowledge || body.packed) { saved.items[0].priceChanged = false; saved.items[0].supportChanged = false; }
        saved.items[0].version++;
        saved.summary.packedCount = saved.items[0].packedAt ? 1 : 0;
      }
    } else if (method === 'PUT') { saved.list.name = body.name; response = saved.list; }
    else if (url.includes('/evidence/')) response = { evaluation: saved.items[0].evaluation, sales: [] };
    return new Response(JSON.stringify(response), { status });
  }));
});
afterEach(() => vi.unstubAllGlobals());

describe('saved show preparation', () => {
  it('starts an empty Shows page with one creation path, then opens the new list', async () => {
    let created = false;
    vi.stubGlobal('fetch', vi.fn(async (url: string, options: RequestInit = {}) => {
      if (url.endsWith('/lists') && options.method === 'POST') {
        saved.list = { ...saved.list, ...JSON.parse(String(options.body)) }; created = true;
        return new Response(JSON.stringify(saved.list));
      }
      return new Response(JSON.stringify(url.endsWith('/lists') ? { lists: created ? [saved.list] : [] } : { ...saved, items: [] }));
    }));
    mount('/shows');
    expect(await screen.findByRole('heading', { name: 'No show lists yet' })).toBeVisible();
    expect(screen.queryByLabelText('Show list')).not.toBeInTheDocument();
    expect(screen.queryByText(/Choose a saved list or create one/)).not.toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('New show name'), { target: { value: 'September packing' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create show list' }));
    expect(await screen.findByRole('link', { name: 'Select cards from inventory →' })).toHaveAttribute('href', '/inventory');
    expect(screen.getByLabelText('Show list')).toHaveValue(saved.list.id);
    expect(screen.queryByLabelText('New show name')).not.toBeInTheDocument();
  });
  it('persists explicit packing and reloads from server; mutations contain displayed versions', async () => {
    const first = mount();
    const packed = await screen.findByRole('checkbox', { name: 'Packed 12345678' });
    expect(packed).not.toBeChecked();
    fireEvent.click(packed);
    await waitFor(() => expect(packed).toBeChecked());
    expect(calls.find(c => c.method === 'PUT')?.body).toEqual({ version: 1, evaluationVersion: 'eval-1', packed: true });
    first.unmount(); mount();
    expect(await screen.findByRole('checkbox', { name: 'Packed 12345678' })).toBeChecked();
    expect(screen.queryByRole('button', { name: /sell|reprice|delist|record sale/i })).not.toBeInTheDocument();
  });

  it.each<Availability>(['not_received', 'sold', 'refunded', 'campaign_closed', 'removed', 'unknown'])('retains %s members and packing history; unpack/remove stay available', async availability => {
    saved = detail([member({ packedAt: '2026-09-14T11:00:00Z', evaluation: evaluation({ availability, canPack: false, canAdd: availability === 'not_received' }) })]);
    saved.summary = { ...saved.summary, packedCount: 1, knownValueCents: 0, unavailableCount: availability === 'not_received' ? 0 : 1, notReceivedCount: availability === 'not_received' ? 1 : 0 };
    mount();
    const packed = await screen.findByRole('checkbox', { name: 'Packed 12345678' });
    expect(packed).toBeChecked(); expect(packed).toBeEnabled();
    fireEvent.click(packed);
    await waitFor(() => expect(packed).not.toBeChecked());
    expect(packed).toBeDisabled();
    expect(screen.getByLabelText('Known ready-to-pack listed value')).toHaveTextContent('$0.00');
    fireEvent.click(screen.getByRole('button', { name: 'Remove 12345678 from show' }));
    await waitFor(() => expect(screen.queryByRole('checkbox', { name: 'Packed 12345678' })).not.toBeInTheDocument());
  });

  it('shows missing/ambiguous totals separately and keeps ambiguous-but-ready cards packable', async () => {
    saved = detail([
      member({ evaluation: evaluation({ status: 'needs_review', priceAssociationUnclear: true, reason: 'DH price association unclear' }) }),
      ...['44444444-4444-4444-8444-444444444444', '55555555-5555-4555-8555-555555555555'].map((id, index) => member({
        id, purchaseId: id, certNumber: `missing-${index}`, evaluation: evaluation({ purchaseId: id, certNumber: `missing-${index}`, status: 'no_listed_price', listedPriceCents: 0 }),
      })),
    ]);
    saved.summary = { ...saved.summary, knownValueCents: 0, ambiguousPriceCount: 1, missingPriceCount: 2, totalCount: 3 };
    mount();
    expect(await screen.findByRole('checkbox', { name: 'Packed 12345678' })).toBeEnabled();
    expect(screen.getByLabelText('Known ready-to-pack listed value')).toHaveTextContent('$0.00');
    expect(screen.getByLabelText('Ambiguous DH prices')).toHaveTextContent('1');
    expect(screen.getByLabelText('Missing DH prices')).toHaveTextContent('2');
    expect(screen.getByText(/DH price association unclear; excluded/)).toBeVisible();
  });

  it('flags changed price/support after packing and acknowledges only observed versions', async () => {
    saved = detail([member({ packedAt: '2026-09-14T11:00:00Z', version: 3, priceChanged: true, supportChanged: true,
      evaluation: evaluation({ listedPriceCents: 35000, status: 'below_target', version: 'eval-2' }) })]);
    mount();
    expect(await screen.findByText(/Price changed.*check the physical sticker/i)).toBeVisible();
    expect(screen.getByText(/Support changed/)).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Acknowledge changes 12345678' }));
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Acknowledge changes 12345678' })).not.toBeInTheDocument());
    expect(calls.find(c => c.method === 'PUT')?.body).toEqual({ version: 3, evaluationVersion: 'eval-2', acknowledge: true });
    expect(screen.getByRole('checkbox', { name: 'Packed 12345678' })).toBeChecked();
  });

  it('does not claim packing success on 409 and refetches current availability', async () => {
    conflict = true; mount();
    const packed = await screen.findByRole('checkbox', { name: 'Packed 12345678' });
    saved.items[0].evaluation = evaluation({ availability: 'refunded', canPack: false, canAdd: false, version: 'eval-2' });
    fireEvent.click(packed);
    expect(await screen.findByRole('alert')).toHaveTextContent(/Stale item/);
    await waitFor(() => expect(packed).toBeDisabled());
    expect(packed).not.toBeChecked();
    expect(screen.getByText('Unavailable: refunded')).toBeVisible();
  });

  it('reevaluates a reopened campaign without automatically repacking the retained member', async () => {
    saved.items[0].evaluation = evaluation({ availability: 'campaign_closed', canPack: false, canAdd: false });
    mount();
    const packed = await screen.findByRole('checkbox', { name: 'Packed 12345678' });
    expect(packed).toBeDisabled(); expect(packed).not.toBeChecked();
    saved.items[0].evaluation = evaluation({ version: 'reopened-version' });
    fireEvent.click(screen.getByRole('button', { name: 'Update list status' }));
    await waitFor(() => expect(packed).toBeEnabled());
    expect(packed).not.toBeChecked();
    expect(calls.some(c => c.method === 'PUT')).toBe(false);
  });

  it('creates named lists, selects them, and renames with trimmed names', async () => {
    mount(); await screen.findByRole('checkbox', { name: 'Packed 12345678' });
    fireEvent.click(screen.getByRole('button', { name: 'New list' }));
    fireEvent.change(screen.getByLabelText('New show name'), { target: { value: '  October show  ' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create show list' }));
    await waitFor(() => expect(screen.getByLabelText('Show list')).not.toHaveValue(listId));
    const creation = calls.find(c => c.method === 'POST');
    expect(creation?.body.name).toBe('October show');
    expect(creation?.body.id).toMatch(/^[a-f0-9-]{36}$/);
    fireEvent.click(await screen.findByRole('button', { name: 'Rename show list' }));
    fireEvent.change(screen.getByLabelText('Show name'), { target: { value: '  October packing  ' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save name' }));
    await waitFor(() => expect(within(screen.getByLabelText('Show list')).getByRole('option', { name: 'October packing' })).toBeInTheDocument());
  });
});
