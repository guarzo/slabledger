import { Link } from 'react-router-dom';
import { RecommendationBadge } from '../../ui/RecommendationBadge';
import SectionEyebrow from '../../ui/SectionEyebrow';
import EmptyState from '../../ui/EmptyState';
import type { Status, TuningColumn, TuningRow } from '../../../types/insights';
import { useMediaQuery } from '../../hooks/useMediaQuery';
import tableStyles from './CampaignTuningTable.module.css';

const STATUS_META: Record<Status, { label: string; badge: string }> = {
  Act:  { label: 'Action', badge: 'bg-[var(--danger)]/15 text-[var(--danger)]' },
  Kill: { label: 'Kill',   badge: 'bg-[var(--danger)]/25 text-[var(--danger)] font-bold' },
  Tune: { label: 'Tune',   badge: 'bg-[var(--warning)]/15 text-[var(--warning)]' },
  OK:   { label: 'OK',     badge: 'bg-[var(--success)]/15 text-[var(--success)]' },
};

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
  const isMobile = useMediaQuery('(max-width: 480px)');

  if (rows.length === 0) {
    return (
      <section className="space-y-2">
        <SectionEyebrow>Campaign tuning</SectionEyebrow>
        <EmptyState
          title="No active campaigns"
          description="Campaign tuning will appear here once campaigns are running."
          compact
        />
      </section>
    );
  }
  const sorted = sortByUrgency(rows);

  if (isMobile) {
    return (
      <section className="space-y-2">
        <SectionEyebrow>Campaign tuning · all active campaigns</SectionEyebrow>
        <div className="space-y-2">
          {sorted.map((row) => {
            const meta = STATUS_META[row.status];
            return (
              <Link
                key={row.campaignId}
                to={`/campaigns/${row.campaignId}`}
                data-severity={row.status.toLowerCase()}
                className="block rounded-xl border border-l-2 border-[var(--surface-2)] bg-[var(--surface-1)] px-3 py-2.5 hover:bg-[var(--surface-2)]/30"
              >
                <div className="flex items-baseline justify-between gap-2 mb-2">
                  <span className="text-sm font-semibold text-[var(--text)] truncate min-w-0">{row.campaignName}</span>
                  <span className={`inline-block px-1.5 py-0.5 rounded text-[10px] ${meta.badge} whitespace-nowrap`}>
                    {meta.label}
                  </span>
                </div>
                <dl className="grid grid-cols-2 gap-x-3 gap-y-1.5 text-xs">
                  {columns.map((c) => {
                    const cell = row.cells[c.key];
                    return (
                      <div key={c.key} className="flex items-center justify-between gap-2">
                        <dt className="text-[var(--text-muted)] uppercase tracking-wider text-[10px]">{c.label}</dt>
                        <dd className="min-w-0">
                          {cell ? (
                            <RecommendationBadge label={cell.recommendation} severity={cell.severity} />
                          ) : (
                            <span className="text-[var(--text-muted)]">—</span>
                          )}
                        </dd>
                      </div>
                    );
                  })}
                </dl>
              </Link>
            );
          })}
        </div>
      </section>
    );
  }

  return (
    <section className="space-y-2">
      <SectionEyebrow>Campaign tuning · all active campaigns</SectionEyebrow>
      <div className="border border-[var(--surface-2)] rounded-xl overflow-hidden">
        <div className={`${tableStyles.grid} pl-4 pr-3 py-2 bg-[var(--surface-2)]/40 text-[10px] uppercase tracking-wider font-semibold text-[var(--text-muted)]`}>
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
              data-severity={row.status.toLowerCase()}
              className={`${tableStyles.grid} pl-3 pr-3 py-2.5 border-t border-l-2 border-[var(--surface-2)] text-sm items-center hover:bg-[var(--surface-2)]/30`}
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
