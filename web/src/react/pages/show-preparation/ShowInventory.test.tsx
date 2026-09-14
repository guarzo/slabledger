import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { ToastProvider } from '../../contexts/ToastContext';
import InventoryTab from '../campaign-detail/InventoryTab';
import { showPrepKeys } from '../../queries/useShowPrepQueries';
import type { InventoryEvaluations } from '../../../js/api/showprep';
import { detail, evaluation, inventoryItem, listId, purchaseId } from './fixtures.test-support';

// jsdom has no layout; keep real rows and interactions, replace only measurement.
vi.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getTotalSize: () => count * 160,
    getVirtualItems: () => Array.from({ length: count }, (_, index) => ({ index, start: index * 160 })),
    measureElement: () => {}, measure: () => {},
  }),
}));
const secondId = '44444444-4444-4444-8444-444444444444';
const thirdId = '55555555-5555-4555-8555-555555555555';
const values = [evaluation(), evaluation({ purchaseId: secondId, certNumber: '87654321', status: 'needs_review', reason: 'Source unavailable' }),
  evaluation({ purchaseId: thirdId, cardName: 'Charizard', certNumber: '99999999', availability: 'not_received', canPack: false })];
const items = values.map((e, i) => inventoryItem(e, i === 2 ? { receivedAt: '', dhStatus: '' } : {}));
let requests: { url: string; body: Record<string, unknown> }[];
let addFails: boolean;
function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={qc}><MemoryRouter><ToastProvider><InventoryTab items={items} isLoading={false} /></ToastProvider></MemoryRouter></QueryClientProvider>);
  return qc;
}
beforeEach(() => {
  requests = []; addFails = false;
  vi.stubGlobal('scrollTo', vi.fn());
  vi.stubGlobal('fetch', vi.fn().mockImplementation(async (url: string, options: RequestInit = {}) => {
    const body = options.body ? JSON.parse(String(options.body)) : {};
    requests.push({ url, body });
    let response: unknown = {};
    let status = 200;
    if (url.endsWith('/evaluate')) response = { evaluations: values };
    else if (url.endsWith('/lists')) response = { lists: [detail().list] };
    else if (url.endsWith('/items')) { response = addFails ? { error: 'Evaluation changed. Review current data.' } : detail(); status = addFails ? 409 : 200; }
    else if (url.includes('/evidence/')) response = { evaluation: values[0], sales: [
      { id: 'sale1', date: '2026-09-13', priceCents: 27000, platform: 'eBay', url: 'https://example.com/sale', listingType: 'Auction' },
      { id: 'sale2', date: '2026-09-12', priceCents: 29000, platform: 'eBay', url: 'javascript:alert(1)', listingType: 'Fixed price' },
    ] };
    return new Response(JSON.stringify(response), { status });
  }));
});
afterEach(() => vi.unstubAllGlobals());

describe('show preparation in the existing inventory', () => {
  it('keeps failed or missing evaluations distinct from no comps, with an explicit retry', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ evaluations: [values[0]] }))));
    mount();
    expect(await screen.findByRole('alert')).toHaveTextContent('2 evaluations unavailable');
    fireEvent.click(screen.getByRole('button', { name: /^All\d/ }));
    expect(screen.queryByText('No recent comps', { selector: 'strong' })).not.toBeInTheDocument();
    expect(screen.getAllByText(/Evaluation unavailable:/)).toHaveLength(2);
    fireEvent.click(screen.getByRole('button', { name: 'Show selection' }));
    expect(screen.getByText('1 card shown')).toBeVisible();
    expect(screen.getByRole('button', { name: 'Retry evaluation' })).toBeEnabled();
  });

  it('intersects support with active search and tab even outside show-selection mode', async () => {
    mount();
    await waitFor(() => expect(screen.getAllByText('Supported', { selector: 'strong' }).length).toBeGreaterThan(0));
    fireEvent.change(screen.getByLabelText('Price support'), { target: { value: 'supported' } });
    fireEvent.click(screen.getByRole('button', { name: /^DH Listed/ }));
    fireEvent.change(screen.getByLabelText('Search cards'), { target: { value: 'Charizard' } });
    await waitFor(() => expect(screen.getByText('0 cards shown')).toBeVisible());
    expect(screen.getByRole('checkbox', { name: 'Select all visible cards' })).not.toBeChecked();
    fireEvent.click(screen.getByRole('button', { name: /^All\d/ }));
    expect(screen.getByText('1 card shown')).toBeVisible();
    expect(screen.getByRole('checkbox', { name: 'Select 99999999' })).toBeInTheDocument();
  });

  it('intersects support, search, tab and availability; select-all adds only displayed slabs and preserves selection on filter changes', async () => {
    mount();
    fireEvent.click(await screen.findByRole('button', { name: 'Show selection' }));
    fireEvent.click(screen.getByRole('button', { name: /^All\d/ }));
    fireEvent.change(screen.getByLabelText('Price support'), { target: { value: 'supported' } });
    await waitFor(() => expect(screen.getByText('1 card shown')).toBeVisible());
    fireEvent.click(screen.getByRole('checkbox', { name: 'Select all visible cards' }));
    expect(screen.getByText('1 selected for show')).toBeVisible();
    fireEvent.change(screen.getByLabelText('Search cards'), { target: { value: 'Charizard' } });
    await waitFor(() => expect(screen.getByText('0 cards shown')).toBeVisible());
    expect(screen.getByText('1 selected for show')).toBeVisible();
    fireEvent.click(screen.getByLabelText('Include not received'));
    await waitFor(() => expect(screen.getByText('1 card shown')).toBeVisible());
    fireEvent.click(screen.getByRole('button', { name: /^DH Listed\d/ }));
    expect(screen.getByText('0 cards shown')).toBeVisible();
    fireEvent.change(screen.getByLabelText('Show list'), { target: { value: listId } });
    fireEvent.click(screen.getByRole('button', { name: 'Add selected to show (1)' }));
    await waitFor(() => expect(requests.some(r => r.url.endsWith('/items'))).toBe(true));
    expect(requests.find(r => r.url.endsWith('/items'))?.body).toEqual({ items: [{ purchaseId, evaluationVersion: 'eval-1' }] });
  });

  it('keeps missing evidence manually selectable and retains selection on a stale add conflict', async () => {
    addFails = true; mount();
    fireEvent.click(await screen.findByRole('button', { name: 'Show selection' }));
    fireEvent.click(screen.getByRole('button', { name: /^All\d/ }));
    fireEvent.change(screen.getByLabelText('Price support'), { target: { value: 'needs_review' } });
    await waitFor(() => expect(screen.getByText('1 card shown')).toBeVisible());
    fireEvent.click(screen.getByRole('checkbox', { name: 'Select all visible cards' }));
    fireEvent.change(screen.getByLabelText('Show list'), { target: { value: listId } });
    fireEvent.click(screen.getByRole('button', { name: 'Add selected to show (1)' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(/changed/i);
    expect(screen.getByText('1 selected for show')).toBeVisible();
    expect(screen.queryByText(/Added .* to show/)).not.toBeInTheDocument();
    await waitFor(() => expect(requests.filter(r => r.url.endsWith('/evaluate')).length).toBeGreaterThan(1));
  });

  it.each(['row', 'select-all'])('retains the observed version of a hidden selection through list creation/refetch (%s)', async selectionMethod => {
    let current = values[0];
    let created = detail().list;
    vi.stubGlobal('fetch', vi.fn(async (url: string, options: RequestInit = {}) => {
      const body = options.body ? JSON.parse(String(options.body)) : {};
      requests.push({ url, body });
      let response: unknown = detail();
      if (url.endsWith('/evaluate')) response = { evaluations: [current, ...values.slice(1)] };
      else if (url.endsWith('/lists') && options.method === 'POST') {
        current = evaluation({ version: 'eval-2', status: 'below_target', listedPriceCents: 35000 });
        created = { ...created, ...body }; response = created;
      } else if (url.endsWith('/lists')) response = { lists: [created] };
      return new Response(JSON.stringify(response));
    }));
    const qc = mount();
    fireEvent.click(await screen.findByRole('button', { name: 'Show selection' }));
    fireEvent.change(screen.getByLabelText('Price support'), { target: { value: 'supported' } });
    await waitFor(() => expect(screen.getByText('1 card shown')).toBeVisible());
    fireEvent.click(screen.getByRole('checkbox', { name: selectionMethod === 'row' ? 'Select 12345678' : 'Select all visible cards' }));
    fireEvent.change(screen.getByLabelText('Search cards'), { target: { value: 'Charizard' } });
    await waitFor(() => expect(screen.queryByRole('checkbox', { name: 'Select 12345678' })).not.toBeInTheDocument());
    fireEvent.change(screen.getByLabelText('New show name'), { target: { value: 'Changed while hidden' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create show list' }));
    await waitFor(() => expect(screen.getByLabelText('Show list')).toHaveValue(created.id));
    await waitFor(() => expect(qc.getQueriesData<InventoryEvaluations>({ queryKey: showPrepKeys.evaluations })[0][1]?.evaluations[purchaseId].version).toBe('eval-2'));
    await waitFor(() => expect(qc.isFetching()).toBe(0));
    const add = screen.getByRole('button', { name: 'Add selected to show (1)' });
    expect(add).toBeDisabled();
    expect(screen.getByText('1 selected for show')).toBeVisible();
    expect(screen.getByText(/Review and reselect/)).toHaveTextContent('12345678');
    fireEvent.click(screen.getByRole('button', { name: 'Show selection' }));
    fireEvent.click(screen.getByRole('button', { name: 'Show selection' }));
    expect(screen.getByRole('button', { name: 'Add selected to show (1)' })).toBeDisabled();
    expect(requests.some(r => r.url.endsWith('/items'))).toBe(false);
    fireEvent.change(screen.getByLabelText('Price support'), { target: { value: 'all' } });
    fireEvent.change(screen.getByLabelText('Search cards'), { target: { value: '' } });
    const row = await screen.findByRole('checkbox', { name: 'Select 12345678' });
    expect(row).toBeChecked();
    expect(screen.getByText('$350.00', { selector: 'b' })).toBeVisible();
    fireEvent.click(screen.getByRole('checkbox', { name: 'Select all visible cards' }));
    expect(screen.getByRole('button', { name: 'Add selected to show (2)' })).toBeDisabled();
    fireEvent.click(screen.getByRole('checkbox', { name: 'Select 87654321' }));
    fireEvent.click(row); fireEvent.click(row);
    fireEvent.change(screen.getByLabelText('Show list'), { target: { value: created.id } });
    fireEvent.click(screen.getByRole('button', { name: 'Add selected to show (1)' }));
    await waitFor(() => expect(requests.find(r => r.url.endsWith('/items'))?.body).toEqual({ items: [{ purchaseId, evaluationVersion: 'eval-2' }] }));
  });

  it('keeps unfinished cards loading when detail publishes during the initial batch read', async () => {
    let release!: (value: Response) => void;
    const pending = new Promise<Response>(resolve => { release = resolve; });
    vi.stubGlobal('fetch', vi.fn(async (url: string) => url.endsWith('/evaluate') ? pending : new Response(JSON.stringify({ evaluation: values[0], sales: [] }))));
    mount();
    try {
      fireEvent.click(screen.getByRole('button', { name: /^All\d/ }));
      fireEvent.click(screen.getByRole('button', { name: 'Show 30-day evidence 12345678' }));
      await screen.findByText(/No detailed sales available/);
      expect(screen.getAllByText('Loading price support…')).toHaveLength(2);
      expect(screen.queryByText(/Evaluation unavailable:/)).not.toBeInTheDocument();
    } finally {
      await act(async () => { release(new Response(JSON.stringify({ evaluations: values }))); });
    }
  });

  it('offers Retry evaluation when a partial aggregate is cancelled while inventory remains mounted', async () => {
    let release!: (value: Response) => void;
    const pending = new Promise<Response>(resolve => { release = resolve; });
    let batches = 0;
    vi.stubGlobal('fetch', vi.fn(async (url: string) => {
      if (!url.endsWith('/evaluate')) return new Response(JSON.stringify({ evaluation: values[0], sales: [] }));
      return ++batches === 1 ? pending : new Response(JSON.stringify({ evaluations: values }));
    }));
    const qc = mount();
    try {
      fireEvent.click(screen.getByRole('button', { name: /^All\d/ }));
      fireEvent.click(screen.getByRole('button', { name: 'Show 30-day evidence 12345678' }));
      await screen.findByText(/No detailed sales available/);
      await act(async () => { await qc.cancelQueries({ queryKey: showPrepKeys.evaluations }); });
      const retry = await screen.findByRole('button', { name: 'Retry evaluation' });
      expect(retry).toBeEnabled();
      expect(screen.getByRole('alert')).toHaveTextContent('2 evaluations unavailable');
      fireEvent.click(retry);
      await waitFor(() => expect(batches).toBe(2));
      await waitFor(() => expect(screen.queryByText(/Evaluation unavailable:/)).not.toBeInTheDocument());
      expect(screen.queryByRole('button', { name: 'Retry evaluation' })).not.toBeInTheDocument();
    } finally {
      await act(async () => { release(new Response(JSON.stringify({ evaluations: values }))); });
    }
  });

  it('loads evidence only on disclosure, displays every sale and rejects unsafe links', async () => {
    mount();
    await waitFor(() => expect(screen.getAllByText('Supported', { selector: 'strong' }).length).toBeGreaterThan(0));
    expect(requests.some(r => r.url.includes('/evidence/'))).toBe(false);
    fireEvent.click(screen.getByRole('button', { name: 'Show 30-day evidence 12345678' }));
    const evidence = await screen.findByRole('region', { name: '30-day evidence 12345678' });
    expect(await within(evidence).findByText('$270.00')).toBeVisible();
    expect(within(evidence).getByText('$290.00')).toBeVisible();
    expect(within(evidence).getByRole('link', { name: /eBay/ })).toHaveAttribute('href', 'https://example.com/sale');
    expect(within(evidence).getAllByRole('link')).toHaveLength(1);
    expect(evidence).toHaveTextContent('2026-08-16');
    expect(requests.some(r => r.url.endsWith('/refresh'))).toBe(false);
  });
});
