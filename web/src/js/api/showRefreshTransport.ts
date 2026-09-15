import { APIError } from './client';
import type { ShowEvaluation } from '../../types/showprep';

/** Source acquisition is never replayed. Keep guards until the entire body settles. */
export async function refreshShowEvidence(purchaseIds: string[], external?: AbortSignal): Promise<{ evaluations: ShowEvaluation[] }> {
  const controller = new AbortController();
  const { signal } = controller;
  let timedOut = false;
  let reader: ReadableStreamDefaultReader<Uint8Array> | undefined;
  const cancel = () => controller.abort(external?.reason);
  const timeout = setTimeout(() => { timedOut = true; controller.abort(); }, 120000);
  const aborted = new Promise<never>((_resolve, reject) => {
    signal.addEventListener('abort', () => {
      // Cancel the locked reader as well as fetch: also settles stalled streams.
      void reader?.cancel().catch(() => {});
      reject(new APIError(timedOut ? 'Request timed out after 120000ms' : 'Request was cancelled', 0, timedOut ? 'TIMEOUT' : 'CANCELLED'));
    }, { once: true });
  });
  external?.addEventListener('abort', cancel, { once: true });
  if (external?.aborted) cancel();
  try {
    const response = await Promise.race([aborted, fetch('/api/show-prep/refresh', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      credentials: 'include', body: JSON.stringify({ purchaseIds }), signal,
    })]);
    signal.throwIfAborted();
    reader = response.body?.getReader();
    let text = '';
    if (reader) {
      const decoder = new TextDecoder();
      while (true) {
        const part = await Promise.race([aborted, reader.read()]);
        signal.throwIfAborted();
        if (part.done) { text += decoder.decode(); break; }
        text += decoder.decode(part.value, { stream: true });
      }
    }
    signal.throwIfAborted();
    let data;
    try { data = JSON.parse(text); } catch (error) { if (response.ok) throw error; }
    if (!response.ok) throw new APIError(data?.error || data?.message || `API error: ${response.status} ${response.statusText}`, response.status, data?.code, data);
    return data;
  } catch (error) {
    if (signal.aborted) throw new APIError(timedOut ? 'Request timed out after 120000ms' : 'Request was cancelled', 0, timedOut ? 'TIMEOUT' : 'CANCELLED');
    if (error instanceof APIError) throw error;
    throw new APIError(error instanceof Error ? error.message : 'Network error', 0, 'NETWORK_ERROR');
  } finally {
    clearTimeout(timeout); external?.removeEventListener('abort', cancel);
    reader?.releaseLock();
  }
}
