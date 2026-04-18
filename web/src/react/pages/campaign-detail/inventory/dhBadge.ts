export type DHBadgeLabel =
  | 'sold'
  | 'listed'
  | 'in stock'
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
): DHBadgeLabel {
  if (dhStatus === 'sold') return 'sold';
  if (dhStatus === 'listed') return 'listed';
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

export const DH_BADGE_COLORS: Record<DHBadgeLabel, string> = {
  sold: 'bg-[var(--surface-1)] text-[var(--text-muted)]',
  listed: 'bg-[rgba(34,211,153,0.1)] text-[#34d399]',
  'in stock': 'bg-[rgba(99,102,241,0.1)] text-[#818cf8]',
  held: 'bg-[rgba(248,113,113,0.1)] text-[#f87171]',
  'no DH match': 'bg-[rgba(248,113,113,0.1)] text-[#f87171]',
  dismissed: 'bg-[rgba(248,113,113,0.1)] text-[#f87171]',
  'matching DH': 'bg-[rgba(245,158,11,0.1)] text-[#fbbf24]',
  'awaiting intake': 'bg-[var(--surface-2)] text-[var(--text-muted)]',
  pushed: 'bg-[rgba(245,158,11,0.1)] text-[#fbbf24]',
  unenrolled: 'bg-[var(--surface-2)] text-[var(--text-muted)]',
};
