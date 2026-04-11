import { Button } from '../../../ui';
import type { ReviewStats } from '../../../../types/campaigns/priceReview';
import { useMediaQuery } from '../../../hooks/useMediaQuery';

export type StatClickTarget = 'unreviewed' | 'flagged';

interface ReviewSummaryBarProps {
  stats: ReviewStats;
  searchQuery: string;
  onSearchChange: (query: string) => void;
  showAll: boolean;
  onToggleShowAll: () => void;
  onStatClick?: (target: StatClickTarget) => void;
}

interface StatBlockProps {
  label: string;
  value: number;
  colorClass?: string;
  onClick?: () => void;
}

function StatBlock({ label, value, colorClass, onClick }: StatBlockProps) {
  const clickable = onClick && value > 0;
  return (
    <div
      className={`text-center px-3 ${clickable ? 'cursor-pointer hover:opacity-80' : ''}`}
      onClick={clickable ? onClick : undefined}
      role={clickable ? 'button' : undefined}
      tabIndex={clickable ? 0 : undefined}
      onKeyDown={clickable ? (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onClick(); } } : undefined}
    >
      <div className={`text-lg font-semibold tabular-nums ${colorClass ?? 'text-[var(--text)]'}`}>
        {value}
      </div>
      <div className={`text-[10px] uppercase tracking-wider text-[var(--text-muted)] ${clickable ? 'underline decoration-dotted underline-offset-2' : ''}`}>
        {label}
      </div>
    </div>
  );
}

export default function ReviewSummaryBar({ stats, searchQuery, onSearchChange, showAll, onToggleShowAll, onStatClick }: ReviewSummaryBarProps) {
  const isMobile = useMediaQuery('(max-width: 768px)');
  return (
    <div className={`rounded-lg border border-[var(--border)] bg-[var(--surface-raised)] px-4 py-3 ${isMobile ? 'space-y-3' : 'flex items-center justify-between gap-4'}`}>
      {/* Stats */}
      <div className="flex items-center gap-1 divide-x divide-[var(--border)]">
        <StatBlock label="Cards" value={stats.total} />
        {!isMobile && (
          <>
            <StatBlock label="Unreviewed" value={stats.needsReview} colorClass="text-[var(--warning)]" onClick={() => onStatClick?.('unreviewed')} />
            <StatBlock label="Reviewed" value={stats.reviewed} colorClass="text-[var(--success)]" />
            <StatBlock label="Flagged" value={stats.flagged} colorClass="text-[var(--danger)]" onClick={() => onStatClick?.('flagged')} />
          </>
        )}
      </div>

      {/* Controls */}
      <div className="flex items-center gap-3">
        <input
          type="text"
          placeholder="Search cards..."
          value={searchQuery}
          onChange={e => onSearchChange(e.target.value)}
          className={`${isMobile ? 'flex-1' : 'w-48'} px-3 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:border-[var(--accent)]`}
        />
        <Button
          variant={showAll ? 'primary' : 'secondary'}
          size="sm"
          onClick={onToggleShowAll}
        >
          {showAll ? 'Show Unreviewed' : 'Show All'}
        </Button>
      </div>
    </div>
  );
}
