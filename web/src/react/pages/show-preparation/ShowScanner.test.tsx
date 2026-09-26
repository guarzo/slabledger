import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, useNavigate } from 'react-router-dom';
import ShowPreparationPage from '../ShowPreparationPage';
import { detail, evaluation, inventoryItem, listId, member, purchaseId } from './fixtures.test-support';

const queryFailure = vi.hoisted(() => ({ enabled: false }));
vi.mock('../../../js/api/showprep', async importOriginal => {
  const actual = await importOriginal<typeof import('../../../js/api/showprep')>();
  return { ...actual, evaluateInventory: (...args: Parameters<typeof actual.evaluateInventory>) =>
    queryFailure.enabled ? Promise.reject(new Error('Evaluation read failed')) : actual.evaluateInventory(...args) };
});

const secondId = '44444444-4444-4444-8444-444444444444';
const secondListId = '66666666-6666-4666-8666-666666666666';
let writes: { items: { purchaseId: string; evaluationVersion: string }[] }[];
let version = 'eval-1';
let conflict = false;
let existingMember = true;
let ambiguous = false;
let listedOnly = false;
let evaluationFails = false;
let mismatchedCert = false;
let collision = false;
let blockAdd = false;
let releaseAdd: (() => void) | undefined;
let partialResponse = false;
const evaluations = () => [evaluation({ version, certNumber: mismatchedCert ? '87654321' : '12345678' }), evaluation({ purchaseId: secondId, certNumber: '87654321', cardName: 'Charizard', version: version === 'eval-1' ? 'eval-2' : version })];
function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={qc}><MemoryRouter initialEntries={[`/shows?list=${listId}`]}><ShowPreparationPage /></MemoryRouter></QueryClientProvider>);
  return qc;
}
function scan(cert: string) {
  const field = screen.getByRole('textbox', { name: 'Scan slab barcode' });
  fireEvent.change(field, { target: { value: cert } });
  fireEvent.keyDown(field, { key: 'Enter' });
  return field;
}
beforeEach(() => {
  writes = []; version = 'eval-1'; conflict = false; existingMember = true; ambiguous = false; listedOnly = false; evaluationFails = false; mismatchedCert = false; collision = false; blockAdd = false; releaseAdd = undefined; queryFailure.enabled = false; partialResponse = false;
  vi.stubGlobal('fetch', vi.fn(async (url: string, options: RequestInit = {}) => {
    if (url.endsWith('/api/inventory')) return Response.json({ items: [...evaluations().filter(e => !listedOnly || e.purchaseId !== purchaseId).map(e => inventoryItem(e, e.purchaseId === purchaseId ? { certNumber: '12345678' } : collision ? { certNumber: '12345678' } : {})), ...(ambiguous ? [inventoryItem(evaluations()[1], { certNumber: '12345678' })] : [])], warnings: [] });
    if (url.endsWith('/evaluate')) return evaluationFails ? Response.json({ error: 'Evaluation unavailable' }, { status: 400 }) : Response.json({ evaluations: evaluations() });
    if (url.endsWith('/items') && options.method === 'POST') {
      writes.push(JSON.parse(String(options.body)));
      if (blockAdd) await new Promise<void>(resolve => { releaseAdd = resolve; });
      const confirmed = detail([
        ...(existingMember ? [member()] : []),
        ...writes[writes.length - 1].items.map(item => member({
          id: item.purchaseId, purchaseId: item.purchaseId,
          certNumber: item.purchaseId === purchaseId ? '12345678' : '87654321',
        })),
      ]);
      return conflict ? Response.json({ error: 'Evaluation changed. Review current data.' }, { status: 409 })
        : Response.json(partialResponse ? detail() : confirmed);
    }
    if (url.endsWith('/lists')) return Response.json({ lists: [detail().list] });
    if (url.endsWith(`/lists/${listId}`)) return Response.json(detail(existingMember ? undefined : []));
    if (url.endsWith(`/lists/${secondListId}`)) return Response.json({ ...detail([]), list: { ...detail().list, id: secondListId } });
    return Response.json({ error: `Unexpected ${url}` }, { status: 400 });
  }));
});
afterEach(() => vi.unstubAllGlobals());

it('focuses the scan field once inventory and the list finish loading', async () => {
  mount();
  const field = screen.getByRole('textbox', { name: 'Scan slab barcode' });
  await waitFor(() => expect(field).toBeEnabled());
  expect(field).toHaveFocus();
});

it('accepts consecutive scanner enters without saving until review, using observed evaluation versions', async () => {
  existingMember = false; mount(); await screen.findByRole('textbox', { name: 'Scan slab barcode' });
  const field = scan('12345678');
  expect(field).toHaveFocus();
  scan('87654321');
  expect(await screen.findByText('Charizard')).toBeVisible();
  expect(writes).toHaveLength(0);
  fireEvent.click(await screen.findByRole('button', { name: 'Add 2 to show' }));
  await waitFor(() => expect(writes).toEqual([{ items: [
    { purchaseId, evaluationVersion: 'eval-1' }, { purchaseId: secondId, evaluationVersion: 'eval-2' },
  ] }]));
  expect(await screen.findByText('2 cards confirmed on show list.')).toBeVisible();
  expect(screen.queryByRole('heading', { name: /Review scans/ })).not.toBeInTheDocument();
});

it('keeps unmatched, repeated and already-listed scans out of the add payload', async () => {
  mount(); await screen.findByRole('textbox', { name: 'Scan slab barcode' });
  scan('99999999'); scan('12345678'); scan('12345678'); scan('87654321');
  expect(await screen.findByText('Charizard')).toBeVisible();
  expect(screen.getByText(/No inventory match/)).toBeVisible();
  expect(screen.getByText(/Already on this show list/)).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Add 1 to show' }));
  await waitFor(() => expect(writes).toEqual([{ items: [{ purchaseId: secondId, evaluationVersion: 'eval-2' }] }]));
});

it('lets the operator retry failed evaluation reads without rescanning', async () => {
  evaluationFails = true; existingMember = false; mount(); await screen.findByRole('textbox', { name: 'Scan slab barcode' });
  scan('87654321');
  const retry = await screen.findByRole('button', { name: 'Retry evaluations' });
  expect(screen.getByRole('button', { name: 'Add 0 to show' })).toBeDisabled();
  evaluationFails = false;
  fireEvent.click(retry);
  expect(await screen.findByRole('button', { name: 'Add 1 to show' })).toBeEnabled();
});

it('offers evaluation retry when the query fails before returning per-card errors', async () => {
  queryFailure.enabled = true; existingMember = false; mount(); await screen.findByRole('textbox', { name: 'Scan slab barcode' });
  scan('87654321');
  const retry = await screen.findByRole('button', { name: 'Retry evaluations' });
  expect(screen.getByText('Evaluation read failed')).toBeVisible();
  queryFailure.enabled = false;
  fireEvent.click(retry);
  expect(await screen.findByRole('button', { name: 'Add 1 to show' })).toBeEnabled();
});

it('blocks cached evaluation values when a refresh fails, until retry succeeds', async () => {
  existingMember = false; const qc = mount(); await screen.findByRole('textbox', { name: 'Scan slab barcode' });
  scan('87654321');
  await screen.findByRole('button', { name: 'Add 1 to show' });
  queryFailure.enabled = true;
  await act(async () => { await qc.invalidateQueries({ queryKey: ['show-prep', 'evaluations'] }); });
  expect(await screen.findByText('Evaluation read failed')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Add 0 to show' })).toBeDisabled();
  queryFailure.enabled = false;
  fireEvent.click(screen.getByRole('button', { name: 'Retry evaluations' }));
  expect(await screen.findByRole('button', { name: 'Add 1 to show' })).toBeEnabled();
});

it('recognizes an already-listed cert even when no longer in unsold inventory', async () => {
  listedOnly = true; mount(); await screen.findByRole('textbox', { name: 'Scan slab barcode' });
  scan('12345678');
  expect(screen.getByText('Already on this show list')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Add 0 to show' })).toBeDisabled();
});

it('does not offer an add when evaluated cert differs from scanned identity', async () => {
  mismatchedCert = true; existingMember = false; mount(); await screen.findByRole('textbox', { name: 'Scan slab barcode' });
  scan('12345678');
  expect(await screen.findByText(/Identity changed/)).toBeVisible();
  expect(screen.getByRole('button', { name: 'Add 0 to show' })).toBeDisabled();
});

it('does not guess between a saved member and a different inventory purchase with the same cert', async () => {
  collision = true; listedOnly = true; mount(); await screen.findByRole('textbox', { name: 'Scan slab barcode' });
  scan('12345678');
  expect(screen.getByText('Multiple inventory matches')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Add 0 to show' })).toBeDisabled();
});

it('shows ambiguous inventory matches without guessing a purchase', async () => {
  ambiguous = true; existingMember = false; mount(); await screen.findByRole('textbox', { name: 'Scan slab barcode' });
  scan('12345678');
  expect(screen.getByText('Multiple inventory matches')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Add 0 to show' })).toBeDisabled();
});

it('blocks a changed evaluation until the operator removes and rescans', async () => {
  existingMember = false;
  const qc = mount(); await screen.findByRole('textbox', { name: 'Scan slab barcode' });
  scan('87654321');
  await screen.findByRole('button', { name: 'Add 1 to show' });
  version = 'eval-3';
  await act(async () => { await qc.invalidateQueries({ queryKey: ['show-prep', 'evaluations'] }); });
  expect(await screen.findByText(/Evaluation changed\. Remove and rescan/)).toBeVisible();
  expect(screen.getByRole('button', { name: 'Add 0 to show' })).toBeDisabled();
  fireEvent.click(screen.getByRole('button', { name: 'Remove scan 87654321' }));
  scan('87654321');
  fireEvent.click(await screen.findByRole('button', { name: 'Add 1 to show' }));
  await waitFor(() => expect(writes).toEqual([{ items: [{ purchaseId: secondId, evaluationVersion: 'eval-3' }] }]));
});

it('keeps scans whose memberships are absent from a successful add response', async () => {
  partialResponse = true; existingMember = false; mount(); await screen.findByRole('textbox', { name: 'Scan slab barcode' });
  scan('12345678'); scan('87654321');
  fireEvent.click(await screen.findByRole('button', { name: 'Add 2 to show' }));
  expect(await screen.findByRole('alert')).toHaveTextContent(/1 card was not confirmed on the show list/i);
  expect(screen.getByText(/1 card confirmed on show list/i)).toBeVisible();
  expect(screen.getByText('Charizard')).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Remove scan 12345678' })).not.toBeInTheDocument();
});

it('prevents switching lists while an add is in flight', async () => {
  blockAdd = true; mount(); await screen.findByRole('textbox', { name: 'Scan slab barcode' });
  scan('87654321');
  fireEvent.click(await screen.findByRole('button', { name: 'Add 1 to show' }));
  expect(screen.getByRole('combobox', { name: 'Show list' })).toBeDisabled();
  await waitFor(() => expect(releaseAdd).toBeTypeOf('function'));
  releaseAdd?.();
  await waitFor(() => expect(screen.getByRole('combobox', { name: 'Show list' })).toBeEnabled());
});

it('releases picker pending state if browser navigation replaces the scanner', async () => {
  function SwitchRoute() { const navigate = useNavigate(); return <button onClick={() => navigate(`/shows?list=${secondListId}`)}>Switch route</button>; }
  blockAdd = true;
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={qc}><MemoryRouter initialEntries={[`/shows?list=${listId}`]}><SwitchRoute /><ShowPreparationPage /></MemoryRouter></QueryClientProvider>);
  await screen.findByRole('textbox', { name: 'Scan slab barcode' });
  scan('87654321');
  fireEvent.click(await screen.findByRole('button', { name: 'Add 1 to show' }));
  expect(screen.getByRole('combobox', { name: 'Show list' })).toBeDisabled();
  await waitFor(() => expect(releaseAdd).toBeTypeOf('function'));
  fireEvent.click(screen.getByRole('button', { name: 'Switch route' }));
  await waitFor(() => expect(screen.getByRole('combobox', { name: 'Show list' })).toBeEnabled());
  releaseAdd?.();
});

it('retains the queue on an add conflict', async () => {
  conflict = true; mount(); await screen.findByRole('textbox', { name: 'Scan slab barcode' });
  scan('87654321');
  await screen.findByRole('button', { name: 'Add 1 to show' });
  fireEvent.click(screen.getByRole('button', { name: 'Add 1 to show' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Evaluation changed');
  expect(screen.getByText('Charizard')).toBeVisible();
  expect(writes).toHaveLength(1);
});
