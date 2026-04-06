import { clsx } from 'clsx';
import { LinkDropdown } from '../LinkDropdown';
import type { GradeKey } from '../../../types/pricing';
import { currency } from '../../utils/formatters';
import { gradeBorderColors, isNoData } from './priceCardUtils';

export interface LegacyPriceRowProps {
  label: string;
  price: number | undefined;
  ebayHref: string;
  altHref: string;
  cardLadderHref: string;
  compact?: boolean;
  gradeKey: GradeKey;
}

export function LegacyPriceRow({
  label,
  price,
  ebayHref,
  altHref,
  cardLadderHref,
  compact,
  gradeKey,
}: LegacyPriceRowProps) {
  if (price == null) return null;

  const isPsa10 = gradeKey === 'psa10';
  const isRaw = gradeKey === 'raw';
  const priceEmpty = isNoData(price);

  if (compact) {
    return (
      <div
        className={clsx(
          'flex justify-between items-center rounded border-l-2 py-1 px-1.5 -mx-1.5 text-xs',
          isPsa10 && 'font-bold',
        )}
        style={{
          borderLeftColor: gradeBorderColors[gradeKey],
          ...(isPsa10 ? { background: 'var(--gradient-psa10-highlight)' } : undefined),
        }}
      >
        <span className="text-[var(--text-muted)]">{label}</span>
        <span className="flex items-center gap-1.5">
          <span className={clsx('text-xs', priceEmpty ? 'text-[var(--text-subtle)]' : 'font-semibold text-[var(--text)]')}>
            {priceEmpty ? '\u2014' : currency(price)}
          </span>
          <a href={ebayHref} target="_blank" rel="noopener noreferrer"
            className="inline-flex items-center rounded px-1 py-0.5 text-[var(--text-muted)] hover:text-[var(--brand-400)] hover:bg-[var(--surface-hover)] transition-colors text-[10px]"
            title="Search on eBay">eBay</a>
          <a href={altHref} target="_blank" rel="noopener noreferrer"
            className="inline-flex items-center rounded px-1 py-0.5 text-[var(--text-muted)] hover:text-[var(--brand-400)] hover:bg-[var(--surface-hover)] transition-colors text-[10px]"
            title="Search on Alt">Alt</a>
          <a href={cardLadderHref} target="_blank" rel="noopener noreferrer"
            className="inline-flex items-center rounded px-1 py-0.5 text-[var(--text-muted)] hover:text-[var(--brand-400)] hover:bg-[var(--surface-hover)] transition-colors text-[10px]"
            title="Search on CL">CL</a>
        </span>
      </div>
    );
  }

  const dropdownLinks = [
    { label: 'eBay', href: ebayHref },
    { label: 'Alt', href: altHref },
    { label: 'Card Ladder', href: cardLadderHref },
  ];

  return (
    <div
      className={clsx(
        'flex justify-between items-center rounded border-l py-2 px-2 -mx-2 border-b border-[var(--surface-0)]',
        isPsa10 ? 'text-base' : 'text-sm',
        priceEmpty && 'opacity-40',
      )}
      style={{
        borderLeftColor: gradeBorderColors[gradeKey],
        borderLeftWidth: gradeKey === 'psa10' ? 4 : gradeKey === 'psa9' ? 3 : 2,
        ...(isPsa10
          ? { background: 'var(--gradient-psa10-highlight)' }
          : gradeKey === 'psa9'
          ? { background: 'var(--gradient-psa9-highlight)' }
          : undefined),
      }}
    >
      <span
        className={clsx('font-medium', isRaw && 'text-[var(--text-muted)]')}
        style={!isRaw ? { color: gradeBorderColors[gradeKey] } : undefined}
      >
        {label}
      </span>
      <span className="flex items-center gap-1.5">
        <span className={clsx(
          'tabular-nums',
          priceEmpty ? 'text-[var(--text-subtle)]' : 'font-semibold text-[var(--text)]',
        )}>
          {priceEmpty ? '\u2014' : currency(price)}
        </span>
        <LinkDropdown links={dropdownLinks} />
      </span>
    </div>
  );
}
