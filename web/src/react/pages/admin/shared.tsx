export function SummaryCard({ label, value, sub, color }: { label: string; value: number | string; sub?: string; color?: string }) {
  return (
    <div className="rounded-xl bg-[var(--surface-1)] border border-[var(--surface-2)] p-4">
      <div className="text-xs text-[var(--text-muted)]">{label}</div>
      <div className="text-xl font-semibold" style={color ? { color } : undefined}>{value}</div>
      {sub && <div className="text-xs text-[var(--text-muted)]">{sub}</div>}
    </div>
  );
}
