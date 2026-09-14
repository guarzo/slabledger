import { afterEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import ShowListPicker from './ShowListPicker';
import { detail } from './fixtures.test-support';

afterEach(() => vi.unstubAllGlobals());

it.each(['unavailable', 'throwing'])('shows a recoverable creation error when randomUUID is %s', async failure => {
  const nativeCrypto = globalThis.crypto;
  const requests: string[] = [];
  vi.stubGlobal('crypto', failure === 'unavailable' ? {} : { randomUUID: () => { throw new Error('Secure UUID generation failed'); } });
  vi.stubGlobal('fetch', vi.fn(async (_url: string, options: RequestInit = {}) => {
    requests.push(options.method || 'GET');
    if (options.method !== 'POST') return new Response(JSON.stringify({ lists: [] }));
    return new Response(JSON.stringify({ ...detail().list, ...JSON.parse(String(options.body)) }));
  }));
  const onChange = vi.fn();
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><ShowListPicker value="" onChange={onChange} /></QueryClientProvider>);
  fireEvent.change(screen.getByLabelText('New show name'), { target: { value: 'Retry securely' } });
  fireEvent.click(screen.getByRole('button', { name: 'Create show list' }));
  expect(await screen.findByRole('alert')).not.toBeEmptyDOMElement();
  expect(screen.getByLabelText('New show name')).toHaveValue('Retry securely');
  expect(screen.getByRole('button', { name: 'Create show list' })).toBeEnabled();
  expect(onChange).not.toHaveBeenCalled();
  expect(requests).not.toContain('POST');
  vi.stubGlobal('crypto', nativeCrypto);
  fireEvent.click(screen.getByRole('button', { name: 'Create show list' }));
  await waitFor(() => expect(screen.getByLabelText('New show name')).toHaveValue(''));
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  expect(onChange).toHaveBeenCalledOnce();
});
it('reuses the caller UUID when an operator retries the same failed creation', async () => {
  const attempts: { id: string; name: string }[] = [];
  vi.stubGlobal('fetch', vi.fn(async (_url, options) => {
    if (options.method !== 'POST') return new Response(JSON.stringify({ lists: [] }));
    const input = JSON.parse(options.body); attempts.push(input);
    if (attempts.length === 1) return new Response(JSON.stringify({ error: 'Temporary conflict' }), { status: 409 });
    return new Response(JSON.stringify({ ...detail().list, ...input }));
  }));
  const onChange = vi.fn();
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><ShowListPicker value="" onChange={onChange} /></QueryClientProvider>);
  fireEvent.change(screen.getByLabelText('New show name'), { target: { value: '  Retry show  ' } });
  fireEvent.click(screen.getByRole('button', { name: 'Create show list' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Temporary conflict');
  expect(onChange).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: 'Create show list' }));
  await waitFor(() => expect(onChange).toHaveBeenCalledWith(attempts[0].id));
  expect(attempts).toHaveLength(2);
  expect(attempts[1]).toEqual(attempts[0]);
  expect(attempts[0].name).toBe('Retry show');
});
