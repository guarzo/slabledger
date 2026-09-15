import { evaluation } from '../pages/show-preparation/fixtures.test-support';
import type { ShowEvaluation } from '../../types/showprep';
export function ready(id: number, state = 'not_checked', overrides: Partial<ShowEvaluation> = {}) {
  return evaluation({ purchaseId: `id-${id}`, status: state === 'current' ? 'supported' : 'needs_review',
    evidenceNeedsReview: state !== 'current', readiness: {
      state, refreshEligibility: state === 'current' ? 'not_needed' : state === 'failed' ? 'retry_only' : 'needed',
      identityKey: id.toString(16).padStart(64, '0'), expiresAt: state === 'current' ? '2026-09-15T00:00:00Z' : '', retryAt: '',
    }, ...overrides });
}
