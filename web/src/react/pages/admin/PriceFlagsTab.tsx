import { useState } from 'react';
import PokeballLoader from '../../PokeballLoader';
import { usePriceFlags, useResolvePriceFlag } from '../../queries/useAdminQueries';
import { formatCents } from '../../utils/formatters';
import { SummaryCard } from './shared';
import type { PriceFlagWithContext } from '../../../types/campaigns/priceReview';
import { PRICE_FLAG_LABELS } from '../../../types/campaigns/priceReview';

type FilterStatus = 'open' | 'resolved' | 'all';

const REASON_COLORS: Record<string, string> = {
  wrong_match: 'bg-red-400/15 text-red-400',
  stale_data: 'bg-yellow-400/15 text-yellow-400',
  wrong_grade: 'bg-orange-400/15 text-orange-400',
  source_disagreement: 'bg-blue-400/15 text-blue-400',
  other: 'bg-[var(--surface-2)] text-[var(--text-muted)]',
};

function FlagCard({ flag }: { flag: PriceFlagWithContext }) {
  const resolveMutation = useResolvePriceFlag();
  const isResolved = !!flag.resolvedAt;

  const reasonLabel = PRICE_FLAG_LABELS[flag.reason] ?? flag.reason;
  const reasonColor = REASON_COLORS[flag.reason] ?? REASON_COLORS.other;

  const cardTitle = [flag.cardName, flag.setName, flag.cardNumber ? `#${flag.cardNumber}` : null]
    .filter(Boolean)
    .join(' · ');

  return (
    <div className="rounded-xl bg-[var(--surface-1)] border border-[var(--surface-2)] p-4 space-y-3">
      {/* Header row */}
      <div className="flex items-start justify-between gap-2">
        <div>
          <div className="text-sm font-medium text-[var(--text)]">{cardTitle}</div>
          <div className="text-xs text-[var(--text-muted)] mt-0.5">
            PSA {flag.grade} · Cert #{flag.certNumber}
          </div>
        </div>
        <span className={`text-xs font-medium px-2 py-0.5 rounded-full whitespace-nowrap ${reasonColor}`}>
          {reasonLabel}
        </span>
      </div>

      {/* Price context */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs">
        <div>
          <div className="text-[var(--text-muted)]">Market Price</div>
          <div className="text-[var(--text)] font-medium">{formatCents(flag.marketPriceCents)}</div>
        </div>
        <div>
          <div className="text-[var(--text-muted)]">CL Value</div>
          <div className="text-[var(--text)] font-medium">{formatCents(flag.clValueCents)}</div>
        </div>
        <div>
          <div className="text-[var(--text-muted)]">Reviewed Price</div>
          <div className="text-[var(--text)] font-medium">{formatCents(flag.reviewedPriceCents)}</div>
        </div>
        <div>
          <div className="text-[var(--text-muted)]">Flagged By</div>
          <div className="text-[var(--text)] font-medium truncate">{flag.flaggedByEmail}</div>
        </div>
      </div>

      {/* Fusion source breakdown */}
      {((flag.sourcePrices && flag.sourcePrices.length > 0) || flag.clValueCents > 0) && (
        <div>
          <div className="text-xs text-[var(--text-muted)] mb-1.5">Source Breakdown</div>
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="text-[var(--text-muted)] border-b border-[var(--surface-2)]">
                  <th className="text-left pb-1 pr-3 font-medium">Source</th>
                  <th className="text-right pb-1 pr-3 font-medium">Price</th>
                  <th className="text-right pb-1 pr-3 font-medium">Confidence</th>
                  <th className="text-right pb-1 font-medium">Sales</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[var(--surface-2)]">
                {flag.sourcePrices?.map((sp) => (
                  <tr key={sp.source}>
                    <td className="py-1 pr-3 text-[var(--text)] capitalize">{sp.source}</td>
                    <td className="py-1 pr-3 text-right text-[var(--text)]">{formatCents(sp.priceCents)}</td>
                    <td className="py-1 pr-3 text-right text-[var(--text-muted)]">{sp.confidence ?? '—'}</td>
                    <td className="py-1 text-right text-[var(--text-muted)]">{sp.saleCount ?? '—'}</td>
                  </tr>
                ))}
                {flag.clValueCents > 0 && (!flag.sourcePrices || flag.sourcePrices.length === 0) && (
                  <tr>
                    <td className="py-1 pr-3 text-[var(--text)] capitalize">Card Ladder</td>
                    <td className="py-1 pr-3 text-right text-[var(--text)]">{formatCents(flag.clValueCents)}</td>
                    <td className="py-1 pr-3 text-right text-[var(--text-muted)]">—</td>
                    <td className="py-1 text-right text-[var(--text-muted)]">—</td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Footer row */}
      <div className="flex items-center justify-between pt-1">
        <div className="text-xs text-[var(--text-muted)]">
          {isResolved
            ? `Resolved ${flag.resolvedAt ? new Date(flag.resolvedAt).toLocaleDateString() : ''}`
            : `Flagged ${new Date(flag.flaggedAt).toLocaleDateString()}`}
        </div>
        {!isResolved && (
          <button
            onClick={() => resolveMutation.mutate(flag.id)}
            disabled={resolveMutation.isPending}
            className="text-xs px-3 py-1 rounded-lg bg-[var(--success-bg,#16a34a22)] text-[var(--success)] border border-[var(--success-border,#16a34a44)] hover:opacity-80 disabled:opacity-50 transition-opacity"
          >
            {resolveMutation.isPending ? 'Resolving...' : 'Mark Resolved'}
          </button>
        )}
      </div>
    </div>
  );
}

export function PriceFlagsTab({ enabled = true }: { enabled?: boolean }) {
  const [filter, setFilter] = useState<FilterStatus>('open');

  const { data, error, isLoading } = usePriceFlags(filter, { enabled });

  // Compute open / resolved counts from the 'all' query for summary display,
  // but only request it when we already have data to avoid an extra fetch on load.
  const { data: allData } = usePriceFlags('all', { enabled: enabled && !!data });

  const openCount = allData?.flags.filter((f) => !f.resolvedAt).length ?? 0;
  const resolvedCount = allData?.flags.filter((f) => !!f.resolvedAt).length ?? 0;

  if (isLoading) {
    return (
      <div className="py-8" role="status" aria-live="polite" aria-atomic="true">
        <span className="sr-only">Loading price flags…</span>
        <PokeballLoader />
      </div>
    );
  }

  if (error && !data) {
    return (
      <div className="p-3 rounded-lg bg-[var(--danger-bg)] border border-[var(--danger-border)] text-[var(--danger)] text-sm">
        Failed to load price flags
      </div>
    );
  }

  const flags = data?.flags ?? [];

  const filterTabs: { id: FilterStatus; label: string }[] = [
    { id: 'open', label: 'Open' },
    { id: 'resolved', label: 'Resolved' },
    { id: 'all', label: 'All' },
  ];

  return (
    <div className="space-y-6">
      {error && data && (
        <div className="p-3 rounded-lg bg-[var(--warning-bg)] border border-[var(--warning-border)] text-[var(--warning)] text-sm">
          Failed to refresh — showing cached data
        </div>
      )}

      {/* Summary */}
      <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
        <SummaryCard
          label="Open Flags"
          value={openCount}
          color={openCount > 0 ? 'var(--danger)' : undefined}
        />
        <SummaryCard
          label="Resolved"
          value={resolvedCount}
          color="var(--success)"
        />
        <SummaryCard
          label="Total"
          value={(allData?.total ?? data?.total) ?? 0}
        />
      </div>

      {/* Filter tabs */}
      <div className="flex gap-1 border-b border-[var(--surface-2)]">
        {filterTabs.map((tab) => (
          <button
            key={tab.id}
            onClick={() => setFilter(tab.id)}
            className={`px-3 py-1.5 text-sm font-medium rounded-t-lg transition-colors ${
              filter === tab.id
                ? 'text-[var(--brand-400)] border-b-2 border-[var(--brand-400)]'
                : 'text-[var(--text-muted)] hover:text-[var(--text)]'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Flag list */}
      {flags.length === 0 ? (
        filter === 'open' ? (
          <div className="text-center py-8 text-[var(--success)]">
            <div className="text-2xl mb-2">✓</div>
            <p className="text-sm font-medium">All clear — no open price flags.</p>
          </div>
        ) : (
          <div className="text-center py-8 text-[var(--text-muted)]">
            <p className="text-sm">No {filter} flags found.</p>
          </div>
        )
      ) : (
        <div className="max-h-[min(600px,calc(100vh-350px))] overflow-y-auto scrollbar-dark space-y-3">
          {flags.map((flag) => (
            <FlagCard key={flag.id} flag={flag} />
          ))}
        </div>
      )}
    </div>
  );
}
