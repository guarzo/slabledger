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
function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={qc}><MemoryRouter><ToastProvider><InventoryTab items={items} isLoading={false} /></ToastProvider></MemoryRouter></QueryClientProvider>);
  return qc;
}
async function chooseList(id = listId) {
  if (!screen.queryByRole('combobox', { name: 'Show list' })) fireEvent.click(screen.getByRole('button', { name: /^Add selected to show/ }));
  await waitFor(() => expect(screen.getByLabelText('Show list')).toBeEnabled());
  fireEvent.change(screen.getByLabelText('Show list'), { target: { value: id } });
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
  it('does not introduce empty pending queues when entering show selection', async () => {
    mount();
    await screen.findAllByText('Supported', { selector: 'strong' });
    fireEvent.click(screen.getByRole('button', { name: 'Show selection' }));
    await screen.findByText('2 cards shown');
    expect(screen.queryByRole('button', { name: /^Pending DH Listing/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^Pending DH Match/ })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^DH Listed/ })).toBeVisible();
  });
  it('keeps one compact price-area disclosure and no permanent evidence footer in either selection mode', async () => {
    mount();
    const trigger = await screen.findByRole('button', { name: 'Show 30-day evidence 12345678' });
    expect(trigger.closest('[role="row"]')).not.toBeNull();
    expect(screen.queryByText(/30d median/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Show selection' }));
    expect(screen.queryByRole('region', { name: 'Show selection actions' })).not.toBeInTheDocument();
    expect(screen.queryByText(/30d median/)).not.toBeInTheDocument();
    fireEvent.click(trigger);
    const region = await screen.findByRole('region', { name: '30-day evidence 12345678' });
    expect(trigger).toHaveAttribute('aria-controls', region.id);
    expect(await within(region).findByText('$270.00')).toBeVisible();
    expect(screen.getAllByRole('button', { name: /30-day evidence 12345678/ })).toHaveLength(1);
  });
  it('keeps failed or missing evaluations distinct from no comps, with an explicit retry', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ evaluations: [values[0]] }))));
    mount();
    expect(await screen.findByRole('alert')).toHaveTextContent('2 evaluations unavailable');
    fireEvent.click(screen.getByRole('button', { name: /^All\d/ }));
    expect(screen.queryByText('No recent comps', { selector: 'strong' })).not.toBeInTheDocument();
    expect(screen.getAllByText('Evaluation unavailable', { selector: 'strong' })).toHaveLength(2);
    fireEvent.click(screen.getByRole('button', { name: 'Show selection' }));
    expect(await screen.findByText('1 card shown')).toBeVisible();
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
    await chooseList();
    fireEvent.click(screen.getByRole('button', { name: 'Add selected to show (1)' }));
    await waitFor(() => expect(requests.some(r => r.url.endsWith('/items'))).toBe(true));
    expect(requests.find(r => r.url.endsWith('/items'))?.body).toEqual({ items: [{ purchaseId, evaluationVersion: 'eval-1' }] });
  });

  it('clears successful-add feedback only when the actual target list changes', async () => {
    const otherListId = '66666666-6666-4666-8666-666666666666';
    const qc = mount();
    fireEvent.click(await screen.findByRole('button', { name: 'Show selection' }));
    await waitFor(() => expect(screen.getByText('2 cards shown')).toBeVisible());
    await act(async () => { qc.setQueryData(showPrepKeys.lists, [detail().list, { ...detail().list, id: otherListId, name: 'Other show' }]); });
    fireEvent.click(screen.getByRole('checkbox', { name: 'Select 12345678' }));
    await chooseList();
    fireEvent.click(screen.getByRole('button', { name: 'Add selected to show (1)' }));
    const link = await screen.findByRole('link', { name: 'Open packing list →' });
    expect(link).toHaveAttribute('href', `/shows?list=${listId}`);
    expect(screen.queryByRole('region', { name: 'Show selection actions' })).not.toBeInTheDocument();
    // List invalidation and selection clearing after add must preserve feedback.
    await act(async () => { qc.setQueryData(showPrepKeys.lists, [detail().list, { ...detail().list, id: otherListId, name: 'Other show' }]); });
    expect(screen.getByText(/Added 1 card/)).toBeVisible();
    fireEvent.click(screen.getByRole('checkbox', { name: 'Select 12345678' }));
    await chooseList();
    expect(screen.getByText(/Added 1 card/)).toBeVisible();
    await chooseList(otherListId);
    expect(screen.queryByText(/Added 1 card/)).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Open packing list →' })).not.toBeInTheDocument();
  });

  it('keeps missing evidence manually selectable and retains selection on a stale add conflict', async () => {
    addFails = true; mount();
    fireEvent.click(await screen.findByRole('button', { name: 'Show selection' }));
    fireEvent.click(screen.getByRole('button', { name: /^All\d/ }));
    fireEvent.change(screen.getByLabelText('Price support'), { target: { value: 'needs_review' } });
    await waitFor(() => expect(screen.getByText('1 card shown')).toBeVisible());
    fireEvent.click(screen.getByRole('checkbox', { name: 'Select all visible cards' }));
    await chooseList();
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
    await chooseList();
    fireEvent.click(screen.getByRole('button', { name: 'New list' }));
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
    fireEvent.click(screen.getByRole('button', { name: 'Show 30-day evidence 12345678' }));
    expect(screen.getByText('$350.00', { selector: 'b' })).toBeVisible();
    fireEvent.click(screen.getByRole('checkbox', { name: 'Select all visible cards' }));
    expect(screen.getByRole('button', { name: 'Add selected to show (2)' })).toBeDisabled();
    fireEvent.click(screen.getByRole('checkbox', { name: 'Select 87654321' }));
    fireEvent.click(row); fireEvent.click(row);
    await chooseList(created.id);
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
      await screen.findByText(/Complete current lookup/);
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
      // Keep both the initial read and the detail-triggered replacement pending.
      return ++batches <= 2 ? pending : new Response(JSON.stringify({ evaluations: values }));
    }));
    const qc = mount();
    try {
      fireEvent.click(screen.getByRole('button', { name: /^All\d/ }));
      fireEvent.click(screen.getByRole('button', { name: 'Show 30-day evidence 12345678' }));
      await screen.findByText(/Complete current lookup/);
      await act(async () => { await qc.cancelQueries({ queryKey: showPrepKeys.evaluations }); });
      const retry = await screen.findByRole('button', { name: 'Retry evaluation' });
      expect(retry).toBeEnabled();
      expect(screen.getByRole('alert')).toHaveTextContent('2 evaluations unavailable');
      fireEvent.click(retry);
      await waitFor(() => expect(batches).toBe(3));
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


it('Supported-first checks the cold pre-Support cohort, not unrelated search/tab/price inventory', async () => {
  const cold = values.map((e, index) => ({ ...e, status: 'needs_review', evidenceNeedsReview: true,
    readiness: { state: 'not_checked', refreshEligibility: 'needed', identityKey: (index + 1).toString().padStart(64, '0'), expiresAt: '', retryAt: '' } }));
  const calls: { url: string; ids: string[] }[] = [];
  vi.stubGlobal('fetch', vi.fn(async (url: string, options: RequestInit = {}) => {
    const ids: string[] = options.body ? JSON.parse(String(options.body)).purchaseIds ?? [] : [];
    calls.push({ url, ids });
    if (url.endsWith('/refresh')) ids.forEach(id => {
      const e = cold.find(value => value.purchaseId === id)!;
      e.status = 'supported'; e.evidenceNeedsReview = false;
      e.readiness = { ...e.readiness, state: 'current', refreshEligibility: 'not_needed', expiresAt: '2099-01-01T00:00:00Z' };
    });
    return new Response(JSON.stringify(url.endsWith('/lists') ? { lists: [] } : { evaluations: cold.filter(e => ids.includes(e.purchaseId)) }));
  }));
  mount();
  fireEvent.click(screen.getByRole('button', { name: /^DH Listed/ }));
  fireEvent.change(screen.getByLabelText('Search cards'), { target: { value: '12345678' } });
  await waitFor(() => expect(screen.getByText('1 card shown')).toBeVisible());
  fireEvent.change(screen.getByLabelText('Price support'), { target: { value: 'supported' } });
  await waitFor(() => expect(calls.filter(c => c.url.endsWith('/refresh')).map(c => c.ids)).toEqual([[purchaseId]]));
  await waitFor(() => expect(screen.getByText('1 card shown')).toBeVisible());
  expect(screen.getByRole('checkbox', { name: 'Select 12345678' })).toBeInTheDocument();
});

it('keeps selected-view membership stable on evidence updates, but uses live versions and explicit filters', async () => {
  const qc = mount();
  fireEvent.click(await screen.findByRole('button', { name: 'Show selection' }));
  fireEvent.change(screen.getByLabelText('Price support'), { target: { value: 'supported' } });
  await waitFor(() => expect(screen.getByText('1 card shown')).toBeVisible());
  fireEvent.click(screen.getByRole('checkbox', { name: 'Select 12345678' }));
  await act(async () => {
    qc.setQueriesData<InventoryEvaluations>({ queryKey: showPrepKeys.evaluations }, old => ({ evaluations: { ...old?.evaluations,
      [purchaseId]: evaluation({ status: 'below_target', version: 'changed' }) }, errors: {} }));
  });
  expect(await screen.findByText(/Review and reselect/)).toHaveTextContent('12345678');
  expect(screen.getByRole('checkbox', { name: 'Select 12345678' })).toBeChecked();
  fireEvent.change(screen.getByLabelText('Search cards'), { target: { value: 'Charizard' } });
  await waitFor(() => expect(screen.queryByRole('checkbox', { name: 'Select 12345678' })).not.toBeInTheDocument());
  expect(screen.getByText(/1 selected outside this view/)).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Reveal selected' }));
  await waitFor(() => expect(screen.getByRole('checkbox', { name: 'Select 12345678' })).toBeChecked());
  expect(screen.getByRole('button', { name: 'Add selected to show (1)' })).toBeDisabled();
});

it('reveals a now-unavailable selected row without making it addable; Escape retains modal priority', async () => {
  const qc = mount(); fireEvent.click(await screen.findByRole('button', { name: 'Show selection' }));
  await waitFor(() => expect(screen.getByText('2 cards shown')).toBeVisible());
  fireEvent.click(screen.getByRole('checkbox', { name: 'Select 12345678' }));
  await act(async () => { qc.setQueriesData<InventoryEvaluations>({ queryKey: showPrepKeys.evaluations }, old => ({ evaluations: { ...old?.evaluations,
    [purchaseId]: evaluation({ availability: 'sold', canAdd: false, version: 'sold' }) }, errors: {} })); });
  await screen.findByText(/Review and reselect/);
  fireEvent.change(screen.getByLabelText('Search cards'), { target: { value: 'Charizard' } });
  await screen.findByText(/1 selected outside this view/);
  fireEvent.click(screen.getByRole('button', { name: 'Reveal selected' }));
  expect(await screen.findByRole('checkbox', { name: 'Select 12345678' })).toBeChecked();
  expect(screen.getByRole('button', { name: 'Add selected to show (1)' })).toBeDisabled();
  fireEvent.keyDown(window, { key: 'Escape' });
  expect(screen.queryByRole('region', { name: 'Show selection actions' })).not.toBeInTheDocument();
});

it('revalidates changed live purchases without acknowledging the selected evaluation version', async () => {
  let current = values[0];
  vi.stubGlobal('fetch', async (url: string) => new Response(JSON.stringify(url.endsWith('/lists') ? { lists: [detail().list] } : { evaluations: [current] })));
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const page = (liveItems: ReturnType<typeof inventoryItem>[]) => <QueryClientProvider client={qc}><MemoryRouter><ToastProvider><InventoryTab items={liveItems} isLoading={false} /></ToastProvider></MemoryRouter></QueryClientProvider>;
  const view = render(page([inventoryItem(current)]));
  fireEvent.click(screen.getByRole('button', { name: 'Show selection' }));
  await screen.findByText('1 card shown');
  fireEvent.click(screen.getByRole('checkbox', { name: 'Select 12345678' }));
  await chooseList();
  await waitFor(() => expect(screen.getByRole('button', { name: 'Add selected to show (1)' })).toBeEnabled());
  current = evaluation({ listedPriceCents: 35000, version: 'live-price-change' });
  view.rerender(page([inventoryItem(current)]));
  expect(await screen.findByText(/Review and reselect/)).toHaveTextContent('12345678');
  expect(screen.getByRole('button', { name: 'Add selected to show (1)' })).toBeDisabled();
  view.unmount(); qc.clear();
});
