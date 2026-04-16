import { dhBadgeFor, DH_BADGE_COLORS } from './dhBadge';

interface DHStateBadgeProps {
  dhPushStatus?: string;
  dhStatus?: string;
  dhCardId?: number;
}

export function DHStateBadge({ dhPushStatus, dhStatus, dhCardId }: DHStateBadgeProps) {
  const label = dhBadgeFor(dhPushStatus, dhStatus);
  const colorClass = DH_BADGE_COLORS[label];

  return (
    <span className="inline-flex items-center gap-1">
      <span className={`inline-block rounded px-1.5 py-0.5 text-xs font-medium ${colorClass}`}>
        {label}
      </span>
      {dhCardId != null && dhCardId > 0 && (
        <a
          href={`https://doubleholo.com/card/${dhCardId}`}
          target="_blank"
          rel="noreferrer"
          onClick={(e) => e.stopPropagation()}
          className="text-xs text-[var(--brand-500)] hover:text-[var(--brand-400)]"
          title="View on DoubleHolo"
        >
          ↗
        </a>
      )}
    </span>
  );
}
