import { useState } from 'react';
import { afterEach, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ToastProvider } from '../../contexts/ToastContext';
import RecordSaleModal from './RecordSaleModal';
import RecordSaleForm from './RecordSaleForm';
import BulkRecordSaleModal from './BulkRecordSaleModal';
import { evaluation, inventoryItem } from '../show-preparation/fixtures.test-support';

afterEach(() => vi.unstubAllGlobals());
const first = inventoryItem();
const second = inventoryItem(evaluation({ purchaseId: 'other-purchase' }), { campaignId: 'other-campaign' });

it.each([
  { mode: 'modal-dismiss', status: 200 },
  { mode: 'modal-navigation', status: 400 },
  { mode: 'inline-navigation', status: 200 },
  { mode: 'bulk-navigation', status: 200 },
])('settles actual sale requests after $mode only when every response body completes ($status)', async ({ mode, status }) => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const succeeded = vi.fn();
  const bodies: ReadableStreamDefaultController<Uint8Array>[] = [];
  const calls: string[] = [];
  vi.stubGlobal('fetch', async (url: string) => {
    calls.push(url);
    if (url.includes('/sales')) return new Response(new ReadableStream({ start(body) {
      bodies.push(body); body.enqueue(new TextEncoder().encode('{'));
    } }), { status });
    return new Response(JSON.stringify({ evaluations: [evaluation()] }));
  });
  function Editor() {
    const [open, setOpen] = useState(true);
    if (mode === 'inline-navigation') return <RecordSaleForm item={first} onCancel={() => {}} onSuccess={succeeded} />;
    return mode === 'bulk-navigation'
      ? <BulkRecordSaleModal open={open} onClose={() => setOpen(false)} items={[first, second]} onSuccess={succeeded} />
      : <RecordSaleModal open={open} onClose={() => setOpen(false)} items={[first]} onSuccess={succeeded} />;
  }
  const view = render(<QueryClientProvider client={qc}><ToastProvider><Editor /></ToastProvider></QueryClientProvider>);
  const finish = (body: ReadableStreamDefaultController<Uint8Array>) => {
    body.enqueue(new TextEncoder().encode(status === 400 ? '"error":"Sale rejected"}' : mode === 'bulk-navigation' ? '"created":1,"failed":0,"errors":[]}' : '}'));
    body.close();
  };
  try {
    if (mode === 'bulk-navigation') fireEvent.change(screen.getByLabelText('% of CL'), { target: { value: '90' } });
    fireEvent.click(screen.getByRole('button', { name: mode === 'bulk-navigation' ? 'Record 2 Sales' : 'Record Sale' }));
    await waitFor(() => expect(bodies).toHaveLength(mode === 'bulk-navigation' ? 2 : 1));
    if (mode === 'modal-dismiss') {
      fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' });
      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    } else view.unmount();
    expect(succeeded).not.toHaveBeenCalled();
    if (mode === 'bulk-navigation') {
      await act(async () => { finish(bodies.shift()!); });
      expect(succeeded).not.toHaveBeenCalled();
    }
    await act(async () => { finish(bodies.shift()!); });
    await waitFor(() => expect(succeeded).toHaveBeenCalledTimes(status === 200 ? 1 : 0));
    expect(calls.filter(url => url.endsWith('/refresh'))).toHaveLength(0);
    expect(calls.filter(url => url.includes('/sales'))).toHaveLength(mode === 'bulk-navigation' ? 2 : 1);
  } finally {
    await act(async () => { bodies.splice(0).forEach(finish); });
    view.unmount(); qc.clear();
  }
});
