import type { ScanCommand, ScanResult } from '../../types/showprep';
import { isShowEvaluation } from './showprep';

const statuses = ['supported', 'thin_evidence', 'below_target', 'mixed_evidence', 'no_recent_comps', 'needs_review', 'no_listed_price'];
const codes = ['contents_changed', 'no_match', 'ambiguous_cert', 'identity_changed', 'not_received', 'sold', 'refunded', 'campaign_closed', 'removed', 'unknown_availability'];
export const isScanUUID = (value: unknown): value is string => typeof value === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(value);
const record = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const nonnegativeInteger = (value: unknown): value is number => Number.isSafeInteger(value) && (value as number) >= 0;
export function isScanCommand(value: unknown): value is ScanCommand {
  if (!record(value) || Object.keys(value).sort().join(',') !== 'certNumber,commandId,withdrawalGeneration') return false;
  return isScanUUID(value.commandId) && typeof value.certNumber === 'string' && !!value.certNumber
    && value.certNumber === value.certNumber.trim() && new TextEncoder().encode(value.certNumber).length <= 128
    && nonnegativeInteger(value.withdrawalGeneration);
}
function timestamp(value: unknown): boolean {
  if (typeof value !== 'string' || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value)) return false;
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) && new Date(parsed).toISOString().slice(0, 19) === value.slice(0, 19);
}
export function validateScanResult(value: unknown, listId: string, command: ScanCommand): ScanResult {
  const invalid = () => { throw new Error('Invalid scan response. Not confirmed; retry the same scan.'); };
  if (!record(value) || !record(value.original) || !record(value.current) || typeof value.replay !== 'boolean') return invalid();
  const original = value.original;
  const current = value.current;
  if (original.commandId !== command.commandId || original.listId !== listId || original.certNumber !== command.certNumber
    || typeof original.outcome !== 'string' || !['confirmed', 'already_confirmed', 'rechecked', 'rejected'].includes(original.outcome)
    || !Number.isSafeInteger(original.acknowledgedPriceCents) || typeof original.cardName !== 'string'
    || typeof current.state !== 'string' || !['present', 'removed', 'unavailable', 'unknown'].includes(current.state)) return invalid();
  if (original.outcome === 'rejected') {
    if (typeof original.code !== 'string' || !codes.includes(original.code) || original.itemId !== '' || original.purchaseId !== '' || original.cardName !== ''
      || original.confirmedAt !== '' || original.acknowledgedPriceCents !== 0 || original.acknowledgedStatus !== '') return invalid();
  } else if (original.code !== '' || !isScanUUID(original.itemId) || typeof original.purchaseId !== 'string' || !original.purchaseId
    || !timestamp(original.confirmedAt) || typeof original.acknowledgedStatus !== 'string' || !statuses.includes(original.acknowledgedStatus)) return invalid();
  if (!value.replay && original.outcome !== 'rejected' && (current.state !== 'present' || current.evaluation === undefined)) return invalid();
  if (current.evaluation !== undefined) {
    const e = current.evaluation as ScanResult['current']['evaluation'];
    if (!isShowEvaluation(e) || e.purchaseId !== original.purchaseId || e.certNumber !== command.certNumber
      || typeof e.cardName !== 'string' || typeof e.grader !== 'string' || !Number.isFinite(e.grade)
      || !Number.isSafeInteger(e.listedPriceCents) || !Number.isSafeInteger(e.localPriceCents)
      || [e.medianCents, e.compCount, e.recent.count, e.recent.medianCents,
        e.recent.latestSaleCount, e.recent.latestSaleMinCents, e.recent.latestSaleMaxCents].some(value => !nonnegativeInteger(value))
      || [e.listingSyncedAt, e.latestSaleDate, e.windowStart, e.windowEnd, e.refreshedAt, e.evidenceVersion].some(value => typeof value !== 'string')) return invalid();
  }
  // Current evidence is a separate read projection, not a replacement baseline.
  return value as unknown as ScanResult;
}
