import { createServer, type ServerResponse } from 'node:http';
import { once } from 'node:events';
import { afterEach, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import type { InventoryEvaluations } from '../../js/api/showprep';
import { showPrepKeys, useShowEvidence } from './useShowPrepQueries';
import { detail, evaluation, listId, member, purchaseId } from '../pages/show-preparation/fixtures.test-support';

afterEach(() => vi.unstubAllGlobals());

it('cannot publish or invalidate from a cancelled evidence read whose real HTTP body finishes late', async () => {
  let stream!: ServerResponse;
  const server = createServer((_request, response) => {
    stream = response;
    response.writeHead(200, { 'Content-Type': 'application/json' });
    response.write('{"evaluation":');
  });
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  const address = server.address();
  if (!address || typeof address === 'string') throw new Error('Missing test server address');
  const nativeFetch = globalThis.fetch;
  let headersRead = false;
  let bodyRead = false;
  let transportSignal: AbortSignal | null | undefined;
  // Only resolve the relative URL and observe timing: APIClient, fetch, the
  // streamed response and JSON parsing all remain real.
  vi.stubGlobal('fetch', async (url: string, options: RequestInit) => {
    transportSignal = options.signal;
    const response = await nativeFetch(new URL(url, `http://127.0.0.1:${address.port}`), options);
    headersRead = true;
    const json = response.json.bind(response);
    response.json = async () => {
      const data = await json();
      bodyRead = true;
      return data;
    };
    return response;
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const summaryKey = [...showPrepKeys.evaluations, [purchaseId]];
  qc.setQueryData(summaryKey, { evaluations: { [purchaseId]: evaluation() }, errors: {} });
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  const hook = renderHook(() => useShowEvidence(purchaseId, true, 'requested-old'), { wrapper });
  try {
    await waitFor(() => expect(headersRead).toBe(true));
    expect(bodyRead).toBe(false);
    await act(async () => { await qc.cancelQueries({ queryKey: showPrepKeys.evidence(purchaseId) }); });
    // APIClient has already removed forwarding at headers. The body can still
    // complete, which is why query cancellation alone does not guard side effects.
    expect(transportSignal?.aborted).toBe(false);
    const newer = evaluation({ version: 'eval-2', status: 'below_target', listedPriceCents: 35000 });
    qc.setQueryData(summaryKey, { evaluations: { [purchaseId]: newer }, errors: {} });
    qc.setQueryData(showPrepKeys.detail(listId), detail([member({ evaluation: newer })]));
    stream.end(`${JSON.stringify(evaluation())},"sales":[]}`);
    await waitFor(() => expect(bodyRead).toBe(true));
    expect(qc.getQueryData<InventoryEvaluations>(summaryKey)?.evaluations[purchaseId]).toEqual(newer);
    expect(qc.getQueryData([...showPrepKeys.evidence(purchaseId), 'eval-1'])).toBeUndefined();
    expect(qc.getQueryState(showPrepKeys.detail(listId))?.isInvalidated).toBe(false);
  } finally {
    hook.unmount();
    qc.clear();
    server.closeAllConnections();
    await new Promise<void>(resolve => server.close(() => resolve()));
  }
});
