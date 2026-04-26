export interface TabularPriceRow {
  label: string;
  value: string;
  /** When true, the row is rendered with subtle highlight (e.g. the recommended price). */
  highlighted?: boolean;
}

interface TabularPriceTripletProps {
  rows: TabularPriceRow[];
  className?: string;
}

/**
 * Right-aligned, tabular-num mini-grid for stacked label/value pairs inside a
 * single table cell (e.g. Cost / CL / Sug in the Reprice table). Matches the
 * column rhythm so values align across rows in a wider table.
 */
export default function TabularPriceTriplet({ rows, className }: TabularPriceTripletProps) {
  if (rows.length === 0) return null;
  return (
    <div className={`grid grid-cols-[auto_1fr] gap-x-2 gap-y-0.5 text-xs tabular-nums ${className ?? ''}`.trim()}>
      {rows.map((row) => (
        <div
          key={row.label}
          data-highlighted={row.highlighted ? 'true' : undefined}
          className={`contents ${row.highlighted ? 'font-semibold text-[var(--text)]' : 'text-[var(--text-muted)]'}`}
        >
          <span className="uppercase tracking-wider">{row.label}</span>
          <span className="text-right">{row.value}</span>
        </div>
      ))}
    </div>
  );
}
