import { APIError, type APIRequestOptions } from './client';
import { showJSON } from './showRefreshTransport';
import type { PricePreview, RecentPriceEvidence } from '../../types/showprep';

const canonicalUUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

function record(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value);
}
function dateOnly(value: unknown): value is string {
  if (typeof value !== 'string' || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) && new Date(parsed).toISOString().slice(0, 10) === value;
}
function recentEvidence(value: unknown): value is RecentPriceEvidence {
  if (!record(value)) return false;
  return dateOnly(value.windowStart) && dateOnly(value.windowEnd)
    && (value.latestSaleDate === '' || dateOnly(value.latestSaleDate))
    && Array.isArray(value.saleIds) && value.saleIds.every(id => typeof id === 'string' && id.length > 0)
    && [value.count, value.medianCents, value.latestSaleCount, value.latestSaleMinCents, value.latestSaleMaxCents]
      .every(cents => typeof cents === 'number' && Number.isSafeInteger(cents) && cents >= 0)
    && (value.gapPct === null || (typeof value.gapPct === 'number' && Number.isFinite(value.gapPct)));
}
function isPreview(value: unknown, purchaseId: string, priceCents: number): value is PricePreview {
  if (!record(value)) return false;
  // Fail closed if an evaluation/list token or a different request's result leaks into this DTO.
  const keys = ['purchaseId', 'currentPriceCents', 'trialPriceCents', 'status', 'reason', 'evidenceNeedsReview',
    'evidenceReason', 'evidenceVersion', 'policyVersion', 'recent'];
  return Object.keys(value).length === keys.length && keys.every(key => Object.prototype.hasOwnProperty.call(value, key))
    && value.purchaseId === purchaseId && value.trialPriceCents === priceCents
    && typeof value.currentPriceCents === 'number' && Number.isSafeInteger(value.currentPriceCents) && value.currentPriceCents >= 0
    && typeof value.status === 'string'
    && ['supported', 'thin_evidence', 'below_target', 'mixed_evidence', 'no_recent_comps', 'needs_review', 'no_listed_price'].includes(value.status)
    && typeof value.reason === 'string' && typeof value.evidenceReason === 'string'
    && typeof value.evidenceNeedsReview === 'boolean' && typeof value.evidenceVersion === 'string'
    && typeof value.policyVersion === 'string' && value.policyVersion.length > 0
    && recentEvidence(value.recent);
}

export const priceReviewAPI = {
  preview: async (purchaseId: string, priceCents: number, options?: APIRequestOptions): Promise<PricePreview> => {
    if (!canonicalUUID.test(purchaseId) || !Number.isSafeInteger(priceCents) || priceCents <= 0) {
      throw new APIError('Invalid purchase UUID or trial price cents', 0, 'INVALID_REQUEST');
    }
    const result = await showJSON<unknown>('/preview', { purchaseId, priceCents }, options);
    if (!isPreview(result, purchaseId, priceCents)) {
      throw new APIError('Invalid price preview response. Retry the read.', 0, 'INVALID_RESPONSE');
    }
    return result;
  },
};
