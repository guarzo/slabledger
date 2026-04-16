import { useMemo, useState } from 'react';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { splitByH2 } from './splitByH2';

const remarkPlugins = [remarkGfm];

const markdownComponents = {
  table: ({ children, ...props }: React.ComponentPropsWithoutRef<'table'>) => (
    <div className="table-wrap">
      <table {...props}>{children}</table>
    </div>
  ),
};

const BODY_PROSE = [
  'prose prose-sm prose-invert max-w-none text-[var(--text)] text-sm leading-relaxed',
  '[&_h3]:text-sm [&_h3]:font-semibold [&_h3]:text-[var(--text)] [&_h3]:mt-4 [&_h3]:mb-2',
  '[&_h4]:text-xs [&_h4]:font-semibold [&_h4]:text-[var(--text-muted)] [&_h4]:uppercase [&_h4]:tracking-wide [&_h4]:mt-3 [&_h4]:mb-1',
  '[&_strong]:text-[var(--text)]',
  '[&_ul]:space-y-1.5 [&_ul]:my-2 [&_ol]:space-y-1.5 [&_ol]:my-2 [&_li]:text-[var(--text)] [&_li]:leading-relaxed',
  '[&_p]:mb-3 [&_p]:leading-relaxed',
  '[&_.table-wrap]:overflow-x-auto [&_.table-wrap]:my-3 [&_.table-wrap]:rounded-lg [&_.table-wrap]:border [&_.table-wrap]:border-[var(--surface-2)]',
  '[&_table]:w-full [&_table]:text-xs [&_table]:border-collapse',
  '[&_th]:text-left [&_th]:text-[var(--text-muted)] [&_th]:font-semibold [&_th]:px-3 [&_th]:py-2 [&_th]:border-b [&_th]:border-[var(--surface-2)] [&_th]:bg-[var(--surface-2)]/50 [&_th]:whitespace-nowrap',
  '[&_td]:px-3 [&_td]:py-2 [&_td]:border-b [&_td]:border-[var(--surface-2)]/50 [&_td]:align-top',
  '[&_tr:last-child_td]:border-b-0',
  '[&_tr:hover_td]:bg-[var(--surface-2)]/30',
  '[&_code]:text-[var(--brand-400)] [&_code]:text-xs',
].join(' ');

function normalize(heading: string): string {
  return heading.toLowerCase().replace(/\s+/g, ' ').trim();
}

export interface SectionSchema {
  heading: string;
  icon?: string;
}

interface SectionedReportProps {
  /** The full markdown returned by the LLM. */
  markdown: string;
  /** Ordered list of H2 headings the LLM was told to produce. */
  schema: SectionSchema[];
  /** Key used to namespace collapse state in localStorage. */
  cacheKey: string;
}

/**
 * Renders LLM-produced markdown as a series of collapsible, schema-matched cards.
 * Sections listed in `schema` render in schema order; unmatched headings from the
 * LLM render last under "Additional Notes" so no content is ever hidden. Schema
 * entries with no matching heading render a "Not generated this run" placeholder
 * so the user can see what the LLM skipped.
 */
export default function SectionedReport({ markdown, schema, cacheKey }: SectionedReportProps) {
  const sections = useMemo(() => splitByH2(markdown), [markdown]);

  const byNormalizedHeading = useMemo(() => {
    const map = new Map<string, string>();
    for (const s of sections) {
      if (s.heading) map.set(normalize(s.heading), s.body);
    }
    return map;
  }, [sections]);

  const schemaKeys = useMemo(() => new Set(schema.map(s => normalize(s.heading))), [schema]);
  const preambleBody = useMemo(() => sections.find(s => !s.heading)?.body ?? '', [sections]);
  const extras = useMemo(() => sections.filter(s => s.heading && !schemaKeys.has(normalize(s.heading))), [sections, schemaKeys]);

  return (
    <div className="flex flex-col gap-3">
      {preambleBody && (
        <SectionCard heading="Summary" body={preambleBody} cacheKey={`${cacheKey}:preamble`} />
      )}

      {schema.map(({ heading, icon }) => {
        const body = byNormalizedHeading.get(normalize(heading)) ?? '';
        return (
          <SectionCard
            key={heading}
            heading={heading}
            icon={icon}
            body={body}
            cacheKey={`${cacheKey}:${normalize(heading)}`}
          />
        );
      })}

      {extras.length > 0 && (
        <div className="mt-2 p-3 bg-[var(--surface-2)]/30 rounded-xl border border-dashed border-[var(--surface-2)]">
          <div className="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)] mb-2">
            Additional Notes (unmatched sections)
          </div>
          {extras.map(s => (
            <div key={s.heading} className="mb-3 last:mb-0">
              <div className="text-sm font-semibold text-[var(--text)] mb-1">{s.heading}</div>
              <div className={BODY_PROSE}>
                <Markdown remarkPlugins={remarkPlugins} components={markdownComponents}>{s.body}</Markdown>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function SectionCard({ heading, icon, body, cacheKey }: { heading: string; icon?: string; body: string; cacheKey: string }) {
  const [collapsed, setCollapsed] = useLocalStorageBool(cacheKey, false);
  const isMissing = body.length === 0;

  return (
    <section className="bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)] overflow-hidden">
      <header className="flex items-center justify-between gap-2 px-4 py-3 border-b border-[var(--surface-2)]/50">
        <h3 className="flex items-center gap-2 text-sm font-semibold text-[var(--text)]">
          {icon && <span aria-hidden="true" className="text-base">{icon}</span>}
          <span>{heading}</span>
          {isMissing && (
            <span className="text-[10px] font-medium text-[var(--text-muted)] bg-[var(--surface-2)] px-1.5 py-0.5 rounded">
              not generated
            </span>
          )}
        </h3>
        {!isMissing && (
          <button
            type="button"
            onClick={() => setCollapsed(c => !c)}
            className="text-[var(--text-muted)] hover:text-[var(--brand-500)] transition-colors"
            aria-expanded={!collapsed}
            aria-label={collapsed ? `Expand ${heading}` : `Collapse ${heading}`}
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
              className={`w-4 h-4 transition-transform duration-200 ${collapsed ? '' : 'rotate-180'}`}
            >
              <polyline points="6 9 12 15 18 9" />
            </svg>
          </button>
        )}
      </header>
      {!collapsed && (
        <div className="px-4 py-3">
          {isMissing ? (
            <p className="text-xs text-[var(--text-muted)]">
              The LLM did not produce this section on the current run. Refresh to try again.
            </p>
          ) : (
            <div className={BODY_PROSE}>
              <Markdown remarkPlugins={remarkPlugins} components={markdownComponents}>{body}</Markdown>
            </div>
          )}
        </div>
      )}
    </section>
  );
}

function useLocalStorageBool(key: string, initial: boolean): [boolean, (updater: (prev: boolean) => boolean) => void] {
  const storageKey = `sectioned-report-collapsed:${key}`;
  const [value, setValue] = useState<boolean>(() => {
    try {
      const stored = localStorage.getItem(storageKey);
      return stored === null ? initial : stored === 'true';
    } catch {
      return initial;
    }
  });
  const update = (updater: (prev: boolean) => boolean) => {
    setValue(prev => {
      const next = updater(prev);
      try { localStorage.setItem(storageKey, String(next)); } catch { /* noop */ }
      return next;
    });
  };
  return [value, update];
}
