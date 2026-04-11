import { useState } from 'react';
import {
  useMarketMoversStatus,
  useCardLadderStatus,
  usePSASyncStatus,
  useMarketMoversFailures,
  useCardLadderFailures,
} from '../../queries/useAdminQueries';
import { SummaryCard } from './shared';
import { formatAdminDate } from './adminUtils';
import { FailureBreakdownModal } from './FailureBreakdownModal';

function formatMs(ms: number): string {
  if (!Number.isFinite(ms)) return '-';
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

export function MMStatsPanel({ enabled = true }: { enabled?: boolean }) {
  const { data: status, isLoading, isError } = useMarketMoversStatus({ enabled });
  const [showFailures, setShowFailures] = useState(false);
  const { data: failures } = useMarketMoversFailures({ enabled: showFailures });

  if (isLoading) return <p className="text-[var(--text-muted)] text-sm">Loading...</p>;
  if (isError) return <p className="text-[var(--danger)] text-sm">Failed to load status.</p>;
  if (!status?.configured) return <p className="text-[var(--text-muted)] text-sm">Not configured.</p>;

  const ps = status.priceStats;
  const lr = status.lastRun;

  const diagnosticsShown =
    !!lr && (lr.tokenMismatches > 0 || lr.noSalesData > 0 || lr.searchFailed > 0);
  const showRemoteActivity =
    !!lr && ((lr.uploadedLastRun ?? 0) > 0 || (lr.deletedLastRun ?? 0) > 0);

  return (
    <div className="space-y-3">
      {/* Row 1: portfolio health */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <SummaryCard label="Cards Mapped" value={status.cardsMapped ?? 0} />
        <SummaryCard
          label="With Value"
          value={ps ? `${ps.withMMPrice} / ${ps.unsoldTotal}` : '—'}
          color={ps && ps.withMMPrice > 0 ? 'var(--success)' : undefined}
        />
        <SummaryCard label="In MM Collection" value={ps?.syncedCount ?? 0} />
        <SummaryCard
          label="Stale (>7d)"
          value={ps?.staleCount ?? 0}
          color={ps && ps.staleCount > 0 ? 'var(--warning)' : undefined}
        />
      </div>

      {/* Row 2: last run */}
      {lr && (
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
          <SummaryCard
            label="Last Run"
            value={formatAdminDate(lr.lastRunAt)}
            sub={formatMs(lr.durationMs)}
          />
          <SummaryCard label="Updated this run" value={lr.updated} />
          <SummaryCard label="New Mappings" value={lr.newMappings} />
          {showRemoteActivity ? (
            <SummaryCard
              label="Uploaded this run"
              value={lr.uploadedLastRun ?? 0}
              sub={(lr.deletedLastRun ?? 0) > 0 ? `${lr.deletedLastRun} deleted` : undefined}
            />
          ) : (
            <SummaryCard
              label="Search Failed"
              value={lr.searchFailed}
              color={lr.searchFailed > 0 ? 'var(--warning)' : undefined}
            />
          )}
        </div>
      )}

      {/* Row 3: diagnostics — only when there are failures worth investigating */}
      {diagnosticsShown && lr && (
        <div className="space-y-2">
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
            <SummaryCard
              label="Token Mismatches"
              value={lr.tokenMismatches}
              color={lr.tokenMismatches > 0 ? 'var(--warning)' : undefined}
              sub="Search hits all rejected"
            />
            <SummaryCard
              label="No 30-day Sales"
              value={lr.noSalesData}
              sub="Mapped but no recent price"
            />
            <SummaryCard
              label="API Errors"
              value={lr.searchFailed}
              color={lr.searchFailed > 0 ? 'var(--warning)' : undefined}
            />
          </div>
          <button
            type="button"
            onClick={() => setShowFailures(true)}
            className="text-xs text-[var(--accent)] hover:underline"
          >
            View failure breakdown →
          </button>
        </div>
      )}

      {showFailures && (
        <FailureBreakdownModal
          title="Market Movers — Failure Breakdown"
          report={failures ?? null}
          onClose={() => setShowFailures(false)}
        />
      )}
    </div>
  );
}

export function CLStatsPanel({ enabled = true }: { enabled?: boolean }) {
  const { data: status, isLoading, isError } = useCardLadderStatus({ enabled });
  const [showFailures, setShowFailures] = useState(false);
  const { data: failures } = useCardLadderFailures({ enabled: showFailures });

  if (isLoading) return <p className="text-[var(--text-muted)] text-sm">Loading...</p>;
  if (isError) return <p className="text-[var(--danger)] text-sm">Failed to load status.</p>;
  if (!status?.configured) return <p className="text-[var(--text-muted)] text-sm">Not configured.</p>;

  const ps = status.priceStats;
  const lr = status.lastRun;

  const diagnosticsShown =
    !!lr &&
    (lr.noImageMatch > 0 || lr.noCertMatch > 0 || lr.noValue > 0 || lr.orphanMappings > 0);
  const showRemoteActivity = !!lr && (lr.cardsPushed > 0 || lr.cardsRemoved > 0);

  return (
    <div className="space-y-3">
      {/* Row 1: portfolio health — mirrors the MM panel */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <SummaryCard label="Cards Mapped" value={status.cardsMapped ?? 0} />
        <SummaryCard
          label="With Value"
          value={ps ? `${ps.withCLValue} / ${ps.unsoldTotal}` : '—'}
          color={ps && ps.withCLValue > 0 ? 'var(--success)' : undefined}
        />
        <SummaryCard label="In CL Collection" value={ps?.syncedCount ?? 0} />
        <SummaryCard
          label="Stale (>7d)"
          value={ps?.staleCount ?? 0}
          color={ps && ps.staleCount > 0 ? 'var(--warning)' : undefined}
        />
      </div>

      {/* Row 2: last run */}
      {lr && (
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
          <SummaryCard
            label="Last Run"
            value={formatAdminDate(lr.lastRunAt)}
            sub={formatMs(lr.durationMs)}
          />
          <SummaryCard label="Updated this run" value={lr.updated} />
          <SummaryCard label="Total CL Cards" value={lr.totalCLCards} />
          {showRemoteActivity ? (
            <SummaryCard
              label="Uploaded this run"
              value={lr.cardsPushed}
              sub={lr.cardsRemoved > 0 ? `${lr.cardsRemoved} deleted` : undefined}
            />
          ) : (
            <SummaryCard label="Skipped (CL side)" value={lr.skipped} />
          )}
        </div>
      )}

      {/* Row 3: diagnostics */}
      {diagnosticsShown && lr && (
        <div className="space-y-2">
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            <SummaryCard
              label="No Image Match"
              value={lr.noImageMatch}
              color={lr.noImageMatch > 0 ? 'var(--warning)' : undefined}
              sub="Purchases with no CL card"
            />
            <SummaryCard
              label="No Cert on Purchase"
              value={lr.noCertMatch}
              sub="Can't fallback-match"
            />
            <SummaryCard
              label="Matched, No Value"
              value={lr.noValue}
              sub="CL returned $0"
            />
            <SummaryCard
              label="Orphan Mappings"
              value={lr.orphanMappings}
              color={lr.orphanMappings > 0 ? 'var(--warning)' : undefined}
              sub="Stored but unresolved"
            />
          </div>
          <button
            type="button"
            onClick={() => setShowFailures(true)}
            className="text-xs text-[var(--accent)] hover:underline"
          >
            View failure breakdown →
          </button>
        </div>
      )}

      {showFailures && (
        <FailureBreakdownModal
          title="Card Ladder — Failure Breakdown"
          report={failures ?? null}
          onClose={() => setShowFailures(false)}
        />
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
            <SummaryCard label="Total Rows" value={lr.totalRows} />
            <SummaryCard
              label="Allocated"
              value={lr.allocated}
              color={lr.allocated > 0 ? 'var(--success)' : undefined}
            />
            <SummaryCard label="Updated" value={lr.updated} />
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
          <SummaryCard label="Refunded" value={lr.refunded} />
        </div>
      )}
    </div>
  );
}
