import { getShowReadiness } from '../../../js/api/showprep';
import type { Availability, ReadinessState, ShowEvaluation, SupportStatus } from '../../../types/showprep';

export const supportLabels: Record<SupportStatus, string> = {
  supported: 'Supported', thin_evidence: 'Thin evidence', below_target: 'Below target',
  no_recent_comps: 'No recent sales', needs_review: 'Needs review', no_listed_price: 'No DH price',
};
export const availabilityLabels: Record<Availability, string> = {
  ready: 'Ready to pack', not_received: 'Not received', sold: 'Unavailable: sold',
  refunded: 'Unavailable: refunded', campaign_closed: 'Unavailable: campaign closed',
  removed: 'Unavailable: record removed', unknown: 'Availability unknown',
};
const readinessLabels: Record<ReadinessState, string> = {
  not_checked: 'No comp data', current: 'Current evidence', stale: 'Out of date', running: 'Data unavailable',
  interrupted: 'Data unavailable', failed: 'Data unavailable', invalid: 'Data unavailable', unavailable: 'Needs matching',
};
export function evidenceLabel(e: ShowEvaluation): string {
  if (e.priceAssociationUnclear) return 'Price association unclear';
  const readiness = getShowReadiness(e);
  if (readiness && readiness.state !== 'current') return readinessLabels[readiness.state];
  return supportLabels[e.status] ?? 'Needs review';
}
export function supportIndicator(e: ShowEvaluation): { label: string; tone: string } {
  if (!e.canAdd && e.availability !== 'ready' && e.availability !== 'not_received') return { label: 'Unavailable', tone: 'muted' };
  const label = e.status === 'no_listed_price' ? supportLabels.no_listed_price : evidenceLabel(e);
  const state = getShowReadiness(e)?.state;
  const tone = label === 'Supported' ? 'success' : state === 'failed' ? 'danger'
    : ['No comp data', 'Unavailable'].includes(label) ? 'muted' : 'warning';
  return { label, tone };
}
export function listingTypeLabel(value: string): string {
  const words = value.trim().replace(/([a-z])([A-Z])/g, '$1 $2').replace(/[_-]+/g, ' ').toLowerCase();
  return words ? words[0].toUpperCase() + words.slice(1) : 'Listing type unknown';
}
export function showError(error: unknown): string {
  return error instanceof Error ? error.message : 'Request failed. Retry when ready.';
}
export function showTime(time: string): string {
  if (!time || Number.isNaN(Date.parse(time))) return 'Unknown';
  return `${new Date(time).toISOString().slice(0, 16).replace('T', ' ')} UTC`;
}
export function safeSourceURL(value: string): string | undefined {
  try {
    const url = new URL(value);
    return ['https:', 'http:'].includes(url.protocol) ? url.href : undefined;
  } catch { return undefined; }
}
