import type { Availability, SupportStatus } from '../../../types/showprep';

export const supportLabels: Record<SupportStatus, string> = {
  supported: 'Supported', thin_evidence: 'Thin evidence', below_target: 'Below target',
  no_recent_comps: 'No recent comps', needs_review: 'Needs review', no_listed_price: 'No listed price',
};
export const availabilityLabels: Record<Availability, string> = {
  ready: 'Ready to pack', not_received: 'Not received', sold: 'Unavailable: sold',
  refunded: 'Unavailable: refunded', campaign_closed: 'Unavailable: campaign closed',
  removed: 'Unavailable: record removed', unknown: 'Availability unknown',
};
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
