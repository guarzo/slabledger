import { Link } from 'react-router-dom';
import { RecommendationBadge } from '../../ui/RecommendationBadge';
import type { Status, TuningColumn, TuningRow } from '../../../types/insights';

const STATUS_META: Record<Status, { label: string; badge: string; strip: string }> = {
  Act:  { label: 'Action', badge: 'bg-[var(--danger)]/15 text-[var(--danger)]',           strip: 'var(--danger)' },
  Kill: { label: 'Kill',   badge: 'bg-[var(--danger)]/25 text-[var(--danger)] font-bold', strip: 'var(--danger)' },
  Tune: { label: 'Tune',   badge: 'bg-[var(--warning)]/15 text-[var(--warning)]',         strip: 'var(--warning)' },
  OK:   { label: 'OK',     badge: 'bg-[var(--success)]/15 text-[var(--success)]',         strip: 'var(--success)' },
};

// Sort order: Act and Kill (most urgent) first, then Tune, then OK, then anything else.
const STATUS_ORDER: Record<Status, number> = { Act: 0, Kill: 1, Tune: 2, OK: 3 };

function sortByUrgency(rows: TuningRow[]): TuningRow[] {
  return [...rows].sort((a, b) => {
    const oa = STATUS_ORDER[a.status] ?? 99;
    const ob = STATUS_ORDER[b.status] ?? 99;
    if (oa !== ob) return oa - ob;
    return a.campaignName.localeCompare(b.campaignName);
  });
}

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
  const sorted = sortByUrgency(rows);
  return (
    <section className="space-y-2">
      <div className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]">
        Campaign tuning · all active campaigns
      </div>
      <div className="border border-[var(--surface-2)] rounded-xl overflow-hidden">
        <div className="grid grid-cols-[1.5fr_1fr_1fr_1fr_1fr_auto] gap-2 pl-4 pr-3 py-2 bg-[var(--surface-2)]/40 text-[10px] uppercase tracking-wider font-semibold text-[var(--text-muted)]">
          <div>Campaign</div>
          {columns.map(c => (
            <div key={c.key}>{c.label}</div>
          ))}
          <div className="w-16 text-right">Status</div>
        </div>
        {sorted.map(row => {
          const meta = STATUS_META[row.status];
          return (
            <Link
              key={row.campaignId}
              to={`/campaigns/${row.campaignId}`}
              data-status={row.status}
              style={{ borderLeftColor: meta.strip }}
              className="grid grid-cols-[1.5fr_1fr_1fr_1fr_1fr_auto] gap-2 pl-3 pr-3 py-2.5 border-t border-l-2 border-[var(--surface-2)] text-sm items-center hover:bg-[var(--surface-2)]/30"
            >
              <div className="font-semibold">{row.campaignName}</div>
              {columns.map(c => {
                const cell = row.cells[c.key];
                if (!cell) {
                  return <div key={c.key} className="text-[var(--text-muted)]">—</div>;
                }
                return (
                  <div key={c.key}>
                    <RecommendationBadge label={cell.recommendation} severity={cell.severity} />
                  </div>
                );
              })}
              <div className="w-16 text-right">
                <span className={`inline-block px-1.5 py-0.5 rounded text-[10px] ${meta.badge}`}>
                  {meta.label}
                </span>
              </div>
            </Link>
          );
        })}
      </div>
    </section>
  );
}
