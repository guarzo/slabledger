export type DHBadgeLabel =
  | 'sold'
  | 'listed'
  | 'in stock'
  | 'held'
  | 'unmatched'
  | 'dismissed'
  | 'pending'
  | 'pushed'
  | 'unenrolled';

export function dhBadgeFor(dhPushStatus?: string, dhStatus?: string): DHBadgeLabel {
  if (dhStatus === 'sold') return 'sold';
  if (dhStatus === 'listed') return 'listed';
  if (dhStatus === 'in_stock') return 'in stock';
  switch (dhPushStatus) {
    case 'held':
      return 'held';
    case 'unmatched':
      return 'unmatched';
    case 'dismissed':
      return 'dismissed';
    case 'pending':
      return 'pending';
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
  unmatched: 'bg-[rgba(248,113,113,0.1)] text-[#f87171]',
  dismissed: 'bg-[rgba(248,113,113,0.1)] text-[#f87171]',
  pending: 'bg-[rgba(245,158,11,0.1)] text-[#fbbf24]',
  pushed: 'bg-[rgba(245,158,11,0.1)] text-[#fbbf24]',
  unenrolled: 'bg-[var(--surface-2)] text-[var(--text-muted)]',
};
