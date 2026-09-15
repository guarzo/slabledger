import { afterEach, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ToastProvider } from './contexts/ToastContext';
import PriceOverrideDialog from './PriceOverrideDialog';
import PriceHintDialog from './PriceHintDialog';
import FixDHMatchDialog from './pages/campaign-detail/inventory/FixDHMatchDialog';
afterEach(() => vi.unstubAllGlobals());
const operations = [
  { kind: 'override', button: 'Save Price', input: 'Override Price ($)', value: '400' },
  { kind: 'override', button: 'Clear Override' },
  { kind: 'override', button: 'Accept' },
  { kind: 'override', button: 'Dismiss' },
  { kind: 'hint', button: 'Save Hint', input: 'External ID', value: '1234' },
  { kind: 'match', button: 'Update Match', input: 'DH product URL', value: 'https://doubleholo.com/card/1234' },
];
it.each(operations.flatMap(operation => [200, 400].map(status => ({ ...operation, status }))))('settles $kind/$button after unmount and full $status response body', async ({ kind, button, input, value, status }) => {
  const qc = new QueryClient();
  let body!: ReadableStreamDefaultController<Uint8Array>;
  const fetcher = vi.fn(async () => new Response(new ReadableStream({ start(s) { body = s; s.enqueue(new TextEncoder().encode('{')); } }), { status }));
  vi.stubGlobal('fetch', fetcher);
  const saved = vi.fn(); const close = vi.fn();
  const props = { cardName: 'Slab', purchaseId: 'id-1', onSaved: saved, onClose: close };
  const view = render(<QueryClientProvider client={qc}><ToastProvider>{kind === 'override'
    ? <PriceOverrideDialog {...props} costBasisCents={20000} currentPriceCents={30000} currentOverrideCents={40000} aiSuggestedCents={35000} />
    : kind === 'hint' ? <PriceHintDialog {...props} setName="Set" cardNumber="1" /> : <FixDHMatchDialog {...props} />}</ToastProvider></QueryClientProvider>);
  if (input) fireEvent.change(screen.getByLabelText(input), { target: { value } });
  const submit = screen.getByRole('button', { name: button });
  fireEvent.click(submit);
  await waitFor(() => expect(fetcher).toHaveBeenCalledOnce());
  expect(submit).toBeDisabled();
  expect(saved).not.toHaveBeenCalled();
  view.unmount();
  await act(async () => { body.enqueue(new TextEncoder().encode(status === 200 ? '"dhCardId":1234,"status":"ok"}' : '"error":"rejected"}')); body.close(); });
  await waitFor(() => expect(close).toHaveBeenCalledTimes(status === 200 ? 1 : 0));
  expect(saved).toHaveBeenCalledTimes(status === 200 ? 1 : 0);
  expect(fetcher).toHaveBeenCalledOnce();
  qc.clear();
});
