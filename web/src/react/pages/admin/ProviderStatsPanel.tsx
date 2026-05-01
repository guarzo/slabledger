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
import PokeballLoader from '../../PokeballLoader';
import Button from '../../ui/Button';

function formatMs(ms: number): string {
  if (!Number.isFinite(ms)) return '-';
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

export function MMStatsPanel({ enabled = true }: { enabled?: boolean }) {
  const { data: status, isLoading, isError, refetch } = useMarketMoversStatus({ enabled });
  const [showFailures, setShowFailures] = useState(false);
  // Fetch failures eagerly so the diagnostics row can surface the
  // "unprocessed" bucket — rows with no MM value and no error tag.
  const { data: failures } = useMarketMoversFailures({ enabled });

  if (isLoading) return <PokeballLoader size="sm" />;
  if (isError) {
    return (
      <div className="space-y-2">
        <p className="text-[var(--danger)] text-sm">Failed to load status.</p>
        <Button size="sm" onClick={() => { void refetch(); }}>Retry</Button>
      </div>
    );
  }
  if (!status?.configured) return <p className="text-[var(--text-muted)] text-sm">Not configured.</p>;

  const ps = status.priceStats;
  const lr = status.lastRun;
  const unprocessed = failures?.byReason?.unprocessed ?? 0;

  const diagnosticsShown =
    (!!lr && (lr.tokenMismatches > 0 || lr.noSalesData > 0 || lr.searchFailed > 0)) ||
    unprocessed > 0;
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

      {/* Row 2: last run. Search Failed is always visible so API errors
          aren't masked when a run also has upload activity. */}
      {lr && (
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
          <SummaryCard
            label="Last Run"
            value={formatAdminDate(lr.lastRunAt)}
            sub={formatMs(lr.durationMs)}
          />
          <SummaryCard label="Updated this run" value={lr.updated} />
          <SummaryCard label="New Mappings" value={lr.newMappings} />
          <SummaryCard
            label="Search Failed"
            value={lr.searchFailed}
            color={lr.searchFailed > 0 ? 'var(--warning)' : undefined}
          />
          {showRemoteActivity && (
            <SummaryCard
              label="Uploaded this run"
              value={lr.uploadedLastRun ?? 0}
              sub={(lr.deletedLastRun ?? 0) > 0 ? `${lr.deletedLastRun} deleted` : undefined}
            />
          )}
        </div>
      )}

      {/* Row 3: diagnostics — only when there are failures worth investigating.
          searchFailed intentionally lives in row 2 (always visible) so it
          isn't duplicated here. "Unprocessed" is current-state (rows with no
          value and no error tag) — orthogonal to last-run counts. */}
      {diagnosticsShown && (
        <div className="space-y-2">
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
            {unprocessed > 0 && (
              <SummaryCard
                label="Unprocessed"
                value={unprocessed}
                color="var(--warning)"
                sub="No value, no error tag"
              />
            )}
            {lr && (
              <>
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
              </>
            )}
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
  const { data: status, isLoading, isError, refetch } = useCardLadderStatus({ enabled });
  const [showFailures, setShowFailures] = useState(false);
  // Fetch failures eagerly (not just on modal open) so the diagnostics row
  // can surface the "unprocessed" bucket — rows with no CL value and no
  // error tag. Without this, silent misses stay invisible until the user
  // clicks through to the modal.
  const { data: failures } = useCardLadderFailures({ enabled });

  if (isLoading) return <PokeballLoader size="sm" />;
  if (isError) {
    return (
      <div className="space-y-2">
        <p className="text-[var(--danger)] text-sm">Failed to load status.</p>
        <Button size="sm" onClick={() => { void refetch(); }}>Retry</Button>
      </div>
    );
  }
  if (!status?.configured) return <p className="text-[var(--text-muted)] text-sm">Not configured.</p>;

  const ps = status.priceStats;
  const lr = status.lastRun;
  const unprocessed = failures?.byReason?.unprocessed ?? 0;

  const diagnosticsShown =
    (!!lr && (lr.certResolveFailed > 0 || lr.noValue > 0 || lr.noCert > 0)) ||
    unprocessed > 0;
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
          <SummaryCard label="New Mappings" value={lr.resolved} />
          {showRemoteActivity ? (
            <SummaryCard
              label="Uploaded this run"
              value={lr.cardsPushed}
              sub={lr.cardsRemoved > 0 ? `${lr.cardsRemoved} deleted` : undefined}
            />
          ) : (
            <SummaryCard label="Total Purchases" value={lr.totalPurchases} />
          )}
        </div>
      )}

      {/* Row 3: diagnostics. "Unprocessed" is current state (inventory with no
          value and no error tag); the other three reflect what the last run
          tagged. Shown together so silent misses are visible alongside
          recorded failures. */}
      {diagnosticsShown && (
        <div className="space-y-2">
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            {unprocessed > 0 && (
              <SummaryCard
                label="Unprocessed"
                value={unprocessed}
                color="var(--warning)"
                sub="No value, no error tag"
              />
            )}
            {lr && (
              <>
                <SummaryCard
                  label="Cert Resolve Failed"
                  value={lr.certResolveFailed}
                  color={lr.certResolveFailed > 0 ? 'var(--warning)' : undefined}
                  sub="CL didn't recognize cert"
                />
                <SummaryCard
                  label="No Cert on Purchase"
                  value={lr.noCert}
                  sub="Can't look up without cert"
                />
                <SummaryCard
                  label="Resolved, No Value"
                  value={lr.noValue}
                  sub="Catalog returned $0"
                />
              </>
            )}
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
  const { data: status, isLoading, isError, refetch } = usePSASyncStatus({ enabled });

  if (isLoading) return <PokeballLoader size="sm" />;
  if (isError) {
    return (
      <div className="space-y-2">
        <p className="text-[var(--danger)] text-sm">Failed to load status.</p>
        <Button size="sm" onClick={() => { void refetch(); }}>Retry</Button>
      </div>
    );
  }
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
