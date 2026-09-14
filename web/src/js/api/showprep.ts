import { APIClient, type APIRequestOptions } from './client';
import type { ShowEvaluation, ShowEvidence, ShowList, ShowListDetail, ShowItemAdd, ShowItemUpdate, ShowReadiness, ReadinessState, RefreshEligibility } from '../../types/showprep';

const client = new APIClient('/api/show-prep');
const refreshClient = new APIClient('/api/show-prep');
// Refresh is an explicit upstream operation: transport failures must not replay it.
refreshClient.maxRetries = 1;

function requireArray<T>(value: T[], label: string): T[] {
  if (!Array.isArray(value)) throw new Error(`Invalid ${label} response. Retry the read.`);
  return value;
}
export function isShowEvaluation(value: ShowEvaluation | undefined): value is ShowEvaluation {
  if (!value?.purchaseId || !value.version) return false;
  return ['supported', 'thin_evidence', 'below_target', 'no_recent_comps', 'needs_review', 'no_listed_price'].includes(value.status)
    && ['ready', 'not_received', 'sold', 'refunded', 'campaign_closed', 'removed', 'unknown'].includes(value.availability)
    && [value.listedPriceCents, value.localPriceCents, value.medianCents, value.compCount].every(Number.isSafeInteger)
    && [value.canAdd, value.canPack, value.priceMismatch, value.priceAssociationUnclear, value.evidenceNeedsReview].every(flag => typeof flag === 'boolean')
    && typeof value.reason === 'string' && typeof value.evidenceReason === 'string';
}
const readinessEligibility: Record<ReadinessState, RefreshEligibility> = {
  not_checked: 'needed', current: 'not_needed', stale: 'needed', running: 'wait',
  interrupted: 'retry_only', failed: 'retry_only', invalid: 'retry_only', unavailable: 'unavailable',
};

function isReadinessTimestamp(value: string): boolean {
  // Require the server's UTC RFC3339Nano format and reject normalized invalid
  // calendar dates (Date.parse alone accepts February 30 and 24:00).
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value)) return false;
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) && new Date(parsed).toISOString().slice(0, 19) === value.slice(0, 19);
}

/** Invalid/missing metadata disables automatic checking, not display or manual APIs. */
export function getShowReadiness(evaluation: Pick<ShowEvaluation, 'readiness'> | null | undefined): ShowReadiness | undefined {
  const value = evaluation?.readiness;
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
  const { state, refreshEligibility, identityKey, expiresAt, retryAt } = value as Record<string, unknown>;
  if (typeof state !== 'string' || !Object.prototype.hasOwnProperty.call(readinessEligibility, state)) return undefined;
  const validatedState = state as ReadinessState;
  const validatedEligibility = readinessEligibility[validatedState];
  if (refreshEligibility !== validatedEligibility) return undefined;
  if (typeof identityKey !== 'string' || (!/^[a-f0-9]{64}$/.test(identityKey) && !(state === 'unavailable' && identityKey === ''))) return undefined;
  if (typeof expiresAt !== 'string' || typeof retryAt !== 'string') return undefined;
  if (state === 'current' || state === 'stale') {
    if (!isReadinessTimestamp(expiresAt)) return undefined;
  } else if (expiresAt !== '') return undefined;
  if (state === 'running' || state === 'interrupted') {
    if (!isReadinessTimestamp(retryAt)) return undefined;
  } else if (retryAt !== '') return undefined;
  return { state: validatedState, refreshEligibility: validatedEligibility, identityKey, expiresAt, retryAt };
}

async function evaluations(response: Promise<{ evaluations: ShowEvaluation[] }>) {
  const result = await response;
  return { evaluations: requireArray(result.evaluations, 'evaluation').filter(isShowEvaluation) };
}
async function listDetail(response: Promise<ShowListDetail>) {
  const result = await response;
  requireArray(result.items, 'show list items');
  if (!result.list || !result.summary) throw new Error('Invalid show list response. Retry the read.');
  return result;
}

export const showPrepAPI = {
  evaluate: (purchaseIds: string[], options?: APIRequestOptions) => evaluations(client.post('/evaluate', { purchaseIds }, options)),
  refresh: (purchaseIds: string[], signal?: AbortSignal) => evaluations(refreshClient.post('/refresh', { purchaseIds }, { signal, timeoutMs: 120000 })),
  evidence: async (purchaseId: string, options?: APIRequestOptions): Promise<ShowEvidence> => {
    const result = await client.get<ShowEvidence>(`/evidence/${encodeURIComponent(purchaseId)}`, options);
    requireArray(result.sales, 'evidence sales');
    if (!isShowEvaluation(result.evaluation) || result.evaluation.purchaseId !== purchaseId) throw new Error('Missing or invalid evidence evaluation. Retry the read.');
    return result;
  },
  lists: async (): Promise<ShowList[]> => {
    const result = await client.get<{ lists: ShowList[] }>('/lists');
    return requireArray(result.lists, 'show lists');
  },
  createList: (id: string, name: string) => client.post<ShowList>('/lists', { id, name }),
  renameList: (id: string, name: string) => client.put<ShowList>(`/lists/${encodeURIComponent(id)}`, { name }),
  detail: (id: string) => listDetail(client.get(`/lists/${encodeURIComponent(id)}`)),
  addItems: (id: string, items: ShowItemAdd[]) => listDetail(client.post(`/lists/${encodeURIComponent(id)}/items`, { items })),
  updateItem: (id: string, itemId: string, input: ShowItemUpdate) => listDetail(client.put(`/lists/${encodeURIComponent(id)}/items/${encodeURIComponent(itemId)}`, input)),
  removeItem: (id: string, itemId: string) => client.deleteResource(`/lists/${encodeURIComponent(id)}/items/${encodeURIComponent(itemId)}`),
};

export interface InventoryEvaluations {
  evaluations: Record<string, ShowEvaluation>;
  errors: Record<string, string>;
}
export async function evaluateInventory(purchaseIds: string[], signal?: AbortSignal): Promise<InventoryEvaluations> {
  const ids = [...new Set(purchaseIds)].sort();
  const batches: string[][] = [];
  for (let i = 0; i < ids.length; i += 200) batches.push(ids.slice(i, i + 200));
  const result: InventoryEvaluations = { evaluations: {}, errors: {} };
  let next = 0;
  async function worker() {
    while (next < batches.length && !signal?.aborted) {
      const batch = batches[next++];
      try {
        const response = await showPrepAPI.evaluate(batch, { signal });
        for (const id of batch) {
          const value = response.evaluations.find(e => e.purchaseId === id);
          if (value) result.evaluations[id] = value;
          else result.errors[id] = 'Evaluation missing from response. Retry evaluation.';
        }
      } catch (error) {
        for (const id of batch) result.errors[id] = error instanceof Error ? error.message : 'Evaluation failed';
      }
    }
  }
  await Promise.all(Array.from({ length: Math.min(3, batches.length) }, worker));
  return result;
}
