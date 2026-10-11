import { beforeEach, afterEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, useLocation, useNavigate } from 'react-router-dom';
import { AuthProvider, useAuth } from './AuthContext';
import ErrorBoundary from '../ErrorBoundary';
import { useShowScanCommands } from '../pages/show-preparation/useShowScanCommands';
import { readPendingScans, writePendingScans } from '../pages/show-preparation/showScanRecovery';
import { listId } from '../pages/show-preparation/fixtures.test-support';

const operator = { id: 7, username: 'Operator A', email: 'a@example.test', avatar_url: '', is_admin: false, last_login_at: null };
const pending = [{ listId, command: { commandId: listId, certNumber: '00123AB', withdrawalGeneration: 0 } }];
let identity: number;
let unknown: boolean;
let writes: string[];

function Recovery() {
  const { user, loading } = useAuth();
  const owner = useShowScanCommands(listId, user ? String(user.id) : '');
  return <>
    <output aria-label="Accepted user">{loading ? 'Loading' : user?.id ?? 'Unknown'}</output>
    <output aria-label="Recovery count">{owner.pending}</output>
    {owner.activity.map(row => <button key={row.command.commandId} onClick={() => owner.retry(row.command.commandId)}>Recover scan</button>)}
  </>;
}
// App's pathname key remounts both auth and its lazy feature owners.
function KeyedRoutes() {
  const location = useLocation(); const navigate = useNavigate();
  return <>
    <button onClick={() => navigate('/inventory')}>Inventory</button>
    <button onClick={() => navigate('/shows')}>Shows</button>
    <ErrorBoundary key={location.pathname}><AuthProvider><Recovery /></AuthProvider></ErrorBoundary>
  </>;
}
function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><MemoryRouter initialEntries={['/shows']}><KeyedRoutes /></MemoryRouter></QueryClientProvider>);
}
async function accepted(id: number | string) {
  await waitFor(() => expect(screen.getByLabelText('Accepted user')).toHaveTextContent(String(id)));
}
beforeEach(() => {
  sessionStorage.clear(); identity = 7; unknown = false; writes = [];
  vi.stubGlobal('fetch', vi.fn(async (url: string, options: RequestInit = {}) => {
    if (options.method && options.method !== 'GET') writes.push(url);
    if (url.endsWith('/auth/user')) return unknown ? Response.json({ error: 'Identity unknown' }, { status: 400 }) : Response.json({ ...operator, id: identity });
    return Response.json({ error: 'Unexpected request' }, { status: 400 });
  }));
});
afterEach(() => vi.unstubAllGlobals());

it('discards A recovery on a confirmed B pathname remount, including later return to A, with no auto-send', async () => {
  writePendingScans('7', pending);
  mount(); await accepted(7);
  expect(await screen.findByRole('button', { name: 'Recover scan' })).toBeVisible();
  expect(screen.getByLabelText('Recovery count')).toHaveTextContent('1');
  expect(writes).toEqual([]);
  identity = 8; fireEvent.click(screen.getByRole('button', { name: 'Inventory' })); await accepted(8);
  expect(readPendingScans('7')).toEqual([]);
  expect(sessionStorage.length).toBe(0);
  expect(screen.getByLabelText('Recovery count')).toHaveTextContent('0');
  expect(writes).toEqual([]);
  identity = 7; fireEvent.click(screen.getByRole('button', { name: 'Shows' })); await accepted(7);
  expect(readPendingScans('7')).toEqual([]);
  expect(screen.queryByRole('button', { name: 'Recover scan' })).not.toBeInTheDocument();
  expect(writes).toEqual([]);
});
it.each([false, true])('preserves same-A explicit recovery across remounts, with unknown identity %s and no auto-send', async transientUnknown => {
  mount(); await accepted(7); writePendingScans('7', pending);
  unknown = transientUnknown;
  fireEvent.click(screen.getByRole('button', { name: 'Inventory' })); await accepted(transientUnknown ? 'Unknown' : 7);
  expect(readPendingScans('7')).toEqual(pending);
  unknown = false;
  fireEvent.click(screen.getByRole('button', { name: 'Shows' })); await accepted(7);
  expect(await screen.findByRole('button', { name: 'Recover scan' })).toBeVisible();
  expect(readPendingScans('7')).toEqual(pending);
  expect(writes).toEqual([]);
});
