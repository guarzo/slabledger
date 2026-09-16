import { APIError, DEFAULT_TIMEOUT_MS, type APIRequestOptions } from './client';
import type { ShowEvaluation } from '../../types/showprep';

/** Evaluation remains a retryable read, even though its large ID set uses POST. */
export async function evaluateShowEvidence(purchaseIds: string[], options?: APIRequestOptions): Promise<{ evaluations: ShowEvaluation[] }> {
  for (let attempt = 1; ; attempt++) {
    options?.signal?.throwIfAborted();
    try { return await showJSON<{ evaluations: ShowEvaluation[] }>('/evaluate', { purchaseIds }, options); }
    catch (error) {
      const retryable = error instanceof APIError && (error.code === 'NETWORK_ERROR' || error.status === 429 || error.status >= 500);
      if (!retryable || attempt >= 3) throw error;
      await new Promise<void>((resolve, reject) => {
        const abort = () => { clearTimeout(timer); reject(options?.signal?.reason); };
        const timer = setTimeout(() => { options?.signal?.removeEventListener('abort', abort); resolve(); }, 1000 * 2 ** (attempt - 1));
        options?.signal?.addEventListener('abort', abort, { once: true });
        if (options?.signal?.aborted) abort();
      });
    }
  }
}

/** One deadline covers headers and the entire body; callers own any retry policy. */
export async function showJSON<T>(endpoint: string, body: object, options?: APIRequestOptions): Promise<T> {
  const external = options?.signal;
  if (external?.aborted) throw new APIError('Request was cancelled', 0, 'CANCELLED');
  const timeoutMs = options?.timeoutMs ?? DEFAULT_TIMEOUT_MS;
  const controller = new AbortController();
  const { signal } = controller;
  let timedOut = false;
  let reader: ReadableStreamDefaultReader<Uint8Array> | undefined;
  const cancel = () => controller.abort(external?.reason);
  const timeout = setTimeout(() => { timedOut = true; controller.abort(); }, timeoutMs);
  const aborted = new Promise<never>((_resolve, reject) => {
    signal.addEventListener('abort', () => {
      // Cancel the locked reader as well as fetch: also settles stalled streams.
      void reader?.cancel().catch(() => {});
      reject(new APIError(timedOut ? `Request timed out after ${timeoutMs}ms` : 'Request was cancelled', 0, timedOut ? 'TIMEOUT' : 'CANCELLED'));
    }, { once: true });
  });
  external?.addEventListener('abort', cancel, { once: true });
  if (external?.aborted) cancel();
  try {
    const response = await Promise.race([aborted, fetch(`/api/show-prep${endpoint}`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      credentials: 'include', body: JSON.stringify(body), signal,
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
    try { data = JSON.parse(text); } catch { if (response.ok) throw new APIError('Invalid show preparation response. Retry the read.', 0, 'INVALID_RESPONSE'); }
    if (!response.ok) throw new APIError(data?.error || data?.message || `API error: ${response.status} ${response.statusText}`, response.status, data?.code, data);
    return data;
  } catch (error) {
    if (signal.aborted) throw new APIError(timedOut ? `Request timed out after ${timeoutMs}ms` : 'Request was cancelled', 0, timedOut ? 'TIMEOUT' : 'CANCELLED');
    if (error instanceof APIError) throw error;
    throw new APIError(error instanceof Error ? error.message : 'Network error', 0, 'NETWORK_ERROR');
  } finally {
    clearTimeout(timeout); external?.removeEventListener('abort', cancel);
    reader?.releaseLock();
  }
}
