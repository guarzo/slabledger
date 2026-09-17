import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, useLocation, useNavigate } from 'react-router-dom';
import { ToastProvider } from '../contexts/ToastContext';
import { queryKeys } from '../queries/queryKeys';
import { showPrepKeys } from '../queries/useShowPrepQueries';
import GlobalInventoryPage from './GlobalInventoryPage';
import { detail } from './show-preparation/fixtures.test-support';
import { evaluation, inventoryItem, otherId, preview, purchaseId, sales } from './price-review/fixtures.test-support';

// Only measurement is replaced: jsdom has no layout. HTTP, queries and all UI are real.
vi.mock('@tanstack/react-virtual', () => ({ useVirtualizer: ({ count }: { count: number }) => ({
  getTotalSize: () => count * 160,
  getVirtualItems: () => Array.from({ length: count }, (_, index) => ({ index, start: index * 160 })),
  measureElement: () => {},
}) }));
let values: ReturnType<typeof evaluation>[];
let requests: { url: string; method: string; body: Record<string, unknown> }[];
let failInventory: boolean;
let failWrite: boolean;
let emptyInventory: boolean;
let unreceived: Set<string>;
let missingEvaluations: Set<string>;
let failEvidence: string;
let inventoryGate: Promise<void> | undefined;
let qc: QueryClient;
function Location() {
  const location = useLocation(); const navigate = useNavigate();
  return <><output aria-label="Location">{location.search}</output><button onClick={() => navigate(-1)}>History back</button></>;
}
function mount(path = '/inventory?keep=yes', seedCache?: (client: QueryClient) => void) {
  qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  seedCache?.(qc);
  render(<QueryClientProvider client={qc}><MemoryRouter initialEntries={[path]}><ToastProvider><GlobalInventoryPage /><Location /></ToastProvider></MemoryRouter></QueryClientProvider>);
}
const click = (name: string | RegExp) => fireEvent.click(screen.getByRole('button', { name }));
const input = () => screen.getByRole('textbox', { name: 'Asking price' });
const draft = (value: string) => fireEvent.change(input(), { target: { value } });
const reviewCard = (name: string) => click(`Review ${name}`);
const selectedA = () => screen.getByRole('checkbox', { name: /^Select (00000001|Aurora Dragon)$/ });
const selectedB = () => screen.getByRole('checkbox', { name: /^Select (00000002|Moon Tortoise)$/ });
const writes = () => requests.filter(r => r.method === 'PATCH');
async function ready() { await screen.findByRole('textbox', { name: 'Asking price' }); await waitFor(() => expect(input()).toBeEnabled()); }
async function saveCurrentTrial() {
  await within(screen.getByRole('region', { name: 'Trial price assessment' })).findByText('Trial only · $2,400.00');
  expect(screen.getByRole('button', { name: 'Save price' })).toBeEnabled();
  expect(writes()).toHaveLength(0);
  click('Save price');
}
beforeEach(() => {
  values = [evaluation(), evaluation({ purchaseId: otherId, certNumber: '00000002', cardName: 'Moon Tortoise', version: 'saved-b' })];
  requests = []; failInventory = false; failWrite = false; emptyInventory = false; failEvidence = ''; inventoryGate = undefined; unreceived = new Set(); missingEvaluations = new Set();
  vi.stubGlobal('scrollTo', vi.fn());
  vi.stubGlobal('fetch', vi.fn(async (url: string, options: RequestInit = {}) => {
    const method = options.method ?? 'GET'; const body = options.body ? JSON.parse(String(options.body)) : {};
    requests.push({ url, method, body });
    if (url === '/api/inventory') {
      if (inventoryGate) await inventoryGate;
      return Response.json(failInventory ? { error: 'Inventory read unavailable' } : { items: emptyInventory ? [] : values.map(e => inventoryItem(e, unreceived.has(e.purchaseId) ? { receivedAt: '' } : {})), warnings: [] }, { status: failInventory ? 400 : 200 });
    }
    if (url.endsWith('/evaluate')) return Response.json({ evaluations: values.filter(e => !missingEvaluations.has(e.purchaseId)) });
    if (url.includes('/evidence/')) return Response.json(failEvidence && url.endsWith(failEvidence) ? { error: 'Evidence read unavailable' }
      : { evaluation: values.find(e => url.endsWith(e.purchaseId)), sales }, { status: failEvidence && url.endsWith(failEvidence) ? 400 : 200 });
    if (url.endsWith('/preview')) return Response.json(preview(body.priceCents, values.find(e => e.purchaseId === body.purchaseId)));
    if (url.endsWith('/review-price') && method === 'PATCH') {
      if (failWrite) return Response.json({ error: 'Price write refused' }, { status: 400 });
      values = values.map(e => url.includes(e.purchaseId) ? { ...e, localPriceCents: body.priceCents, version: 'saved-new', status: 'supported', reason: 'Authoritative saved assessment' } : e);
      return Response.json({ success: true, reviewedAt: '2026-09-16T12:00:00Z' });
    }
    if (url.endsWith('/lists')) return Response.json({ lists: [] });
    return Response.json({ error: `Unexpected fixture request: ${method} ${url}` }, { status: 400 });
  }));
});
afterEach(() => { qc?.clear(); vi.unstubAllGlobals(); });

it('defaults to operational inventory and compact links enter the correct review without duplicate expansion', async () => {
  mount(); await screen.findByRole('link', { name: /Review price 00000002/ });
  expect(screen.getAllByRole('button', { name: 'Sell' })).toHaveLength(2);
  expect(screen.queryByRole('region', { name: 'Price review' })).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Price support')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /Show 30-day evidence/ })).not.toBeInTheDocument();
  expect(requests.some(r => r.url.includes('/evidence/'))).toBe(false);
  fireEvent.click(screen.getByRole('link', { name: /Review price 00000002/ }));
  expect(await screen.findByRole('region', { name: 'Price review' })).toBeVisible();
  expect(within(screen.getByRole('region', { name: 'Price details' })).getByRole('heading', { name: 'Moon Tortoise' })).toBeVisible();
  expect(screen.getByLabelText('Location')).toHaveTextContent(`?keep=yes&view=pricing&review=${otherId}`);
  click('Inventory'); expect(screen.getAllByRole('button', { name: 'Sell' })).toHaveLength(2);
  expect(screen.getByLabelText('Location')).toHaveTextContent('?keep=yes');
  click('History back'); expect(screen.getByRole('region', { name: 'Price review' })).toBeVisible();
});

it('deep links and retains shared search, selection, both drafts and active identity through view changes', async () => {
  mount(`/inventory?view=pricing&review=${otherId}&keep=yes`); await ready(); draft('2500');
  reviewCard('Aurora Dragon'); draft('2400'); fireEvent.click(selectedA());
  fireEvent.change(screen.getByLabelText('Search cards'), { target: { value: 'Autumn' } });
  await waitFor(() => expect(screen.getByText('2 of 2 in-hand cards')).toBeVisible());
  click('Inventory'); expect(screen.getByLabelText('Search cards')).toHaveValue('Autumn');
  expect(screen.getByRole('button', { name: 'Add to show (1)' })).toBeVisible();
  click('Price review'); expect(input()).toHaveValue('2400'); expect(selectedA()).toBeChecked();
  reviewCard('Moon Tortoise'); expect(input()).toHaveValue('2500');
  expect(screen.getByLabelText('Location')).toHaveTextContent(`review=${otherId}`);
  expect(writes()).toHaveLength(0);
});

it('retains owner/data/drafts/focus/selection after confirmed PATCH then failed inventory GET; retry only reads', async () => {
  mount(`/inventory?view=pricing&review=${otherId}`); await ready(); draft('2500');
  reviewCard('Aurora Dragon'); draft('2400'); fireEvent.click(selectedA());
  await waitFor(() => expect(screen.getByRole('button', { name: 'Save price' })).toBeEnabled());
  failInventory = true; click('Save price');
  // Put keyboard focus on the other dirty editor while background inventory fails.
  reviewCard('Moon Tortoise'); const focused = input(); focused.focus();
  await screen.findByText(/Inventory could not be refreshed/); await waitFor(() => expect(qc.isMutating()).toBe(0));
  expect(focused).toHaveFocus(); expect(focused).toHaveValue('2500');
  expect(qc.getQueryData(queryKeys.portfolio.globalInventory)).toBeDefined(); expect(selectedA()).toBeChecked();
  reviewCard('Aurora Dragon'); expect(input()).toHaveValue('2400');
  expect(screen.getByText(/Price saved; assessment could not be refreshed/)).toBeVisible();
  expect(screen.getByRole('region', { name: 'Saved price assessment' })).toHaveTextContent(/pending|stale/i);
  expect(screen.getByRole('button', { name: 'Save price' })).toBeDisabled();
  expect(screen.queryByText(/Save not confirmed/)).not.toBeInTheDocument();
  expect(writes()).toEqual([{ url: `/api/purchases/${purchaseId}/review-price`, method: 'PATCH', body: { priceCents: 240000, source: 'manual' } }]);
  click('Inventory'); click('Price review'); expect(screen.getByText(/Price saved; assessment could not be refreshed/)).toBeVisible();
  const start = requests.length; failInventory = false; click('Recheck saved state');
  await screen.findByText(/Saved state rechecked/);
  await waitFor(() => expect(screen.queryByText(/Inventory could not be refreshed/)).not.toBeInTheDocument());
  expect(input()).toHaveValue('2400.00');
  expect(within(screen.getByRole('region', { name: 'Saved price assessment' })).getByText('Supported')).toBeVisible();
  expect(requests.slice(start).some(r => r.url === '/api/inventory')).toBe(true);
  expect(requests.slice(start).every(r => r.method === 'GET' || r.url.endsWith('/evaluate'))).toBe(true); expect(writes()).toHaveLength(1);
  reviewCard('Moon Tortoise'); expect(input()).toHaveValue('2500');
  expect(screen.getByRole('button', { name: 'Add to show (1)' })).toBeDisabled(); // captured version never silently advanced
});

it('failed PATCH retains draft and error without success or advance and never calls override API', async () => {
  mount(`/inventory?view=pricing&review=${purchaseId}`); await ready(); draft('2400'); failWrite = true; await saveCurrentTrial();
  await screen.findByText(/Save not confirmed: Price write refused/);
  expect(input()).toHaveValue('2400'); expect(screen.queryByText(/Price saved locally/)).not.toBeInTheDocument();
  expect(requests.some(r => r.url.includes('override'))).toBe(false); expect(writes()).toHaveLength(1);
  expect(screen.getByLabelText('Location')).toHaveTextContent(`review=${purchaseId}`);
});

it('select all uses the review queue, independent of normal operational filters', async () => {
  values[1] = { ...values[1], status: 'supported' }; mount('/inventory?view=pricing'); await ready();
  click(/^Supported 1$/); fireEvent.click(screen.getByRole('checkbox', { name: 'Select all visible cards' }));
  expect(selectedB()).toBeChecked(); expect(screen.getByRole('button', { name: 'Add to show (1)' })).toBeEnabled();
  click('Inventory'); expect(selectedB()).toBeChecked(); expect(selectedA()).not.toBeChecked();
});

it('reserves blocking errors for no successful response, not an empty successful inventory', async () => {
  failInventory = true; mount(); await screen.findByRole('button', { name: 'Retry' });
  expect(screen.queryByRole('button', { name: 'Price review' })).not.toBeInTheDocument();
  failInventory = false; emptyInventory = true; click('Retry'); await screen.findByText('All cards sold!');
  failInventory = true; await act(async () => { await qc.invalidateQueries({ queryKey: queryKeys.portfolio.globalInventory }); });
  expect(await screen.findByText(/Inventory could not be refreshed/)).toBeVisible(); expect(screen.getByText('All cards sold!')).toBeVisible();
});

it('rechecks the submitted card even if focus changes during the inventory read', async () => {
  mount(`/inventory?view=pricing&review=${purchaseId}`); await ready(); draft('2400'); failInventory = true; await saveCurrentTrial();
  await screen.findByText(/Price saved; assessment could not be refreshed/);
  let release!: () => void; inventoryGate = new Promise<void>(resolve => { release = resolve; });
  failInventory = false; failEvidence = purchaseId; click('Recheck saved state'); reviewCard('Moon Tortoise');
  await act(async () => release()); await waitFor(() => expect(qc.isFetching()).toBe(0));
  reviewCard('Aurora Dragon');
  expect(screen.getByText(/Price saved; assessment could not be refreshed/)).toBeVisible();
  expect(input()).toHaveValue('2400'); expect(screen.queryByText(/Saved state rechecked/)).not.toBeInTheDocument();
});

it('revealed selection is the actual review navigation queue, not a display-only replacement', async () => {
  mount('/inventory?view=pricing'); await ready(); fireEvent.click(screen.getByRole('checkbox', { name: 'Select all visible cards' }));
  click(/^Unavailable 0$/); click('Reveal selected');
  expect(screen.getByRole('list', { name: 'Price review queue' })).toBeVisible();
  click('Next card');
  expect(within(screen.getByRole('region', { name: 'Price details' })).getByRole('heading', { name: 'Moon Tortoise' })).toBeVisible();
  click('Previous card');
  expect(within(screen.getByRole('region', { name: 'Price details' })).getByRole('heading', { name: 'Aurora Dragon' })).toBeVisible();
  expect(selectedA()).toBeChecked(); expect(selectedB()).toBeChecked(); expect(writes()).toHaveLength(0);
});

it('scopes review select-all/reveal/search but preserves normal Awaiting intake and explicit price controls', async () => {
  unreceived.add(otherId);
  mount(); await screen.findByRole('link', { name: /Review price 00000001/ });
  click(/^All\s*2$/); expect(selectedB()).toBeVisible(); fireEvent.click(selectedB());
  const awaiting = within(selectedB().closest('[role="row"]')! as HTMLElement);
  expect(awaiting.getByTitle('Awaiting intake')).toBeVisible();
  fireEvent.click(awaiting.getByTitle('Click to edit list price'));
  expect(awaiting.getByRole('textbox', { name: 'List price' })).toBeEnabled();
  click('Price review'); await ready();
  expect(screen.getByRole('button', { name: 'All 1' })).toBeVisible();
  expect(screen.queryByRole('checkbox', { name: 'Select Moon Tortoise' })).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('checkbox', { name: 'Select all visible cards' }));
  expect(selectedA()).toBeChecked();
  click(/^Unavailable 0$/); click('Reveal selected');
  expect(screen.getAllByRole('button', { name: /^Review / })).toHaveLength(1);
  expect(screen.queryByRole('button', { name: 'Review Moon Tortoise' })).not.toBeInTheDocument();
  fireEvent.change(screen.getByLabelText('Search cards'), { target: { value: 'Moon' } });
  await waitFor(() => expect(screen.getByRole('button', { name: 'All 0' })).toBeVisible());
  expect(screen.getByText(/No in-hand cards match/)).toBeVisible();
  click('Inventory'); expect(selectedB()).toBeChecked();
  fireEvent.click(screen.getByTitle('Click to edit list price'));
  expect(screen.getByRole('textbox', { name: 'List price' })).toBeEnabled();
  expect(writes()).toHaveLength(0);
});

it('keeps received evaluation failures visible while scoping the review failure count, without narrowing shared evaluations', async () => {
  unreceived.add(otherId); missingEvaluations = new Set([purchaseId, otherId]); failEvidence = purchaseId;
  mount('/inventory?view=pricing');
  await screen.findByRole('button', { name: 'Unavailable 1' });
  expect(await screen.findByText(/1 evaluations unavailable/)).toBeVisible();
  expect(screen.getByRole('button', { name: 'Review Aurora Dragon' })).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Review Moon Tortoise' })).not.toBeInTheDocument();
  expect(requests.find(r => r.url.endsWith('/evaluate'))?.body).toEqual({ purchaseIds: [purchaseId, otherId] });
  click('Inventory'); expect(screen.getByText(/2 evaluations unavailable/)).toBeVisible();
});

it('rejects an unreceived direct link even with successful detail cached, and explains an empty in-hand cohort', async () => {
  unreceived = new Set([purchaseId, otherId]);
  mount(`/inventory?view=pricing&review=${otherId}`, client => {
    client.setQueryData([...showPrepKeys.evidence(otherId), ''], { evaluation: values[1], sales });
    client.setQueryData([...showPrepKeys.evidence(otherId), values[1].version], { evaluation: values[1], sales });
  });
  expect(await screen.findByText(/No in-hand inventory to review/)).toBeVisible();
  expect(screen.getByText(/Price review is for in-hand inventory only/)).toBeVisible();
  expect(screen.queryByRole('region', { name: 'Price editor' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /^Review / })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'All 0' })).toBeVisible();
  expect(requests.some(r => /evidence\/|preview|review-price/.test(r.url))).toBe(false);
  click('Inventory'); click(/^All\s*2$/);
  expect(screen.getAllByTitle('Awaiting intake')).toHaveLength(2);
  expect(screen.getAllByTitle('Click to edit list price')).toHaveLength(2);
});

it('invalidates the whole show cache family after the canonical reviewed-price write', async () => {
  mount('/inventory?view=pricing'); await ready();
  const keys = [[...showPrepKeys.all, 'price-preview', 'inactive'], [...showPrepKeys.all, 'list', 'inactive'], [...showPrepKeys.all, 'evidence', 'inactive']];
  qc.setQueryData(keys[0], preview()); qc.setQueryData(keys[1], detail([])); qc.setQueryData(keys[2], { evaluation: evaluation(), sales });
  draft('2400'); await saveCurrentTrial(); await screen.findByText(/Saved state rechecked/);
  keys.forEach(key => expect(qc.getQueryState(key)?.isInvalidated).toBe(true));
});
