import { Link } from 'react-router-dom';
import { RecommendationBadge } from '../../ui/RecommendationBadge';
import { StatusPill, type StatusTone } from '../../ui/StatusPill';
import SectionEyebrow from '../../ui/SectionEyebrow';
import EmptyState from '../../ui/EmptyState';
import type { Status, TuningColumn, TuningRow } from '../../../types/insights';
import { useMediaQuery } from '../../hooks/useMediaQuery';
import tableStyles from './CampaignTuningTable.module.css';

const STATUS_META: Record<Status, { label: string; tone: StatusTone }> = {
  Act:  { label: 'Action', tone: 'danger' },
  Kill: { label: 'Kill',   tone: 'danger' },
  Tune: { label: 'Tune',   tone: 'warning' },
  OK:   { label: 'OK',     tone: 'success' },
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

const columns: Array<{ key: TuningColumn; label: string; description: string }> = [
  { key: 'buyPct',     label: 'Buy %',      description: 'Buy threshold relative to CardLadder. Higher = more selective on purchases.' },
  { key: 'characters', label: 'Characters', description: 'Inclusion list size and breadth. Recommendation reflects whether the list is too narrow or wide given fill rate.' },
  { key: 'years',      label: 'Years',      description: 'Year range targeted. Recommendation reflects whether the range is too narrow or wide given fill rate.' },
  { key: 'spendCap',   label: 'Spend cap',  description: 'Daily spend ceiling. Recommendation reflects whether the cap is constraining or oversized for current sell-through.' },
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
                  <StatusPill tone={meta.tone} size="xs">{meta.label}</StatusPill>
                </div>
                <dl className="grid grid-cols-2 gap-x-3 gap-y-1.5 text-xs">
                  {columns.map((c) => {
                    const cell = row.cells[c.key];
                    return (
                      <div key={c.key} className="flex items-center justify-between gap-2">
                        <dt className="text-[var(--text-muted)] uppercase tracking-wider text-[10px]" title={c.description}>{c.label}</dt>
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
            <abbr
              key={c.key}
              title={c.description}
              style={{ textDecoration: 'none', cursor: 'help' }}
            >
              {c.label}
            </abbr>
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
                <StatusPill tone={meta.tone} size="xs">{meta.label}</StatusPill>
              </div>
            </Link>
          );
        })}
      </div>
    </section>
  );
}
