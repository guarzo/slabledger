import { Button } from '../../../ui';
import { useMediaQuery } from '../../../hooks/useMediaQuery';

export type StatClickTarget = 'flagged';

interface ReviewSummaryBarProps {
  searchQuery: string;
  onSearchChange: (query: string) => void;
  showAll: boolean;
  onToggleShowAll: () => void;
}

export default function ReviewSummaryBar({ searchQuery, onSearchChange, showAll, onToggleShowAll }: ReviewSummaryBarProps) {
  const isMobile = useMediaQuery('(max-width: 768px)');
  return (
    <div className={`rounded-lg border border-[var(--border)] bg-[var(--surface-raised)] px-4 py-3 ${isMobile ? '' : 'flex items-center justify-end'}`}>
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
          title={showAll ? 'Return to filter tabs' : 'Show every card, ignoring filter tabs'}
        >
          {showAll ? 'Use Filters' : 'Show All'}
        </Button>
      </div>
    </div>
  );
}
