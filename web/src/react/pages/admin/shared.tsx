export function ProgressBar({ value, max, warningThreshold, dangerThreshold, invertColors }: {
  value: number; max: number;
  warningThreshold: number; dangerThreshold: number;
  invertColors?: boolean;
}) {
  const pct = max > 0 ? Math.min(100, Math.max(0, (value / max) * 100)) : 0;
  let color = 'bg-[var(--success)]';
  if (invertColors) {
    // Low values are bad (e.g., cache fill)
    if (pct < dangerThreshold) color = 'bg-[var(--danger)]';
    else if (pct < warningThreshold) color = 'bg-[var(--warning)]';
  } else {
    // High values are bad (e.g., API usage)
    if (pct >= dangerThreshold) color = 'bg-[var(--danger)]';
    else if (pct >= warningThreshold) color = 'bg-[var(--warning)]';
  }
  return (
    <div className="w-full bg-[var(--surface-2)] rounded-full h-2.5 overflow-hidden" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(pct)}>
      <div className={`h-full rounded-full transition-all duration-500 ${color}`} style={{ width: `${pct}%` }} aria-hidden="true" />
    </div>
  );
}

export function SummaryCard({ label, value, sub, color }: { label: string; value: number | string; sub?: string; color?: string }) {
  return (
    <div className="rounded-xl bg-[var(--surface-1)] border border-[var(--surface-2)] p-4">
      <div className="text-xs text-[var(--text-muted)]">{label}</div>
      <div className="text-xl font-semibold" style={color ? { color } : undefined}>{value}</div>
      {sub && <div className="text-xs text-[var(--text-muted)]">{sub}</div>}
    </div>
  );
}
