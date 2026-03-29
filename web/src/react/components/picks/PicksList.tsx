/**
 * PicksList
 *
 * Fetches and displays today's AI-generated acquisition picks.
 */
import { useQuery } from '@tanstack/react-query';
import { api } from '../../../js/api';
import { currency } from '../../utils/formatters';
import type { Pick, Signal } from '../../../types/picks';

const glassStyle = { background: 'var(--glass-bg)', backdropFilter: 'blur(12px)' } as const;

/* ------------------------------------------------------------------ */
/*  Helpers                                                            */
/* ------------------------------------------------------------------ */

function directionPillClass(direction: Pick['direction']): string {
  switch (direction) {
    case 'buy':    return 'bg-emerald-500/20 text-emerald-300 border border-emerald-500/30';
    case 'watch':  return 'bg-amber-500/20 text-amber-300 border border-amber-500/30';
    case 'avoid':  return 'bg-red-500/20 text-red-400 border border-red-500/30';
  }
}

function directionLabel(direction: Pick['direction']): string {
  switch (direction) {
    case 'buy':   return 'Buy';
    case 'watch': return 'Watch';
    case 'avoid': return 'Avoid';
  }
}

function confidenceBadgeClass(confidence: Pick['confidence']): string {
  switch (confidence) {
    case 'high':   return 'text-emerald-400';
    case 'medium': return 'text-amber-400';
    case 'low':    return 'text-[var(--text-muted)]';
  }
}

function signalChipClass(direction: Signal['direction']): string {
  switch (direction) {
    case 'bullish': return 'bg-emerald-500/15 text-emerald-300 border border-emerald-500/20';
    case 'bearish': return 'bg-red-500/15 text-red-400 border border-red-500/20';
    case 'neutral': return 'bg-[var(--surface-2)]/60 text-[var(--text-muted)] border border-[var(--surface-2)]';
  }
}

/* ------------------------------------------------------------------ */
/*  Sub-components                                                     */
/* ------------------------------------------------------------------ */

function PickCard({ pick }: { pick: Pick }) {
  const margin =
    pick.target_buy_price > 0
      ? (((pick.expected_sell_price - pick.target_buy_price) / pick.target_buy_price) * 100).toFixed(0)
      : null;

  return (
    <div
      className="p-4 rounded-xl border border-[var(--surface-2)]/50 space-y-3"
      style={glassStyle}
    >
      {/* Header row */}
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-start gap-3 min-w-0">
          {/* Rank badge */}
          <span className="shrink-0 w-7 h-7 rounded-full bg-[var(--brand-500)]/20 text-[var(--brand-500)] text-xs font-bold flex items-center justify-center border border-[var(--brand-500)]/30">
            {pick.rank}
          </span>
          <div className="min-w-0">
            <p className="text-sm font-semibold text-[var(--text)] leading-snug truncate">
              {pick.card_name}
            </p>
            <p className="text-xs text-[var(--text-muted)] truncate">
              {pick.set_name} &middot; PSA {pick.grade}
            </p>
          </div>
        </div>
        <div className="shrink-0 flex flex-col items-end gap-1">
          <span className={`text-xs font-semibold px-2 py-0.5 rounded-full ${directionPillClass(pick.direction)}`}>
            {directionLabel(pick.direction)}
          </span>
          <span className={`text-xs font-medium ${confidenceBadgeClass(pick.confidence)}`}>
            {pick.confidence} confidence
          </span>
        </div>
      </div>

      {/* Buy thesis */}
      {pick.buy_thesis && (
        <p className="text-xs text-[var(--text-secondary)] leading-relaxed">
          {pick.buy_thesis}
        </p>
      )}

      {/* Price targets */}
      <div className="flex items-center gap-4 text-xs">
        <div>
          <span className="text-[var(--text-muted)]">Target buy </span>
          <span className="font-semibold text-[var(--text)]">{currency(pick.target_buy_price)}</span>
        </div>
        <div>
          <span className="text-[var(--text-muted)]">Expected sell </span>
          <span className="font-semibold text-[var(--text)]">{currency(pick.expected_sell_price)}</span>
        </div>
        {margin !== null && (
          <div>
            <span className={`font-semibold ${Number(margin) >= 0 ? 'text-emerald-400' : 'text-red-400'}`}>
              {Number(margin) >= 0 ? '+' : ''}{margin}% margin
            </span>
          </div>
        )}
      </div>

      {/* Signal chips */}
      {pick.signals && pick.signals.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {pick.signals.map((sig, i) => (
            <span
              key={i}
              title={sig.detail}
              className={`text-xs px-2 py-0.5 rounded-full font-medium ${signalChipClass(sig.direction)}`}
            >
              {sig.title}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Main component                                                     */
/* ------------------------------------------------------------------ */

export default function PicksList() {
  const { data, isLoading, isError } = useQuery({
    queryKey: ['picks', 'latest'],
    queryFn: () => api.getPicks(),
    staleTime: 5 * 60 * 1000,
  });

  if (isLoading) {
    return (
      <div className="space-y-3">
        {[1, 2, 3].map(n => (
          <div
            key={n}
            className="h-28 rounded-xl animate-pulse border border-[var(--surface-2)]/40"
            style={glassStyle}
          />
        ))}
      </div>
    );
  }

  if (isError) {
    return (
      <div className="p-4 rounded-xl border border-[var(--danger-border)] bg-[var(--danger-bg)] text-sm text-[var(--danger)]">
        Failed to load picks. Please try again later.
      </div>
    );
  }

  const picks = data?.picks ?? [];

  if (picks.length === 0) {
    return (
      <div
        className="p-6 rounded-xl border border-[var(--surface-2)]/50 text-center text-sm text-[var(--text-muted)]"
        style={glassStyle}
      >
        No picks available yet. Check back after the daily AI analysis runs.
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {picks.map(pick => (
        <PickCard key={pick.id} pick={pick} />
      ))}
    </div>
  );
}
