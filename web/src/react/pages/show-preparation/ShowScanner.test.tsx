import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, useNavigate } from 'react-router-dom';
import { AuthProvider, useAuth } from '../../contexts/AuthContext';
import ShowPreparationPage from '../ShowPreparationPage';
import { detail, evaluation, inventoryItem, listId, member, purchaseId } from './fixtures.test-support';
import type { ScanCommand } from '../../../types/showprep';
import { AUTH_CONTEXT_RESET_EVENT } from '../../utils/authContextEvents';

const secondList = '66666666-6666-4666-8666-666666666666';
let requests: { list: string; body: ScanCommand }[];
let gate: Promise<void> | undefined;
let release: () => void;
let uncertain: boolean;
let generation: number;
let removed: boolean;
let listGate: Promise<void> | undefined;
let removeGate: Promise<void> | undefined;
let askingCents: number;
let authFailure: boolean;
let stalledBody: boolean;
function response(command: ScanCommand, destination: string) {
  const rejected = command.withdrawalGeneration !== generation;
  return { original: { ...command, listId: destination, outcome: rejected ? 'rejected' : 'confirmed', code: rejected ? 'contents_changed' : '',
    itemId: rejected ? '' : member().id, purchaseId: rejected ? '' : purchaseId, cardName: rejected ? '' : 'Pikachu',
    confirmedAt: rejected ? '' : '2026-10-09T12:00:00Z', acknowledgedPriceCents: rejected ? 0 : askingCents, acknowledgedStatus: rejected ? '' : askingCents <= 0 ? 'no_listed_price' : 'supported' },
    replay: false, current: { state: rejected ? 'unknown' : 'present', ...(rejected ? {} : { evaluation: evaluation({ certNumber: command.certNumber, localPriceCents: askingCents, status: askingCents <= 0 ? 'no_listed_price' : 'supported' }) }) } };
}
function mount(navigation = false) {
  function Switch() { const navigate = useNavigate(); return <button onClick={() => navigate(`/shows?list=${secondList}`)}>Switch route</button>; }
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  function VerifyIdentity() { const auth = useAuth(); return <button onClick={() => void auth.refetchUser()}>Verify identity</button>; }
  const view = render(<QueryClientProvider client={qc}><AuthProvider><VerifyIdentity /><MemoryRouter initialEntries={[`/shows?list=${listId}`]}>{navigation && <Switch />}<ShowPreparationPage /></MemoryRouter></AuthProvider></QueryClientProvider>);
  return { ...view, qc };
}
function scan(cert: string) {
  const field = screen.getByRole('textbox', { name: 'Scan slab barcode' });
  fireEvent.change(field, { target: { value: cert } }); fireEvent.keyDown(field, { key: 'Enter' }); return field;
}
beforeEach(() => {
  sessionStorage.clear(); requests = []; gate = undefined; uncertain = false; generation = 0; removed = false; listGate = undefined; removeGate = undefined; askingCents = 30000; authFailure = false; stalledBody = false;
  vi.stubGlobal('fetch', vi.fn(async (url: string, options: RequestInit = {}) => {
    if (url.endsWith('/auth/user') && authFailure) return Response.json({ error: 'Unknown identity' }, { status: 500 });
    if (url.endsWith('/auth/user')) return Response.json({ id: 7, username: 'Operator', email: 'operator@example.test', avatar_url: '', is_admin: false, last_login_at: null });
    if (url.endsWith('/inventory')) return Response.json({ items: [inventoryItem()], warnings: [] });
    if (url.endsWith('/evaluate')) return Response.json({ evaluations: [evaluation()] });
    if (url.endsWith('/lists')) return Response.json({ lists: [detail().list, { ...detail().list, id: secondList, name: 'Other show' }] });
    if (url.endsWith('/scan-confirm')) {
      const body = JSON.parse(String(options.body)); const destination = url.split('/lists/')[1].split('/')[0]; requests.push({ list: destination, body });
      await gate;
      if (stalledBody) return new Response(new ReadableStream({ start(stream) { stream.enqueue(new TextEncoder().encode('{')); } }));
      if (uncertain) return Response.json({ error: 'Scan temporarily contended', code: 'scan_contended', retryable: true }, { status: 503 });
      return Response.json(response(body, destination));
    }
    if (options.method === 'DELETE') { await removeGate; generation++; removed = true; return Response.json({ removed: true }); }
    if (url.includes('/lists/')) { await listGate; const data = detail(removed ? [] : [member()]); return Response.json({ ...data, list: { ...data.list, id: url.split('/lists/')[1], withdrawalGeneration: generation } }); }
    return Response.json({ error: `Unexpected ${url}` }, { status: 400 });
  }));
});
afterEach(() => { release?.(); vi.unstubAllGlobals(); vi.restoreAllMocks(); vi.useRealTimers(); });
async function ready() { await waitFor(() => expect(screen.getByRole('textbox', { name: 'Scan slab barcode' })).toBeEnabled()); }

it('Enter immediately persists and sends a scan, accepts consecutive certs, and never offers Add/Pack/Acknowledge', async () => {
  gate = new Promise(resolve => { release = resolve; }); mount(); await ready();
  expect(scan('00123AB')).toHaveFocus(); scan('87654321');
  await waitFor(() => expect(requests).toHaveLength(1));
  expect(requests[0].body).toMatchObject({ certNumber: '00123AB', withdrawalGeneration: 0 });
  expect(sessionStorage.length).toBe(1);
  expect(screen.getByRole('combobox', { name: 'Show list' })).toBeDisabled();
  expect(screen.queryByRole('button', { name: /Add.*to show|Acknowledge/i })).not.toBeInTheDocument();
  expect(screen.queryByRole('checkbox', { name: /Packed/ })).not.toBeInTheDocument();
  release(); await waitFor(() => expect(requests).toHaveLength(2));
  expect(requests[1].body.certNumber).toBe('87654321');
  await waitFor(() => expect(sessionStorage.length).toBe(0));
  expect(await screen.findAllByText('Confirmed')).toHaveLength(2);
});
it('captures a complete barcode during a held post-settlement background detail refetch', async () => {
  const view = mount(); await ready();
  let finishRead!: () => void;
  listGate = new Promise(resolve => { finishRead = resolve; });
  try {
    scan('12345678');
    await screen.findByText('Confirmed');
    await waitFor(() => expect(view.qc.isFetching({ queryKey: ['show-prep', 'list', listId] })).toBeGreaterThan(0));
    // No ready/enabled wait between barcodes: a scanner cannot wait for a read.
    const field = scan('87654321');
    await waitFor(() => expect(requests).toHaveLength(2));
    expect(requests[1].body).toMatchObject({ certNumber: '87654321', withdrawalGeneration: 0 });
    expect(field).toHaveValue('');
  } finally { listGate = undefined; finishRead(); }
});
it('settles valid negative asking confirmation as missing asking instead of retaining it for recovery', async () => {
  askingCents = -30000; mount(); await ready(); scan('12345678');
  expect(await screen.findByText('Confirmed')).toBeVisible();
  expect(screen.getByText('Recorded asking: Missing asking price')).toBeVisible();
  await waitFor(() => expect(sessionStorage.length).toBe(0));
  expect(screen.queryByRole('button', { name: /Retry scan|Recover scan/ })).not.toBeInTheDocument();
});
it('retains an uncertain command and retries exactly its original ID and generation', async () => {
  uncertain = true; mount(); await ready(); scan('12345678');
  expect(await screen.findByText(/Not confirmed.*Retry the same scan/)).toBeVisible();
  expect(requests).toHaveLength(1); expect(screen.getByLabelText('Confirmed ready asking value')).toHaveTextContent('$0.00');
  generation = 1; uncertain = false;
  fireEvent.click(screen.getByRole('button', { name: /Retry scan/ }));
  expect(await screen.findByText(/Show contents changed after this scan; rescan to confirm/)).toBeVisible();
  expect(requests[1]).toEqual(requests[0]);
});
it('reload restores only for explicit recovery with an immutable destination and payload', async () => {
  uncertain = true; const first = mount(); await ready(); scan('12345678');
  await screen.findByText(/Retry the same scan/); first.unmount();
  uncertain = false; generation = 8; mount(); await ready();
  expect(requests).toHaveLength(1); expect(await screen.findByRole('button', { name: /Recover scan/ })).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: /Recover scan/ }));
  await waitFor(() => expect(requests).toHaveLength(2)); expect(requests[1]).toEqual(requests[0]);
});
it('a stalled response becomes uncertain, preserves recovery, and retries the same payload', async () => {
  mount(); await ready(); stalledBody = true; vi.useFakeTimers();
  scan('12345678');
  await act(async () => { await vi.advanceTimersByTimeAsync(30001); });
  vi.useRealTimers();
  expect(await screen.findByRole('button', { name: 'Retry scan 12345678' })).toBeEnabled();
  const captured = JSON.parse(sessionStorage.getItem(sessionStorage.key(0)!)!).scans[0].command;
  expect(captured).toEqual(requests[0].body);
  stalledBody = false;
  fireEvent.click(screen.getByRole('button', { name: 'Retry scan 12345678' }));
  await screen.findByText('Confirmed');
  expect(requests[1]).toEqual(requests[0]);
  await waitFor(() => expect(sessionStorage.length).toBe(0));
});
it('blocks authenticated actions on transient unknown identity, then restores only explicit same-user recovery', async () => {
  uncertain = true; mount(); await ready(); scan('12345678');
  await screen.findByRole('button', { name: 'Retry scan 12345678' });
  const captured = sessionStorage.getItem(sessionStorage.key(0)!)!;
  authFailure = true;
  fireEvent.click(screen.getByRole('button', { name: 'Verify identity' }));
  await waitFor(() => expect(screen.getByRole('textbox', { name: 'Scan slab barcode' })).toBeDisabled());
  await act(async () => { await new Promise(resolve => setTimeout(resolve, 3200)); });
  expect(sessionStorage.getItem(sessionStorage.key(0)!)).toEqual(captured);
  expect(screen.queryByRole('button', { name: /Retry scan|Recover scan/ })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'New list' })).toBeDisabled();
  expect(screen.getByRole('button', { name: /Remove.*from show/ })).toBeDisabled();
  authFailure = false; uncertain = false;
  fireEvent.click(screen.getByRole('button', { name: 'Verify identity' }));
  expect(await screen.findByRole('button', { name: 'Recover scan 12345678' })).toBeEnabled();
  expect(requests).toHaveLength(1);
  fireEvent.click(screen.getByRole('button', { name: 'Recover scan 12345678' }));
  await screen.findByText('Confirmed');
  expect(requests[1]).toEqual(requests[0]);
});
it('navigation preserves the old-show owner but does not send its queued scans or publish under the new show', async () => {
  gate = new Promise(resolve => { release = resolve; }); mount(true); await ready(); scan('12345678'); scan('87654321');
  await waitFor(() => expect(requests).toHaveLength(1)); fireEvent.click(screen.getByRole('button', { name: 'Switch route' }));
  await waitFor(() => expect(screen.getByRole('combobox', { name: 'Show list' })).toHaveValue(secondList));
  release(); await screen.findByText('Confirmed'); expect(requests).toHaveLength(1);
  expect(screen.getByRole('combobox', { name: 'Show list' })).toBeDisabled();
  expect(within(screen.getByRole('region', { name: 'Recent scans' })).getAllByText(new RegExp(listId))).not.toHaveLength(0);
});
it('continues fresh current-show scans after explicit old-show recovery without automatically recovering other old scans', async () => {
  gate = new Promise(resolve => { release = resolve; }); mount(true); await ready();
  scan('12345678'); scan('87654321'); scan('00123AB');
  await waitFor(() => expect(requests).toHaveLength(1));
  const captured = JSON.parse(sessionStorage.getItem(sessionStorage.key(0)!)!).scans;
  fireEvent.click(screen.getByRole('button', { name: 'Switch route' }));
  await waitFor(() => expect(screen.getByRole('combobox', { name: 'Show list' })).toHaveValue(secondList)); await ready();
  scan('99999999');
  const currentCommand = JSON.parse(sessionStorage.getItem(sessionStorage.key(0)!)!).scans[3];
  release(); await screen.findByRole('button', { name: 'Recover scan 87654321' }); expect(requests).toHaveLength(1);
  fireEvent.click(screen.getByRole('button', { name: 'Recover scan 87654321' }));
  await waitFor(() => expect(requests).toHaveLength(2));
  expect(requests[1]).toEqual({ list: listId, body: captured[1].command });
  await waitFor(() => expect(screen.getByRole('button', { name: 'Recover scan 00123AB' })).toBeEnabled());
  expect(requests).toHaveLength(2); // The remaining old-show row still requires explicit consent to recover.
  fireEvent.click(screen.getByRole('button', { name: 'Recover scan 00123AB' }));
  await waitFor(() => expect(requests).toHaveLength(4));
  expect(requests[2]).toEqual({ list: listId, body: captured[2].command });
  expect(requests[3]).toEqual({ list: secondList, body: currentCommand.command });
  await waitFor(() => expect(sessionStorage.length).toBe(0));
  expect(screen.getByRole('combobox', { name: 'Show list' })).toBeEnabled();
});
it('unmount never automatically sends queued work', async () => {
  gate = new Promise(resolve => { release = resolve; }); const view = mount(); await ready(); scan('12345678'); scan('87654321');
  await waitFor(() => expect(requests).toHaveLength(1)); view.unmount(); release(); await act(async () => { await Promise.resolve(); });
  expect(requests).toHaveLength(1); expect(sessionStorage.length).toBe(1);
});
it('blocks future captures until the post-removal read, without rebasing queued commands', async () => {
  gate = new Promise(resolve => { release = resolve; }); mount(); await ready(); scan('12345678'); scan('87654321');
  await waitFor(() => expect(requests).toHaveLength(1));
  let releaseList!: () => void; listGate = new Promise(resolve => { releaseList = resolve; });
  fireEvent.click(screen.getByRole('button', { name: 'Remove 12345678 from show' }));
  await waitFor(() => expect(screen.getByRole('textbox', { name: 'Scan slab barcode' })).toBeDisabled());
  release(); await waitFor(() => expect(requests).toHaveLength(2)); expect(requests[1].body.withdrawalGeneration).toBe(0);
  listGate = undefined; releaseList(); await ready(); scan('99999999');
  await waitFor(() => expect(requests).toHaveLength(3)); expect(requests[2].body.withdrawalGeneration).toBe(1);
});
it('pauses captures immediately while a remove request is unresolved', async () => {
  let finishRemove!: () => void; removeGate = new Promise(resolve => { finishRemove = resolve; }); mount(); await ready();
  fireEvent.click(screen.getByRole('button', { name: 'Remove 12345678 from show' }));
  await waitFor(() => expect(screen.getByRole('textbox', { name: 'Scan slab barcode' })).toBeDisabled());
  finishRemove(); await ready();
});
it('visibly refuses a barcode when recovery cannot be persisted, before sending', async () => {
  mount(); await ready(); vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('Quota exceeded'); }); scan('12345678');
  expect(await screen.findByRole('alert')).toHaveTextContent(/recovery.*storage/i); expect(requests).toHaveLength(0);
});
it('auth reset discards in-flight owners and prevents queued calls or stale completion publication', async () => {
  gate = new Promise(resolve => { release = resolve; }); mount(); await ready(); scan('12345678'); scan('87654321');
  await waitFor(() => expect(requests).toHaveLength(1));
  act(() => window.dispatchEvent(new Event(AUTH_CONTEXT_RESET_EVENT))); release();
  await act(async () => { await Promise.resolve(); });
  expect(requests).toHaveLength(1); expect(sessionStorage.length).toBe(0);
  expect(screen.queryByRole('region', { name: 'Recent scans' })).not.toBeInTheDocument();
  expect(screen.getByRole('textbox', { name: 'Scan slab barcode' })).toBeDisabled();
});
it('never silently drops the 201st unsettled barcode', async () => {
  gate = new Promise(resolve => { release = resolve; }); mount(); await ready();
  for (let i = 0; i < 200; i++) scan(String(i)); scan('overflow');
  expect(await screen.findByRole('alert')).toHaveTextContent(/200.*unsettled/i);
  expect(JSON.parse(sessionStorage.getItem(sessionStorage.key(0)!)!).scans).toHaveLength(200);
});
