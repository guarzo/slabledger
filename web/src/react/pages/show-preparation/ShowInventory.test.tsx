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
const values = [evaluation(), evaluation({ purchaseId: secondId, certNumber: '87654321', status: 'needs_review', reason: 'Source unavailable', evidenceNeedsReview: true, evidenceReason: 'Source unavailable' }),
  evaluation({ purchaseId: thirdId, cardName: 'Charizard', certNumber: '99999999', availability: 'not_received', canPack: false })];
const items = values.map((e, i) => inventoryItem(e, i === 2 ? { receivedAt: '', dhStatus: '' } : {}));
let requests: { url: string; body: Record<string, unknown> }[];
let addFails: boolean;
function mount(liveItems = items) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={qc}><MemoryRouter><ToastProvider><InventoryTab items={liveItems} isLoading={false} /></ToastProvider></MemoryRouter></QueryClientProvider>);
  return qc;
}
const add = (n = 1) => screen.getByRole('button', { name: `Add to show (${n})` });
async function chooseList(id = listId, n = 1) {
  if (!screen.queryByRole('combobox', { name: 'Show list' })) fireEvent.click(add(n));
  await waitFor(() => expect(screen.getByLabelText('Show list')).toBeEnabled());
  fireEvent.change(screen.getByLabelText('Show list'), { target: { value: id } });
}
const select = () => fireEvent.click(screen.getByRole('checkbox', { name: 'Select 12345678' }));
const support = (value: string) => fireEvent.change(screen.getByLabelText('Price support'), { target: { value } });
beforeEach(() => {
  requests = []; addFails = false;
  vi.stubGlobal('scrollTo', vi.fn());
  vi.stubGlobal('fetch', vi.fn().mockImplementation(async (url: string, options: RequestInit = {}) => {
    const body = options.body ? JSON.parse(String(options.body)) : {};
    requests.push({ url, body });
    let response: unknown = {}; let status = 200;
    if (url.endsWith('/refresh')) { response = { error: 'retired' }; status = 410; }
    else if (url.endsWith('/coverage')) response = { enabled: true, configured: true, state: 'idle', eligibleIdentities: 2, currentIdentities: 1, missingIdentities: 1, staleIdentities: 0, failedIdentities: 0, eligibleCards: 3, currentCards: 2, unresolvedCards: 0, lastSweepAt: '', retryAt: '', error: '' };
    else if (url.endsWith('/evaluate')) response = { evaluations: values };
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

describe('cached show preparation in the existing inventory', () => {
  it('uses the existing bulk bar for cached-only selection without checkbox or filter requests', async () => {
    const qc = mount(); await waitFor(() => expect(qc.isFetching()).toBe(0));
    expect(requests.filter(r => r.url.endsWith('/coverage'))).toHaveLength(1);
    const before = requests.length; select();
    const bar = screen.getByRole('region', { name: 'Bulk actions for selected cards' });
    expect(within(bar).getByRole('button', { name: /^Add to show/ })).toBeEnabled();
    expect(within(bar).getByRole('button', { name: /^Record sale/ })).toBeEnabled();
    expect(within(bar).getByRole('button', { name: /^List on DH/ })).toBeEnabled();
    await act(async () => {}); expect(requests).toHaveLength(before);
    support('supported'); await act(async () => {}); expect(requests).toHaveLength(before);
    expect(screen.queryByRole('button', { name: /Show selection|Check selected|Cancel checking|Check comps|Retry.*checking/ })).not.toBeInTheDocument();
    expect(requests.filter(r => r.url.endsWith('/refresh'))).toEqual([]);
  });
  it('intersects support, search and tab; not-received cards remain manually plannable', async () => {
    mount(); await screen.findAllByText('Supported', { selector: 'strong' });
    support('supported'); fireEvent.click(screen.getByRole('button', { name: /^DH Listed/ }));
    fireEvent.click(screen.getByRole('checkbox', { name: 'Select all visible cards' }));
    expect(add()).toBeEnabled();
    fireEvent.change(screen.getByLabelText('Search cards'), { target: { value: 'Charizard' } });
    await screen.findByText('0 cards shown'); expect(add()).toBeEnabled();
    fireEvent.click(screen.getByRole('button', { name: /^All\d/ }));
    await screen.findByText('1 card shown');
    fireEvent.click(screen.getByRole('checkbox', { name: 'Select 99999999' }));
    await chooseList(listId, 2); fireEvent.click(add(2));
    await waitFor(() => expect(requests.find(r => r.url.endsWith('/items'))?.body).toEqual({ items: [
      { purchaseId, evaluationVersion: 'eval-1' }, { purchaseId: thirdId, evaluationVersion: 'eval-1' },
    ] }));
  });
  it('keeps failed or missing evaluations distinct from no recent sales, with a database-read retry', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ evaluations: [values[0]] }))));
    mount(); expect(await screen.findByRole('alert')).toHaveTextContent('2 evaluations unavailable');
    fireEvent.click(screen.getByRole('button', { name: /^All\d/ }));
    expect(screen.queryByText('No recent sales', { selector: 'strong' })).not.toBeInTheDocument();
    expect(screen.getAllByText('Evaluation unavailable', { selector: 'strong' })).toHaveLength(2);
    expect(screen.getByRole('button', { name: 'Retry evaluation' })).toBeEnabled();
  });
  it('keeps missing evidence manually selectable and retains selection on a stale add conflict', async () => {
    addFails = true; mount(); await screen.findAllByText('Supported', { selector: 'strong' });
    support('needs_review'); await screen.findByText('1 card shown');
    fireEvent.click(screen.getByRole('checkbox', { name: 'Select all visible cards' }));
    await chooseList(); fireEvent.click(add());
    expect(await screen.findByRole('alert')).toHaveTextContent(/changed/i);
    expect(add()).toBeVisible(); expect(screen.queryByText(/Added .* to show/)).not.toBeInTheDocument();
    await waitFor(() => expect(requests.filter(r => r.url.endsWith('/evaluate')).length).toBeGreaterThan(1));
  });
  it.each(['row', 'select-all'])('retains hidden observed versions through creation/refetch (%s)', async method => {
    let current = values[0]; let created = detail().list;
    vi.stubGlobal('fetch', vi.fn(async (url: string, options: RequestInit = {}) => {
      const body = options.body ? JSON.parse(String(options.body)) : {}; requests.push({ url, body });
      let response: unknown = detail();
      if (url.endsWith('/evaluate')) response = { evaluations: [current, ...values.slice(1)] };
      else if (url.endsWith('/lists') && options.method === 'POST') {
        current = evaluation({ version: 'eval-2', status: 'below_target', listedPriceCents: 35000 });
        created = { ...created, ...body }; response = created;
      } else if (url.endsWith('/lists')) response = { lists: [created] };
      return new Response(JSON.stringify(response));
    }));
    const qc = mount(); await waitFor(() => expect(qc.isFetching()).toBe(0));
    fireEvent.click(screen.getByRole('button', { name: /^DH Listed/ })); support('supported');
    fireEvent.click(screen.getByRole('checkbox', { name: method === 'row' ? 'Select 12345678' : 'Select all visible cards' }));
    fireEvent.change(screen.getByLabelText('Search cards'), { target: { value: 'Charizard' } });
    await screen.findByText('0 cards shown'); await chooseList();
    fireEvent.click(screen.getByRole('button', { name: 'New list' }));
    fireEvent.change(screen.getByLabelText('New show name'), { target: { value: 'Changed while hidden' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create show list' }));
    await screen.findByText(/Review and reselect/); expect(add()).toBeDisabled();
    expect(requests.some(r => r.url.endsWith('/items'))).toBe(false);
    fireEvent.click(screen.getByRole('button', { name: 'Reveal selected' }));
    const row = await screen.findByRole('checkbox', { name: 'Select 12345678' }); expect(row).toBeChecked();
    fireEvent.click(row);
    support('all'); fireEvent.change(screen.getByLabelText('Search cards'), { target: { value: '' } });
    await screen.findByRole('checkbox', { name: 'Select 12345678' }); select();
    await chooseList(created.id); fireEvent.click(add());
    await waitFor(() => expect(requests.find(r => r.url.endsWith('/items'))?.body).toEqual({ items: [{ purchaseId, evaluationVersion: 'eval-2' }] }));
  });
  it.each(['below_target', 'sold'])('keeps selected row membership stable but requires review after %s', async change => {
    const qc = mount(); await waitFor(() => expect(qc.isFetching()).toBe(0));
    support('supported'); select();
    await act(async () => qc.setQueriesData<InventoryEvaluations>({ queryKey: showPrepKeys.evaluations }, old => ({ evaluations: { ...old?.evaluations,
      [purchaseId]: evaluation(change === 'sold' ? { availability: 'sold', canAdd: false, version: 'changed' } : { status: 'below_target', version: 'changed' }) }, errors: {} })));
    expect(await screen.findByText(/Review and reselect/)).toHaveTextContent('12345678');
    expect(screen.getByRole('checkbox', { name: 'Select 12345678' })).toBeChecked();
    fireEvent.change(screen.getByLabelText('Search cards'), { target: { value: 'Charizard' } });
    await screen.findByText(/1 selected outside this view/); fireEvent.click(screen.getByRole('button', { name: 'Reveal selected' }));
    expect(await screen.findByRole('checkbox', { name: 'Select 12345678' })).toBeChecked(); expect(add()).toBeDisabled();
    fireEvent.keyDown(window, { key: 'Escape' }); expect(screen.queryByRole('region', { name: 'Bulk actions for selected cards' })).not.toBeInTheDocument();
  });
  it('retains price-band controls when publication removes the last match above a held selection', async () => {
    const qc = mount(items.map(i => ({ ...i, currentMarket: { lastSoldCents: 30000, gradePriceCents: 30000 } })));
    await waitFor(() => expect(qc.isFetching()).toBe(0));
    fireEvent.click(screen.getByRole('button', { name: /^All\d/ }));
    fireEvent.change(screen.getByLabelText('Search cards'), { target: { value: '12345678' } });
    support('supported'); await screen.findByText('1 card shown'); select();
    expect(screen.getByRole('button', { name: /\$250–500/ })).toHaveTextContent('1');
    await act(async () => qc.setQueriesData<InventoryEvaluations>({ queryKey: showPrepKeys.evaluations }, old => ({ evaluations: { ...old?.evaluations,
      [purchaseId]: evaluation({ status: 'below_target', version: 'published' }) }, errors: {} })));
    await screen.findByText(/Review and reselect/);
    expect(screen.getByRole('button', { name: /\$250–500/ })).toHaveTextContent('0');
    expect(screen.getByRole('checkbox', { name: 'Select 12345678' })).toBeChecked();
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(screen.queryByRole('button', { name: /\$250–500/ })).not.toBeInTheDocument();
    expect(screen.getByText('No current matches under these filters.')).toBeVisible();
  });
  it('revalidates live purchase changes without acknowledging selected versions', async () => {
    let current = values[0];
    vi.stubGlobal('fetch', async (url: string) => new Response(JSON.stringify(url.endsWith('/lists') ? { lists: [detail().list] } : { evaluations: [current] })));
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const page = (live: ReturnType<typeof inventoryItem>[]) => <QueryClientProvider client={qc}><MemoryRouter><ToastProvider><InventoryTab items={live} isLoading={false} /></ToastProvider></MemoryRouter></QueryClientProvider>;
    const view = render(page([inventoryItem(current)])); await waitFor(() => expect(qc.isFetching()).toBe(0)); select();
    expect(add()).toBeEnabled(); current = evaluation({ listedPriceCents: 35000, version: 'live-change' });
    view.rerender(page([inventoryItem(current)])); await screen.findByText(/Review and reselect/); expect(add()).toBeDisabled();
    view.unmount(); qc.clear();
  });
  it.each([false, true])('keeps unfinished cards honest when detail interrupts an initial aggregate (cancel=%s)', async cancel => {
    let release!: (value: Response) => void;
    const pending = new Promise<Response>(resolve => { release = resolve; }); let batches = 0;
    vi.stubGlobal('fetch', vi.fn(async (url: string) => {
      if (!url.endsWith('/evaluate')) return new Response(JSON.stringify({ evaluation: values[0], sales: [] }));
      return ++batches <= 2 ? pending : new Response(JSON.stringify({ evaluations: values }));
    }));
    const qc = mount();
    try {
      fireEvent.click(screen.getByRole('button', { name: /^All\d/ }));
      fireEvent.click(screen.getByRole('button', { name: /Show 30-day evidence 12345678/ }));
      await screen.findByText(/Complete current lookup/);
      expect(screen.getAllByText('Loading price support…')).toHaveLength(2);
      if (cancel) {
        await act(async () => qc.cancelQueries({ queryKey: showPrepKeys.evaluations }));
        const retry = await screen.findByRole('button', { name: 'Retry evaluation' });
        expect(screen.getByRole('alert')).toHaveTextContent('2 evaluations unavailable'); fireEvent.click(retry);
        await waitFor(() => expect(batches).toBe(3));
        await waitFor(() => expect(screen.queryByRole('button', { name: 'Retry evaluation' })).not.toBeInTheDocument());
      }
    } finally { await act(async () => release(new Response(JSON.stringify({ evaluations: values })))); }
  });
  it('loads one compact stored evidence disclosure only on intent, showing every sale and safe links', async () => {
    mount(); const trigger = await screen.findByRole('button', { name: /Show 30-day evidence 12345678/ });
    expect(trigger.closest('[role="row"]')).not.toBeNull(); expect(screen.queryByText(/30d median/)).not.toBeInTheDocument();
    expect(requests.some(r => r.url.includes('/evidence/'))).toBe(false); fireEvent.click(trigger);
    const region = await screen.findByRole('region', { name: '30-day evidence 12345678' });
    expect(trigger).toHaveAttribute('aria-controls', region.id);
    expect(await within(region).findByText('$270.00')).toBeVisible(); expect(within(region).getByText('$290.00')).toBeVisible();
    expect(within(region).getByRole('link', { name: /eBay/ })).toHaveAttribute('href', 'https://example.com/sale');
    expect(within(region).getAllByRole('link')).toHaveLength(1); expect(region).toHaveTextContent('2026-08-16');
    expect(requests.some(r => r.url.endsWith('/refresh'))).toBe(false);
  });
  it('does not warm a cold Supported filter or conflate missing data with no sales', async () => {
    const cold = values.map(e => ({ ...e, status: 'needs_review', evidenceNeedsReview: true, readiness: {
      state: 'not_checked', refreshEligibility: 'needed', identityKey: '1'.padStart(64, '0'), expiresAt: '', retryAt: '',
    } }));
    const calls: string[] = [];
    vi.stubGlobal('fetch', async (url: string) => { calls.push(url); return Response.json({ evaluations: cold }); });
    const qc = mount(); await waitFor(() => expect(qc.isFetching()).toBe(0)); support('supported');
    await screen.findByText('No current matches under these filters.');
    expect(screen.getByText(/Comp data coverage is incomplete/)).toBeVisible();
    await act(async () => {}); expect(calls.sort()).toEqual(['/api/show-prep/coverage', '/api/show-prep/evaluate']);
  });
});
