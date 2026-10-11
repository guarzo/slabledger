import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { ToastProvider } from '../../contexts/ToastContext';
import InventoryTab from '../campaign-detail/InventoryTab';
import { detail, evaluation, inventoryItem, purchaseId } from './fixtures.test-support';

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
let values: ReturnType<typeof evaluation>[];
let requests: { url: string; body: Record<string, unknown> }[];
let addFails: boolean;
let missing: boolean;
function mount(liveItems = values.map((e, i) => inventoryItem(e, i === 2 ? { receivedAt: '', dhStatus: '' } : {}))) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={qc}><MemoryRouter><ToastProvider><InventoryTab items={liveItems} isLoading={false} /></ToastProvider></MemoryRouter></QueryClientProvider>);
  return qc;
}
const select = () => fireEvent.click(screen.getByRole('checkbox', { name: 'Select 12345678' }));
const search = (value: string) => fireEvent.change(screen.getByLabelText('Search cards'), { target: { value } });
const all = () => fireEvent.click(screen.getByRole('button', { name: /^All\d/ }));
beforeEach(() => {
  requests = []; addFails = false; missing = false;
  values = [evaluation(), evaluation({ purchaseId: secondId, certNumber: '87654321', status: 'needs_review', reason: 'Source unavailable', evidenceNeedsReview: true, evidenceReason: 'Source unavailable' }),
    evaluation({ purchaseId: thirdId, cardName: 'Charizard', certNumber: '99999999', availability: 'not_received', canPack: false })];
  vi.stubGlobal('scrollTo', vi.fn());
  vi.stubGlobal('fetch', vi.fn(async (url: string, options: RequestInit = {}) => {
    const body = options.body ? JSON.parse(String(options.body)) : {}; requests.push({ url, body });
    if (url.endsWith('/evaluate')) return Response.json({ evaluations: missing ? [values[0]] : values });
    if (url.endsWith('/lists')) return Response.json({ lists: [detail().list] });
    if (url.endsWith('/items')) return Response.json(addFails ? { error: 'Evaluation changed. Review current data.' } : detail(), { status: addFails ? 409 : 200 });
    return Response.json({ error: 'Unexpected fixture request' }, { status: 400 });
  }));
});
afterEach(() => vi.unstubAllGlobals());

describe('cached show selection alongside normal inventory', () => {
  it('uses one existing bulk bar with sales and DH actions, without checkbox or filter acquisition', async () => {
    const qc = mount(); await waitFor(() => expect(qc.isFetching()).toBe(0)); const before = requests.length; select();
    expect(screen.queryByRole('button', { name: /Add.*show/ })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^Record sale/ })).toBeEnabled();
    expect(screen.getByRole('button', { name: /^List on DH/ })).toBeEnabled();
    all(); await act(async () => {}); expect(requests).toHaveLength(before);
    expect(screen.queryByLabelText('Price support')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Comp data coverage')).not.toBeInTheDocument();
    expect(requests.some(r => r.url.endsWith('/refresh'))).toBe(false);
  });
  it('preserves normal search tab-bypass and financial selection of not-received cards', async () => {
    const qc = mount(); await waitFor(() => expect(qc.isFetching()).toBe(0));
    fireEvent.click(screen.getByRole('button', { name: /^DH Listed/ })); select(); search('Charizard');
    expect(await screen.findByRole('checkbox', { name: 'Select 99999999' })).not.toBeChecked();
    expect(screen.getByText(/1 selected outside this view/)).toBeVisible();
    fireEvent.click(screen.getByRole('checkbox', { name: 'Select 99999999' }));
    expect(screen.getByRole('button', { name: 'Record sale (2)' })).toBeEnabled();
    expect(screen.queryByLabelText('Show list')).not.toBeInTheDocument();
    expect(requests.some(request => request.url.endsWith('/items'))).toBe(false);
  });
  it('keeps missing evaluations distinct from limited sales and retries only the read', async () => {
    missing = true; mount(); expect(await screen.findByRole('alert')).toHaveTextContent('2 evaluations unavailable'); all();
    expect(screen.queryByText('Limited evidence', { selector: 'strong' })).not.toBeInTheDocument();
    expect(screen.getAllByText('Evaluation unavailable', { selector: 'strong' })).toHaveLength(2);
    missing = false; fireEvent.click(screen.getByRole('button', { name: 'Retry evaluation' }));
    await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument());
    expect(requests.every(r => r.url.endsWith('/evaluate'))).toBe(true);
  });
  it('clears a failed freshness observation through the visible read retry', async () => {
    const qc = mount(); await waitFor(() => expect(qc.isFetching()).toBe(0)); missing = true;
    await act(async () => window.dispatchEvent(new Event('focus')));
    await screen.findByRole('alert'); await waitFor(() => expect(qc.isFetching()).toBe(0));
    missing = false; const clock = vi.spyOn(Date, 'now').mockReturnValue(Date.now() + 2000);
    try {
      fireEvent.click(screen.getByRole('button', { name: 'Retry evaluation' }));
      await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument());
    } finally { clock.mockRestore(); }
  });
  it('keeps missing evidence financially selectable without requiring show evaluation consent', async () => {
    const qc = mount(); await waitFor(() => expect(qc.isFetching()).toBe(0)); all();
    fireEvent.click(screen.getByRole('checkbox', { name: 'Select 87654321' }));
    expect(screen.getByRole('checkbox', { name: 'Select 87654321' })).toBeChecked();
    expect(screen.getByRole('button', { name: 'Record sale (1)' })).toBeEnabled();
    expect(screen.queryByRole('button', { name: /Add.*show/ })).not.toBeInTheDocument();
  });
  it.each(['row', 'select-all'])('retains hidden financial selection through a real evaluation read (%s)', async method => {
    const qc = mount(); await waitFor(() => expect(qc.isFetching()).toBe(0)); search('12345678');
    await screen.findByText('1 of 3 cards');
    fireEvent.click(screen.getByRole('checkbox', { name: method === 'row' ? 'Select 12345678' : 'Select all visible cards' }));
    search('Charizard'); await screen.findByText(/1 selected outside this view/);
    values[0] = evaluation({ version: 'eval-2', status: 'below_target', localPriceCents: 35000 });
    await act(async () => window.dispatchEvent(new Event('focus')));
    await waitFor(() => expect(qc.isFetching()).toBe(0));
    expect(screen.getByRole('button', { name: 'Record sale (1)' })).toBeEnabled();
    fireEvent.click(screen.getByRole('button', { name: 'Reveal selected' }));
    const row = await screen.findByRole('checkbox', { name: 'Select 12345678' }); expect(row).toBeChecked(); fireEvent.click(row);
    expect(row).not.toBeChecked();
    expect(requests.some(request => request.url.endsWith('/items'))).toBe(false);
  });
  it.each(['below_target', 'sold'])('keeps financial selection without show approval after %s publication', async change => {
    const qc = mount(); await waitFor(() => expect(qc.isFetching()).toBe(0)); select();
    values[0] = evaluation(change === 'sold' ? { availability: 'sold', canAdd: false, version: 'changed' } : { status: 'below_target', version: 'changed' });
    await act(async () => window.dispatchEvent(new Event('focus')));
    await waitFor(() => expect(qc.isFetching()).toBe(0));
    expect(screen.getByRole('checkbox', { name: 'Select 12345678' })).toBeChecked();
    expect(screen.queryByRole('button', { name: /Add.*show/ })).not.toBeInTheDocument();
    fireEvent.keyDown(window, { key: 'Escape' }); expect(screen.queryByRole('region', { name: 'Bulk actions for selected cards' })).not.toBeInTheDocument();
  });
  it('revalidates live purchase changes without losing financial selection', async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const page = (live: ReturnType<typeof inventoryItem>[]) => <QueryClientProvider client={qc}><MemoryRouter><ToastProvider><InventoryTab items={live} isLoading={false} /></ToastProvider></MemoryRouter></QueryClientProvider>;
    const view = render(page([inventoryItem(values[0])])); await waitFor(() => expect(qc.isFetching()).toBe(0)); select();
    values[0] = evaluation({ listedPriceCents: 35000, version: 'live-change' });
    view.rerender(page([inventoryItem(values[0])])); await waitFor(() => expect(qc.isFetching()).toBe(0));
    expect(screen.getByRole('checkbox', { name: 'Select 12345678' })).toBeChecked();
    expect(screen.getByRole('button', { name: 'Record sale (1)' })).toBeEnabled(); view.unmount(); qc.clear();
  });
  it('links compact assessment to global review while retaining operational sale expansion', async () => {
    mount(); const link = await screen.findByRole('link', { name: /Review price 12345678/ });
    expect(link.closest('[role="row"]')).not.toBeNull(); expect(link).toHaveAttribute('href', `/inventory?view=pricing&review=${purchaseId}`);
    expect(requests.some(r => r.url.includes('/evidence/'))).toBe(false);
    fireEvent.click(screen.getAllByRole('button', { name: 'Sell' })[0]);
    expect(await screen.findByText('Record sale')).toBeVisible();
    expect(screen.queryByRole('region', { name: /30-day evidence/ })).not.toBeInTheDocument();
  });
});
