/**
 * StatCard
 *
 * Compact stat display with label, value, optional color accent, and optional small text.
 */

export interface StatCardProps {
  label: string;
  value: string;
  color?: 'green' | 'red';
  small?: boolean;
}

export default function StatCard({ label, value, color, small }: StatCardProps) {
  const colorClass = color === 'green' ? 'text-[var(--success)]' : color === 'red' ? 'text-[var(--danger)]' : 'text-[var(--text)]';
  return (
    <div className="bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)] p-3 text-center">
      <div className="text-xs text-[var(--text-muted)] mb-1">{label}</div>
      <div className={`${small ? 'text-xs' : 'text-lg'} font-bold ${colorClass}`}>{value}</div>
    </div>
  );
}
