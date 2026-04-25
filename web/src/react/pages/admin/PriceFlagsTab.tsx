import { useState } from 'react';
import PokeballLoader from '../../PokeballLoader';
import { usePriceFlags, useResolvePriceFlag } from '../../queries/useAdminQueries';
import { formatCents } from '../../utils/formatters';
import type { PriceFlagWithContext } from '../../../types/campaigns/priceReview';
import { PRICE_FLAG_LABELS } from '../../../types/campaigns/priceReview';

type FilterStatus = 'open' | 'resolved' | 'all';

const REASON_COLORS: Record<string, string> = {
  wrong_match: 'bg-[var(--danger-bg)] text-[var(--danger)]',
  stale_data: 'bg-[var(--warning-bg)] text-[var(--warning)]',
  wrong_grade: 'bg-[var(--warning-bg)] text-[var(--warning)]',
  source_disagreement: 'bg-[var(--info-bg)] text-[var(--info)]',
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
          <div className="text-[var(--text)] font-medium tabular-nums">{formatCents(flag.marketPriceCents)}</div>
        </div>
        <div>
          <div className="text-[var(--text-muted)]">CL Value</div>
          <div className="text-[var(--text)] font-medium tabular-nums">{formatCents(flag.clValueCents)}</div>
        </div>
        <div>
          <div className="text-[var(--text-muted)]">Reviewed Price</div>
          <div className="text-[var(--text)] font-medium tabular-nums">{formatCents(flag.reviewedPriceCents)}</div>
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
                    <td className="py-1 pr-3 text-right text-[var(--text)] tabular-nums">{formatCents(sp.priceCents)}</td>
                    <td className="py-1 pr-3 text-right text-[var(--text-muted)]">{sp.confidence ?? '—'}</td>
                    <td className="py-1 text-right text-[var(--text-muted)]">{sp.saleCount ?? '—'}</td>
                  </tr>
                ))}
                {flag.clValueCents > 0 && (!flag.sourcePrices || flag.sourcePrices.length === 0) && (
                  <tr>
                    <td className="py-1 pr-3 text-[var(--text)] capitalize">Card Ladder</td>
                    <td className="py-1 pr-3 text-right text-[var(--text)] tabular-nums">{formatCents(flag.clValueCents)}</td>
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
  // Use only the global ('all') total so the "All" count doesn't flash a filtered subtotal
  // while allData is still loading.
  const totalCount = allData?.total ?? 0;

  const filterTabs: { id: FilterStatus; label: string; count: number }[] = [
    { id: 'open', label: 'Open', count: openCount },
    { id: 'resolved', label: 'Resolved', count: resolvedCount },
    { id: 'all', label: 'All', count: totalCount },
  ];

  return (
    <div className="space-y-4">
      {error && data && (
        <div className="p-3 rounded-lg bg-[var(--warning-bg)] border border-[var(--warning-border)] text-[var(--warning)] text-sm">
          Failed to refresh — showing cached data
        </div>
      )}

      {/* Filter tabs — counts inline */}
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
            {tab.label} <span className="ml-1 text-[var(--text-muted)] tabular-nums">({tab.count})</span>
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
            <p className="text-sm">No {filter} flags — none recorded.</p>
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
