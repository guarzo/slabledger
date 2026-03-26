import { Button } from '../../../ui';
import type { ReviewStats } from '../../../../types/campaigns/priceReview';

interface ReviewSummaryBarProps {
  stats: ReviewStats;
  searchQuery: string;
  onSearchChange: (query: string) => void;
  showAll: boolean;
  onToggleShowAll: () => void;
}

interface StatBlockProps {
  label: string;
  value: number;
  colorClass?: string;
}

function StatBlock({ label, value, colorClass }: StatBlockProps) {
  return (
    <div className="text-center px-3">
      <div className={`text-lg font-semibold tabular-nums ${colorClass ?? 'text-[var(--text)]'}`}>
        {value}
      </div>
      <div className="text-[10px] uppercase tracking-wider text-[var(--text-muted)]">
        {label}
      </div>
    </div>
  );
}

export default function ReviewSummaryBar({ stats, searchQuery, onSearchChange, showAll, onToggleShowAll }: ReviewSummaryBarProps) {
  return (
    <div className="flex items-center justify-between gap-4 rounded-lg border border-[var(--border)] bg-[var(--surface-raised)] px-4 py-3">
      {/* Stats */}
      <div className="flex items-center gap-1 divide-x divide-[var(--border)]">
        <StatBlock label="Cards" value={stats.total} />
        <StatBlock label="Need Review" value={stats.needsReview} colorClass="text-[var(--warning)]" />
        <StatBlock label="Reviewed" value={stats.reviewed} colorClass="text-[var(--success)]" />
        <StatBlock label="Flagged" value={stats.flagged} colorClass="text-[var(--danger)]" />
      </div>

      {/* Controls */}
      <div className="flex items-center gap-3">
        <input
          type="text"
          placeholder="Search cards..."
          value={searchQuery}
          onChange={e => onSearchChange(e.target.value)}
          className="w-48 px-3 py-1.5 text-sm rounded-md border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:border-[var(--accent)]"
        />
        <Button
          variant={showAll ? 'primary' : 'secondary'}
          size="sm"
          onClick={onToggleShowAll}
        >
          {showAll ? 'Show Needs Review' : 'Show All'}
        </Button>
      </div>
    </div>
  );
}
