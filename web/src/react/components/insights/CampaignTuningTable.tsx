import { Link } from 'react-router-dom';
import type { TuningCell, TuningColumn, TuningRow } from '../../../types/insights';

const cellTone: Record<TuningCell['severity'], string> = {
  act: 'text-[var(--danger)]',
  tune: 'text-[var(--warning)]',
  ok: 'text-[var(--success)]',
};

const statusBadge = {
  Act: 'bg-[var(--danger)]/15 text-[var(--danger)]',
  Tune: 'bg-[var(--warning)]/15 text-[var(--warning)]',
  OK: 'bg-[var(--success)]/15 text-[var(--success)]',
  Kill: 'bg-[var(--danger)]/25 text-[var(--danger)] font-bold',
} as const;

const columns: Array<{ key: TuningColumn; label: string }> = [
  { key: 'buyPct', label: 'Buy %' },
  { key: 'characters', label: 'Characters' },
  { key: 'years', label: 'Years' },
  { key: 'spendCap', label: 'Spend cap' },
];

export default function CampaignTuningTable({ rows }: { rows: TuningRow[] }) {
  if (rows.length === 0) {
    return (
      <section className="space-y-2">
        <div className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]">
          Campaign tuning
        </div>
        <div className="p-4 rounded-xl border border-[var(--surface-2)] bg-[var(--surface-1)] text-sm text-[var(--text-muted)]">
          No active campaigns.
        </div>
      </section>
    );
  }
  return (
    <section className="space-y-2">
      <div className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]">
        Campaign tuning · all active campaigns
      </div>
      <div className="border border-[var(--surface-2)] rounded-xl overflow-hidden">
        <div className="grid grid-cols-[1.5fr_1fr_1fr_1fr_1fr_auto] gap-2 px-3 py-2 bg-[var(--surface-2)]/40 text-[10px] uppercase tracking-wider font-semibold text-[var(--text-muted)]">
          <div>Campaign</div>
          {columns.map(c => (
            <div key={c.key}>{c.label}</div>
          ))}
          <div className="w-14 text-right">Status</div>
        </div>
        {rows.map(row => (
          <Link
            key={row.campaignId}
            to={`/campaigns/${row.campaignId}`}
            className="grid grid-cols-[1.5fr_1fr_1fr_1fr_1fr_auto] gap-2 px-3 py-2.5 border-t border-[var(--surface-2)] text-sm items-center hover:bg-[var(--surface-2)]/30"
          >
            <div className="font-semibold">{row.campaignName}</div>
            {columns.map(c => {
              const cell = row.cells[c.key];
              return (
                <div key={c.key} className={cell ? cellTone[cell.severity] : 'text-[var(--text-muted)]'}>
                  {cell ? cell.recommendation : '—'}
                </div>
              );
            })}
            <div className="w-14 text-right">
              <span className={`inline-block px-1.5 py-0.5 rounded text-[10px] ${statusBadge[row.status]}`}>
                {row.status}
              </span>
            </div>
          </Link>
        ))}
      </div>
    </section>
  );
}
