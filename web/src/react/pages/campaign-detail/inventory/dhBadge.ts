import type { StatusTone } from '../../../ui/StatusPill';

export type DHBadgeLabel =
  | 'sold'
  | 'listed'
  | 'in stock'
  | 'shipped'
  | 'held'
  | 'no DH match'
  | 'dismissed'
  | 'matching DH'
  | 'awaiting intake'
  | 'pushed'
  | 'unenrolled';

export function dhBadgeFor(
  dhPushStatus?: string,
  dhStatus?: string,
  receivedAt?: string | null,
  psaShipDate?: string | null,
): DHBadgeLabel {
  if (dhStatus === 'sold') return 'sold';
  if (dhStatus === 'listed') return 'listed';
  if (!receivedAt && psaShipDate) return 'shipped';
  if (dhStatus === 'in_stock') return 'in stock';
  switch (dhPushStatus) {
    case 'held':
      return 'held';
    case 'unmatched':
      return 'no DH match';
    case 'dismissed':
      return 'dismissed';
    case 'pending':
      return receivedAt ? 'matching DH' : 'awaiting intake';
    case 'matched':
      return 'pushed';
    default:
      return 'unenrolled';
  }
}

/**
 * Tones for the shared StatusPill component.
 * Mapping: terminal states (sold) → neutral; positive (listed/in stock) → success/brand;
 * pipeline (shipped/matching/pushed) → warning; problems (held/no match/dismissed) → danger.
 */
export const DH_BADGE_TONES: Record<DHBadgeLabel, StatusTone> = {
  sold: 'neutral',
  listed: 'success',
  'in stock': 'brand',
  shipped: 'warning',
  held: 'danger',
  'no DH match': 'danger',
  dismissed: 'danger',
  'matching DH': 'warning',
  'awaiting intake': 'neutral',
  pushed: 'warning',
  unenrolled: 'neutral',
};
