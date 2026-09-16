import { APIClient, type APIRequestOptions } from './client';
import type { ShowPrepWorkerStatus } from '../../types/showprepWorker';

const client = new APIClient();
// Retry failed advances a durable epoch. Do not replay an ambiguous POST as an
// automatic transport retry; the operator can inspect status and request again.
client.maxRetries = 1;

async function readStatus(path: string, options?: APIRequestOptions): Promise<ShowPrepWorkerStatus> {
  const value = await client.get<ShowPrepWorkerStatus>(path, options);
  const counts = value && [value.eligibleIdentities, value.currentIdentities, value.missingIdentities,
    value.staleIdentities, value.failedIdentities, value.eligibleCards, value.currentCards, value.unresolvedCards];
  if (!value || typeof value.enabled !== 'boolean' || typeof value.configured !== 'boolean'
    || !['disabled', 'unconfigured', 'idle', 'running', 'failed', 'auth_hold'].includes(value.state)
    || !counts?.every(count => Number.isSafeInteger(count) && count >= 0)
    || ![value.lastSweepAt, value.retryAt, value.error].every(text => typeof text === 'string')) {
    throw new Error('Evidence worker status unavailable');
  }
  return value;
}

export const showPrepWorkerAPI = {
  coverage: (options?: APIRequestOptions) => readStatus('/show-prep/coverage', options),
  status: (options?: APIRequestOptions) => readStatus('/admin/show-prep/worker', options),
  request: (retry: boolean) => client.post<{ status: 'accepted' }>(`/admin/show-prep/worker/${retry ? 'retry' : 'run'}`, {}),
};
