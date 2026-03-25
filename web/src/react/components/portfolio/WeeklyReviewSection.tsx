import { useState, useMemo } from 'react';
import type { WeeklyReviewSummary, WeeklyPerformer } from '../../../types/campaigns';
import { formatCents } from '../../utils/formatters';
import { saleChannelLabels } from '../../utils/campaignConstants';
import CollapsibleHeader from './CollapsibleHeader';

function DeltaIndicator({ current, previous, isCents = false }: { current: number; previous: number; isCents?: boolean }) {
  if (previous === 0 && current === 0) return <span className="text-[var(--text-muted)]">--</span>;
  const delta = previous !== 0 ? ((current - previous) / Math.abs(previous)) * 100 : (current > 0 ? 100 : current < 0 ? -100 : 0);
  const isUp = delta > 0;
  const isDown = delta < 0;
  const color = isUp ? 'text-[var(--success)]' : isDown ? 'text-[var(--danger)]' : 'text-[var(--text-muted)]';
  const arrow = isUp ? '\u2191' : isDown ? '\u2193' : '';
  const displayVal = isCents ? formatCents(current) : current.toString();

  return (
    <span className={color}>
      {displayVal} {arrow && <span className="text-xs">{arrow} {Math.abs(delta).toFixed(0)}%</span>}
    </span>
  );
}

function PerformerList({ title, items, titleColorClass, itemColorClass }: {
  title: string;
  items: WeeklyPerformer[];
  titleColorClass: string;
  itemColorClass?: string;
}) {
  return (
    <div>
      <div className={`text-xs font-medium ${titleColorClass} mb-1.5`}>{title}</div>
      <div className="space-y-1">
        {items.map(p => (
          <div key={p.certNumber} className="flex items-center justify-between text-xs bg-[var(--surface-0)]/30 rounded-lg px-2.5 py-1.5 border border-[var(--surface-2)]/30 transition-colors hover:bg-[var(--surface-2)]/30">
            <span className="text-[var(--text)] truncate mr-2" title={p.cardName}>
              {p.cardName} <span className="text-[var(--text-muted)]">{p.grader ?? 'PSA'} {p.grade}</span>
            </span>
            <span className={`${itemColorClass ?? titleColorClass} whitespace-nowrap font-medium`}>
              {formatCents(p.profitCents)} <span className="text-[var(--text-muted)]">({saleChannelLabels[p.channel as keyof typeof saleChannelLabels] ?? p.channel})</span>
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

function MetricTile({ title, current, previous, isCents, className }: {
  title: string;
  current: number;
  previous: number;
  isCents?: boolean;
  className?: string;
}) {
  return (
    <div className={`bg-[var(--surface-0)]/40 rounded-xl border border-[var(--surface-2)]/50 p-3 text-center ${className ?? ''}`}>
      <div className="text-xs text-[var(--text-muted)] mb-1">{title}</div>
      <div className="text-sm font-semibold">
        <DeltaIndicator current={current} previous={previous} isCents={isCents} />
      </div>
    </div>
  );
}

export default function WeeklyReviewSection({ data }: { data: WeeklyReviewSummary }) {
  const [open, setOpen] = useState(true);

  const weekLabel = useMemo(() => {
    const start = new Date(data.weekStart + 'T12:00:00');
    const end = new Date(data.weekEnd + 'T12:00:00');
    const fmt = (d: Date) => d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
    return `${fmt(start)} - ${fmt(end)}`;
  }, [data.weekStart, data.weekEnd]);

  return (
    <div className="p-4 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)]">
      <CollapsibleHeader title={`Weekly Review (${weekLabel})`} open={open} onToggle={() => setOpen(!open)} />
      {open && (
        <div className="mt-3 space-y-4">
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
            <MetricTile title="Purchases" current={data.purchasesThisWeek} previous={data.purchasesLastWeek} />
            <MetricTile title="Spend" current={data.spendThisWeekCents} previous={data.spendLastWeekCents} isCents />
            <MetricTile title="Sales" current={data.salesThisWeek} previous={data.salesLastWeek} />
            <MetricTile title="Revenue" current={data.revenueThisWeekCents} previous={data.revenueLastWeekCents} isCents />
            <MetricTile title="Profit" current={data.profitThisWeekCents} previous={data.profitLastWeekCents} isCents className="col-span-2 sm:col-span-1" />
          </div>

          {((data.topPerformers?.length ?? 0) > 0 || (data.bottomPerformers?.length ?? 0) > 0) && (
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              {(data.topPerformers?.length ?? 0) > 0 && (
                <PerformerList title="Top Performers" items={data.topPerformers} titleColorClass="text-[var(--success)]" />
              )}
              {(data.bottomPerformers?.length ?? 0) > 0 && (
                <PerformerList title="Bottom Performers" items={data.bottomPerformers} titleColorClass="text-[var(--danger)]" />
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
