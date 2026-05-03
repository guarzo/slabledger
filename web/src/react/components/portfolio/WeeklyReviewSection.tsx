import { useState, useMemo, useEffect } from 'react';
import type { WeeklyReviewSummary, WeeklyPerformer } from '../../../types/campaigns';
import { formatCents } from '../../utils/formatters';
import { saleChannelLabels } from '../../utils/campaignConstants';
import { CardShell } from '../../ui';
import CollapsibleHeader from './CollapsibleHeader';

function DeltaIndicator({ current, previous, isCents = false, muted = false }: { current: number; previous: number; isCents?: boolean; muted?: boolean }) {
  if (previous === 0 && current === 0) return <span className="text-[var(--text-muted)]">--</span>;
  const delta = previous !== 0 ? ((current - previous) / Math.abs(previous)) * 100 : (current > 0 ? 100 : current < 0 ? -100 : 0);
  const isUp = delta > 0;
  const isDown = delta < 0;
  const semanticColor = isUp ? 'text-[var(--success)]' : isDown ? 'text-[var(--danger)]' : 'text-[var(--text-muted)]';
  const color = muted ? 'text-[var(--text)]' : semanticColor;
  const arrow = isUp ? '\u2191' : isDown ? '\u2193' : '';
  const displayVal = isCents ? formatCents(current) : current.toString();
  const arrowColor = muted ? 'text-[var(--text-muted)]' : '';

  return (
    <span className={`${color} tabular-nums`}>
      {displayVal} {arrow && <span className={`text-xs ${arrowColor}`}>{arrow} {Math.abs(delta).toFixed(0)}%</span>}
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
            <span className={`${itemColorClass ?? titleColorClass} whitespace-nowrap font-medium tabular-nums`}>
              {formatCents(p.profitCents)} <span className="text-[var(--text-muted)]">({saleChannelLabels[p.channel as keyof typeof saleChannelLabels] ?? p.channel})</span>
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

function MetricInline({ title, current, previous, isCents, muted }: {
  title: string;
  current: number;
  previous: number;
  isCents?: boolean;
  muted?: boolean;
}) {
  return (
    <div className="flex items-baseline gap-2 min-w-0">
      <span className="text-[10px] uppercase tracking-wider text-[var(--text-muted)] font-medium">{title}</span>
      <span className="text-sm font-semibold tabular-nums">
        <DeltaIndicator current={current} previous={previous} isCents={isCents} muted={muted} />
      </span>
    </div>
  );
}

export default function WeeklyReviewSection({ data }: { data: WeeklyReviewSummary }) {
  const [open, setOpen] = useState(true);
  // Tick once per minute so `now` stays fresh without re-rendering on every paint.
  // The minute granularity is enough — the header only shows the current day of week.
  const [nowMs, setNowMs] = useState(() => Date.now());
  useEffect(() => {
    const interval = window.setInterval(() => setNowMs(Date.now()), 60_000);
    return () => window.clearInterval(interval);
  }, []);

  const { weekLabel, inProgress, daysElapsed } = useMemo(() => {
    // Anchor at local start-of-day so the "day X of 7" count doesn't jitter with time-of-day.
    const startOfDay = (d: Date): Date => {
      const copy = new Date(d);
      copy.setHours(0, 0, 0, 0);
      return copy;
    };
    const start = new Date(data.weekStart + 'T12:00:00');
    const end = new Date(data.weekEnd + 'T12:00:00');
    const fmt = (d: Date) => d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
    const now = new Date(nowMs);
    const inProg = now >= start && now < end;
    const msPerDay = 24 * 60 * 60 * 1000;
    const elapsed = Math.min(
      7,
      Math.max(1, Math.floor((startOfDay(now).getTime() - startOfDay(start).getTime()) / msPerDay) + 1),
    );
    return { weekLabel: `${fmt(start)} - ${fmt(end)}`, inProgress: inProg, daysElapsed: elapsed };
  }, [data.weekStart, data.weekEnd, nowMs]);

  return (
    <CardShell variant="default" padding="sm" radius="sm">
      <CollapsibleHeader
        title={`Weekly Review (${weekLabel})${inProgress ? ` \u00b7 in progress \u2014 day ${daysElapsed} of 7` : ''}`}
        open={open}
        onToggle={() => setOpen(!open)}
      />
      {open && (
        <div className="mt-3 space-y-4">
          <div className="flex flex-wrap items-baseline gap-x-6 gap-y-3 py-2 border-y border-[rgba(255,255,255,0.06)]">
            <MetricInline title="Purchases" current={data.purchasesThisWeek} previous={data.purchasesLastWeek} muted={inProgress} />
            <MetricInline title="Spend" current={data.spendThisWeekCents} previous={data.spendLastWeekCents} isCents muted={inProgress} />
            <MetricInline title="Sales" current={data.salesThisWeek} previous={data.salesLastWeek} muted={inProgress} />
            <MetricInline title="Revenue" current={data.revenueThisWeekCents} previous={data.revenueLastWeekCents} isCents muted={inProgress} />
            <MetricInline title="Profit" current={data.profitThisWeekCents} previous={data.profitLastWeekCents} isCents muted={inProgress} />
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
    </CardShell>
  );
}
