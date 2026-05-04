import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import { RecommendationBadge } from '../../ui/RecommendationBadge';
import SectionEyebrow from '../../ui/SectionEyebrow';
import EmptyState from '../../ui/EmptyState';
import type { Action, Severity } from '../../../types/insights';

const SEVERITY_ORDER: Record<Severity, number> = { act: 0, tune: 1, ok: 2 };

const severityLabel: Record<Severity, string> = {
  act: 'Action',
  tune: 'Tune',
  ok: 'OK',
};

/** Visible card budget on the recommendation grid. The remaining items still
    render as compact rows below the card grid so nothing is hidden, but only
    the top N get the prominent slab-framed treatment. Three is the cognitive
    load ceiling per the friction-log iter-20 onboarding work. */
const VISIBLE_CARDS = 3;

function sortByUrgency(actions: Action[]): Action[] {
  return [...actions].sort((a, b) => {
    const oa = SEVERITY_ORDER[a.severity] ?? 99;
    const ob = SEVERITY_ORDER[b.severity] ?? 99;
    return oa - ob;
  });
}

function queryString(q?: Record<string, string>): string {
  if (!q) return '';
  const params = new URLSearchParams(q);
  const s = params.toString();
  return s ? `?${s}` : '';
}

export default function DoNowSection({ actions }: { actions: Action[] }) {
  const sorted = useMemo(() => sortByUrgency(actions), [actions]);
  if (actions.length === 0) {
    return (
      <section className="space-y-2">
        <SectionEyebrow>Do now</SectionEyebrow>
        <EmptyState
          title="Nothing needs attention"
          description="Your campaigns are healthy."
          compact
        />
      </section>
    );
  }

  const cards = sorted.slice(0, VISIBLE_CARDS);
  const overflow = sorted.slice(VISIBLE_CARDS);

  return (
    <section className="space-y-3">
      <SectionEyebrow tone="danger">Do now</SectionEyebrow>

      {/* Prominent card grid — three across on desktop, stacks on mobile.
          Each card is a slab-framed link so the whole surface is clickable
          and the keyboard target matches the visual target. */}
      <div className="grid gap-3 grid-cols-1 md:grid-cols-2 lg:grid-cols-3">
        {cards.map((a) => (
          <RecommendationCard key={a.id} action={a} />
        ))}
      </div>

      {/* Anything beyond the card budget still renders, but as compact rows
          beneath so the cognitive load of the page-leading section stays
          tight. Same severity bar + Open link so the contract holds. */}
      {overflow.length > 0 && (
        <div className="border border-[var(--surface-2)] rounded-xl overflow-hidden">
          {overflow.map((a, i) => (
            <div
              key={a.id}
              data-severity={a.severity}
              className={`flex items-center gap-3 pl-3 pr-3 py-2.5 border-l-2 ${
                i + 1 < overflow.length ? 'border-b border-b-[var(--surface-2)]' : ''
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
      )}
    </section>
  );
}

/** A single recommendation tile. Slab-framed (Phase 1 utility, first real
    placement on a content surface), severity-tinted left edge, Fraunces title
    so the campaign name reads as a subject rather than a row. */
function RecommendationCard({ action }: { action: Action }) {
  return (
    <Link
      to={{ pathname: action.link.path, search: queryString(action.link.query) }}
      aria-label={`Open: ${action.title}`}
      data-severity={action.severity}
      className="group relative block rounded-xl bg-[var(--surface-1)] p-4 pl-5
                 border border-[var(--surface-2)] hover:border-[var(--brand-500)]/60
                 transition-colors focus-ring focus-visible:outline-2"
    >
      {/* Severity-coloured left edge — same data-severity hook the existing
          rule in base.css paints, so the colour token stays centralised. */}
      <span
        aria-hidden="true"
        className="absolute left-0 top-3 bottom-3 w-[3px] rounded-r-sm"
        style={{
          background:
            action.severity === 'act' ? 'var(--danger)' :
            action.severity === 'tune' ? 'var(--warning)' :
            'var(--success)',
        }}
      />
      <div className="flex items-baseline justify-between gap-3 mb-1.5">
        <RecommendationBadge label={severityLabel[action.severity] ?? 'OK'} severity={action.severity} />
        <span className="text-xs text-[var(--text-muted)] group-hover:text-[var(--brand-400)] transition-colors whitespace-nowrap">
          Open →
        </span>
      </div>
      <h3
        className="text-base font-semibold text-[var(--text)] mb-1 truncate"
        style={{ fontFamily: 'var(--font-display)', fontWeight: 500, fontSize: '1.125rem' }}
        title={action.title}
      >
        {action.title}
      </h3>
      <p className="text-xs text-[var(--text-muted)] leading-relaxed line-clamp-3">
        {action.detail}
      </p>
    </Link>
  );
}
