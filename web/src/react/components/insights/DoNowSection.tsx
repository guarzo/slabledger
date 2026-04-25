import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import { RecommendationBadge } from '../../ui/RecommendationBadge';
import type { Action, Severity } from '../../../types/insights';

const SEVERITY_ORDER: Record<Severity, number> = { act: 0, tune: 1, ok: 2 };

const stripColor: Record<Severity, string> = {
  act: 'border-l-[var(--danger)]',
  tune: 'border-l-[var(--warning)]',
  ok: 'border-l-[var(--success)]',
};

const severityLabel: Record<Severity, string> = {
  act: 'Action',
  tune: 'Tune',
  ok: 'OK',
};

function sortByUrgency(actions: Action[]): Action[] {
  return [...actions].sort((a, b) => {
    const oa = SEVERITY_ORDER[a.severity] ?? 99;
    const ob = SEVERITY_ORDER[b.severity] ?? 99;
    return oa - ob;
  });
}

export default function DoNowSection({ actions }: { actions: Action[] }) {
  const sorted = useMemo(() => sortByUrgency(actions), [actions]);
  if (actions.length === 0) {
    return (
      <section className="space-y-2">
        <div className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]">Do now</div>
        <div className="p-4 rounded-xl border border-[var(--surface-2)] bg-[var(--surface-1)] text-sm text-[var(--text-muted)]">
          Nothing needs attention — your campaigns are healthy.
        </div>
      </section>
    );
  }
  return (
    <section className="space-y-2">
      <div className="text-[11px] font-bold uppercase tracking-wider text-[var(--danger)]">Do now</div>
      <div className="border border-[var(--surface-2)] rounded-xl overflow-hidden">
        {sorted.map((a, i) => (
          <div
            key={a.id}
            className={`flex items-center gap-3 pl-3 pr-3 py-2.5 border-l-2 ${stripColor[a.severity] ?? 'border-l-[var(--surface-3)]'} ${
              i + 1 < sorted.length ? 'border-b border-b-[var(--surface-2)]' : ''
            }`}
          >
            <div className="flex-1 text-sm min-w-0">
              <span className="font-semibold">{a.title}</span>
              <span className="text-[var(--text-muted)]"> · {a.detail}</span>
            </div>
            <RecommendationBadge label={severityLabel[a.severity] ?? 'OK'} severity={a.severity} />
            <Link
              to={{ pathname: a.link.path, search: queryString(a.link.query) }}
              aria-label={`Open: ${a.title}`}
              className="text-xs text-[var(--brand-400)] hover:underline"
            >
              Open →
            </Link>
          </div>
        ))}
      </div>
    </section>
  );
}

function queryString(q?: Record<string, string>): string {
  if (!q) return '';
  const params = new URLSearchParams(q);
  const s = params.toString();
  return s ? `?${s}` : '';
}
