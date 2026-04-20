import { Link } from 'react-router-dom';
import type { Action } from '../../../types/insights';

const dotColor = {
  act: 'bg-[var(--danger)]',
  tune: 'bg-[var(--warning)]',
  ok: 'bg-[var(--success)]',
} as const;

export default function DoNowSection({ actions }: { actions: Action[] }) {
  if (actions.length === 0) {
    return (
      <section className="space-y-2">
        <div className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]">Do now</div>
        <div className="p-4 rounded-xl border border-[var(--surface-2)] bg-[var(--surface-1)] text-sm text-[var(--text-muted)]">
          Nothing needs your attention right now.
        </div>
      </section>
    );
  }
  return (
    <section className="space-y-2">
      <div className="text-[11px] font-bold uppercase tracking-wider text-[var(--danger)]">Do now</div>
      <div className="border border-[var(--surface-2)] rounded-xl overflow-hidden">
        {actions.map((a, i) => (
          <div
            key={a.id}
            className={`flex items-center gap-3 px-3 py-2.5 ${
              i + 1 < actions.length ? 'border-b border-[var(--surface-2)]' : ''
            }`}
          >
            <span className={`w-1.5 h-1.5 rounded-full ${dotColor[a.severity]}`} aria-hidden />
            <div className="flex-1 text-sm">
              <span className="font-semibold">{a.title}</span>
              <span className="text-[var(--text-muted)]"> · {a.detail}</span>
            </div>
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
