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
  large?: boolean;
}

export default function StatCard({ label, value, color, small, large }: StatCardProps) {
  const colorClass = color === 'green' ? 'text-[var(--success)]' : color === 'red' ? 'text-[var(--danger)]' : 'text-[var(--text)]';
  const sizeClass = large ? 'text-2xl font-extrabold' : small ? 'text-xs font-bold' : 'text-lg font-bold';
  return (
    <div className={`bg-[var(--surface-1)] rounded-xl border ${large ? 'border-[var(--surface-3)] p-4' : 'border-[var(--surface-2)] p-3'} text-center`}>
      <div className="text-xs text-[var(--text-muted)] mb-1">{label}</div>
      <div className={`${sizeClass} ${colorClass}`}>{value}</div>
    </div>
  );
}
