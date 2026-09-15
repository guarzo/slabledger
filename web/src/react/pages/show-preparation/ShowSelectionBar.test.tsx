import { useState } from 'react';
import { afterEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import InventorySelectionBar from '../campaign-detail/inventory/InventorySelectionBar';
import { detail, evaluation, inventoryItem, listId, purchaseId } from './fixtures.test-support';

const otherId = '66666666-6666-4666-8666-666666666666';
const add = () => screen.getByRole('button', { name: 'Add to show (1)' });
const bar = () => screen.queryByRole('region', { name: 'Bulk actions for selected cards' });
afterEach(() => vi.unstubAllGlobals());
function mount({ count = 1, disabled = false, version = 'eval-1', outsideView = 0, fetcher = undefined as typeof fetch | undefined } = {}) {
  const calls: { url: string; body: unknown }[] = [];
  vi.stubGlobal('fetch', fetcher ?? vi.fn(async (url: string, options: RequestInit = {}) => {
    calls.push({ url, body: options.body ? JSON.parse(String(options.body)) : undefined });
    return new Response(JSON.stringify(url.endsWith('/lists') ? { lists: [detail().list, { ...detail().list, id: otherId, name: 'October show' }] } : detail()));
  }));
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  function Harness() {
    const [selected, setSelected] = useState(new Set(count ? [purchaseId] : []));
    return <><button onClick={() => setSelected(new Set([purchaseId]))}>Select again</button><InventorySelectionBar
      selectedItems={selected.size ? [inventoryItem()] : []} selected={selected} selectedVersions={{ [purchaseId]: version }} evaluations={{ [purchaseId]: evaluation() }}
      disabled={disabled} onClear={() => setSelected(new Set())} onAdded={() => setSelected(new Set())}
      onRecordSale={() => {}} onListOnDH={() => {}} outsideView={outsideView} onReveal={() => {}} /></>;
  }
  render(<QueryClientProvider client={qc}><MemoryRouter><Harness /></MemoryRouter></QueryClientProvider>);
  return calls;
}
it('has no action bar or destination form until cards are selected', () => {
  const calls = mount({ count: 0 }); expect(bar()).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Show list')).not.toBeInTheDocument(); expect(calls).toEqual([]);
});
it('reveals destination then creation progressively, and Escape backs out before clearing', async () => {
  mount(); expect(screen.queryByLabelText('Show list')).not.toBeInTheDocument();
  fireEvent.click(add()); expect(await screen.findByLabelText('Show list')).toBeEnabled();
  expect(screen.queryByLabelText('New show name')).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'New list' }));
  fireEvent.keyDown(screen.getByLabelText('New show name'), { key: 'Escape' });
  expect(screen.queryByLabelText('New show name')).not.toBeInTheDocument();
  expect(screen.getByLabelText('Show list')).toBeVisible();
  fireEvent.keyDown(window, { key: 'Escape' });
  expect(screen.queryByRole('combobox', { name: 'Show list' })).not.toBeInTheDocument(); expect(add()).toHaveFocus();
  fireEvent.keyDown(window, { key: 'Escape' }); expect(bar()).not.toBeInTheDocument();
});
it('keeps success at its submitted destination after clearing and choosing a different list', async () => {
  const calls = mount(); fireEvent.click(add());
  fireEvent.change(await screen.findByLabelText('Show list'), { target: { value: listId } }); fireEvent.click(add());
  const link = await screen.findByRole('link', { name: 'Open packing list →' }); expect(link).toHaveAttribute('href', `/shows?list=${listId}`);
  expect(bar()).not.toBeInTheDocument();
  expect(calls.find(call => call.url.endsWith('/items'))?.body).toEqual({ items: [{ purchaseId, evaluationVersion: 'eval-1' }] });
  fireEvent.click(screen.getByRole('button', { name: 'Select again' })); fireEvent.click(add());
  expect(link).toBeVisible();
  fireEvent.change(await screen.findByLabelText('Show list'), { target: { value: otherId } });
  expect(screen.queryByRole('link', { name: 'Open packing list →' })).not.toBeInTheDocument();
});
it('retains the creation UUID when a failed destination is closed and reopened for retry', async () => {
  const attempts: { id: string; name: string }[] = [];
  mount({ fetcher: vi.fn(async (_url, options: RequestInit = {}) => {
    if (options.method !== 'POST') return Response.json({ lists: [] });
    attempts.push(JSON.parse(String(options.body))); return Response.json({ error: 'Temporary conflict' }, { status: 409 });
  }) });
  fireEvent.click(add()); fireEvent.change(await screen.findByLabelText('New show name'), { target: { value: 'Retry same destination' } });
  fireEvent.click(screen.getByRole('button', { name: 'Create show list' })); await screen.findByRole('alert');
  fireEvent.click(screen.getByRole('button', { name: 'Close destination' })); fireEvent.click(add());
  expect(await screen.findByLabelText('New show name')).toHaveValue('Retry same destination');
  fireEvent.click(screen.getByRole('button', { name: 'Create show list' }));
  await waitFor(() => expect(attempts).toHaveLength(2)); expect(attempts[1]).toEqual(attempts[0]);
});
it.each(['modal', 'dialog', 'prevented'])('retains selection when Escape belongs to %s', priority => {
  mount({ disabled: priority === 'modal' });
  let dialog: HTMLDivElement | undefined;
  if (priority === 'dialog') { dialog = document.createElement('div'); dialog.setAttribute('role', 'dialog'); document.body.append(dialog); }
  const event = new KeyboardEvent('keydown', { key: 'Escape', cancelable: true });
  if (priority === 'prevented') event.preventDefault(); fireEvent(window, event);
  expect(bar()).toBeVisible(); dialog?.remove();
});
it('keeps changed and hidden selections identifiable and non-addable', () => {
  mount({ version: 'old', outsideView: 1 });
  expect(screen.getByText(/Review and reselect/)).toHaveTextContent('12345678');
  expect(screen.getByText(/1 selected outside this view/)).toBeVisible();
  expect(screen.getByRole('button', { name: 'Reveal selected' })).toBeEnabled(); expect(add()).toBeDisabled();
});
