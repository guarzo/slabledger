import { useMarketMoversStatus, useCardLadderStatus, usePSASyncStatus } from '../../queries/useAdminQueries';
import { SummaryCard } from './shared';
import { formatAdminDate } from './adminUtils';

function formatMs(ms: number): string {
  if (!Number.isFinite(ms)) return '-';
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

export function MMStatsPanel({ enabled = true }: { enabled?: boolean }) {
  const { data: status, isLoading, isError } = useMarketMoversStatus({ enabled });

  if (isLoading) return <p className="text-[var(--text-muted)] text-sm">Loading...</p>;
  if (isError) return <p className="text-[var(--danger)] text-sm">Failed to load status.</p>;
  if (!status?.configured) return <p className="text-[var(--text-muted)] text-sm">Not configured.</p>;

  const ps = status.priceStats;
  const lr = status.lastRun;

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <SummaryCard
          label="Cards Mapped"
          value={status.cardsMapped ?? 0}
        />
        <SummaryCard
          label="With MM Price"
          value={ps ? `${ps.withMMPrice} / ${ps.unsoldTotal}` : '—'}
          color={ps && ps.withMMPrice > 0 ? 'var(--success)' : undefined}
        />
        <SummaryCard
          label="Synced to MM"
          value={ps?.syncedCount ?? 0}
        />
        <SummaryCard
          label="Stale (>7d)"
          value={ps?.staleCount ?? 0}
          color={ps && ps.staleCount > 0 ? 'var(--warning)' : undefined}
        />
      </div>
      {lr && (
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
          <SummaryCard
            label="Last Run"
            value={formatAdminDate(lr.lastRunAt)}
            sub={formatMs(lr.durationMs)}
          />
          <SummaryCard
            label="Updated"
            value={lr.updated}
          />
          <SummaryCard
            label="New Mappings"
            value={lr.newMappings}
          />
          <SummaryCard
            label="Search Failed"
            value={lr.searchFailed}
            color={lr.searchFailed > 0 ? 'var(--warning)' : undefined}
          />
        </div>
      )}
    </div>
  );
}

export function CLStatsPanel({ enabled = true }: { enabled?: boolean }) {
  const { data: status, isLoading, isError } = useCardLadderStatus({ enabled });

  if (isLoading) return <p className="text-[var(--text-muted)] text-sm">Loading...</p>;
  if (isError) return <p className="text-[var(--danger)] text-sm">Failed to load status.</p>;
  if (!status?.configured) return <p className="text-[var(--text-muted)] text-sm">Not configured.</p>;

  const lr = status.lastRun;

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <SummaryCard
          label="Cards Mapped"
          value={status.cardsMapped ?? 0}
        />
        {lr && (
          <>
            <SummaryCard
              label="Updated"
              value={lr.updated}
            />
            <SummaryCard
              label="Cards Pushed"
              value={lr.cardsPushed}
            />
            <SummaryCard
              label="Cards Removed"
              value={lr.cardsRemoved}
            />
          </>
        )}
      </div>
      {lr && (
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
          <SummaryCard
            label="Last Run"
            value={formatAdminDate(lr.lastRunAt)}
            sub={formatMs(lr.durationMs)}
          />
          <SummaryCard
            label="Mapped"
            value={lr.mapped}
          />
          <SummaryCard
            label="Skipped"
            value={lr.skipped}
          />
          <SummaryCard
            label="Total CL Cards"
            value={lr.totalCLCards}
          />
        </div>
      )}
    </div>
  );
}

export function PSAStatsPanel({ enabled = true }: { enabled?: boolean }) {
  const { data: status, isLoading, isError } = usePSASyncStatus({ enabled });

  if (isLoading) return <p className="text-[var(--text-muted)] text-sm">Loading...</p>;
  if (isError) return <p className="text-[var(--danger)] text-sm">Failed to load status.</p>;
  if (!status?.configured) return <p className="text-[var(--text-muted)] text-sm">Not configured.</p>;

  const lr = status.lastRun;

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <SummaryCard
          label="Pending Items"
          value={status.pendingCount ?? 0}
          color={status.pendingCount && status.pendingCount > 0 ? 'var(--warning)' : undefined}
        />
        {lr && (
          <>
            <SummaryCard
              label="Total Rows"
              value={lr.totalRows}
            />
            <SummaryCard
              label="Allocated"
              value={lr.allocated}
              color={lr.allocated > 0 ? 'var(--success)' : undefined}
            />
            <SummaryCard
              label="Updated"
              value={lr.updated}
            />
          </>
        )}
      </div>
      {lr && (
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
          <SummaryCard
            label="Last Run"
            value={formatAdminDate(lr.lastRunAt)}
            sub={formatMs(lr.durationMs)}
          />
          <SummaryCard
            label="Unmatched"
            value={lr.unmatched}
            color={lr.unmatched > 0 ? 'var(--warning)' : undefined}
          />
          <SummaryCard
            label="Parse Errors"
            value={lr.parseErrors}
            color={lr.parseErrors > 0 ? 'var(--danger)' : undefined}
          />
          <SummaryCard
            label="Refunded"
            value={lr.refunded}
          />
        </div>
      )}
    </div>
  );
}
