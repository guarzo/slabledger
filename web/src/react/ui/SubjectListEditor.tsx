import { useEffect, useId, useMemo, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from '../../js/api';
import { queryKeys } from '../queries/queryKeys';
import type { SubjectRef } from '../../types/campaigns';
import { LEGACY_UNRECONCILED_SUBJECT_ID } from '../utils/campaignConstants';

const CATALOG_MAX_AGE_MS = 7 * 24 * 60 * 60 * 1000;

interface SubjectListEditorProps {
  label: string;
  value: SubjectRef[];
  onChange: (next: SubjectRef[]) => void;
  inputSize?: 'sm';
}

export default function SubjectListEditor({ label, value, onChange, inputSize }: SubjectListEditorProps) {
  const { data, isLoading, isError } = useQuery({
    queryKey: queryKeys.psaSubjects.list,
    queryFn: () => api.listPSASubjects(),
    staleTime: 5 * 60 * 1000,
  });

  const [query, setQuery] = useState('');
  const [open, setOpen] = useState(false);
  // Names of legacy (-1) subjects the operator confirmed removing, lower-cased.
  // Blocking re-entry is what keeps "remove then retype" from becoming a hand
  // repair: a retyped name resolves case-insensitively at push time
  // (resolver.go:146) and can land on a *different* portal subject that happens
  // to share the name. Session-scoped by design — see the design doc's
  // "Known limit".
  const [removedLegacyNames, setRemovedLegacyNames] = useState<Set<string>>(new Set());
  const containerRef = useRef<HTMLDivElement>(null);
  const inputId = useId();

  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (!containerRef.current?.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  // A never-harvested catalog reads back as { subjects: null, fetchedAt: zero-time }
  // (see PSAPortalCatalogStore.Subjects) rather than an error — treat a missing or
  // empty subjects array the same as "not yet harvested".
  // Memoized so the `matches` useMemo below only recomputes when the underlying
  // data actually changes, not the `?? []` fallback's fresh identity each render.
  const catalog = useMemo(() => data?.subjects ?? [], [data]);
  const fetchedAt = data?.fetchedAt;
  const isStale = !!fetchedAt && Date.now() - new Date(fetchedAt).getTime() > CATALOG_MAX_AGE_MS;

  const matches = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return [];
    // Suppress by id AND by name. Id alone is not enough: an operator-typed
    // subject carries id 0, so a catalog entry with the same name has a
    // different id and would still be offered — and accepting it would put
    // two entries with the same name on the campaign.
    const selectedIds = new Set(value.filter(s => s.id !== 0).map(s => s.id));
    const selectedNames = new Set(value.map(s => s.name.toLowerCase()));
    return catalog
      .filter(s => s.name.toLowerCase().includes(q)
        && !selectedIds.has(s.id)
        && !selectedNames.has(s.name.toLowerCase())
        && !removedLegacyNames.has(s.name.toLowerCase()))
      .slice(0, 20);
  }, [query, catalog, value, removedLegacyNames]);

  function addSubject(subject: SubjectRef) {
    // Enter-to-add bypasses the dropdown, so the removed-legacy block needs
    // enforcing here as well as in `matches`.
    if (removedLegacyNames.has(subject.name.toLowerCase())) {
      setQuery('');
      setOpen(false);
      return;
    }
    // Same two-sided check as `matches`: Enter-to-add can reach this with a
    // typed name that duplicates an already-selected catalog subject, which
    // an id-only comparison would let through.
    const alreadyPresent = value.some(s =>
      (subject.id !== 0 && s.id === subject.id) ||
      s.name.toLowerCase() === subject.name.toLowerCase(),
    );
    if (!alreadyPresent) onChange([...value, subject]);
    setQuery('');
    setOpen(false);
  }

  function removeSubject(index: number) {
    const subject = value[index];
    if (subject?.id === LEGACY_UNRECONCILED_SUBJECT_ID) {
      const ok = window.confirm(
        `Remove "${subject.name}"?\n\n` +
        'This subject has no portal id yet, so removing it drops that targeting — ' +
        'it does not repair it. A real portal id can only come from the harvester ' +
        'baseline pull, and this name cannot be added back on this form.',
      );
      if (!ok) return;
      setRemovedLegacyNames(prev => new Set(prev).add(subject.name.toLowerCase()));
    }
    onChange(value.filter((_, i) => i !== index));
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key !== 'Enter') return;
    e.preventDefault();
    const trimmed = query.trim();
    if (!trimmed) return;
    const exact = matches.find(s => s.name.toLowerCase() === trimmed.toLowerCase());
    addSubject(exact ?? { id: 0, name: trimmed });
  }

  const inputPad = inputSize === 'sm' ? 'py-1.5 text-xs' : 'py-2 text-sm';

  return (
    <div className="space-y-2" ref={containerRef}>
      <label htmlFor={inputId} className="block text-xs text-[var(--text-muted)] mb-1">{label}</label>
      {isLoading && (
        <p className="text-xs text-[var(--text-muted)]">Loading subject catalog…</p>
      )}
      {isError && (
        <p className="text-xs text-[var(--danger)]">
          Could not load the subject catalog — try again later.
        </p>
      )}
      {!isLoading && !isError && catalog.length === 0 && (
        <p className="text-xs text-[var(--text-muted)]">
          Subject catalog not yet harvested — run the harvester.
        </p>
      )}
      {catalog.length > 0 && isStale && (
        <p className="text-xs text-[var(--warning)]">
          Subject catalog is over 7 days old (last fetched {new Date(fetchedAt as string).toLocaleDateString()}).
        </p>
      )}
      <div className="relative">
        <input
          id={inputId}
          type="text"
          value={query}
          onChange={e => { setQuery(e.target.value); setOpen(true); }}
          onFocus={() => setOpen(true)}
          onKeyDown={handleKeyDown}
          placeholder="Add a subject by name…"
          className={`w-full px-4 ${inputPad} text-[var(--text)] bg-[var(--surface-2)] border border-[var(--surface-2)] rounded-lg transition-colors focus:outline-none focus:ring-2 focus:ring-[var(--brand-500)]/20 focus:border-[var(--brand-500)]`}
        />
        {open && matches.length > 0 && (
          <div className="absolute z-20 mt-1 w-full max-h-48 overflow-y-auto rounded-lg border border-[var(--surface-2)] bg-[var(--surface-1)] shadow-lg">
            {matches.map(s => (
              <button
                key={s.id}
                type="button"
                onClick={() => addSubject(s)}
                className="block w-full text-left px-3 py-1.5 text-xs text-[var(--text)] hover:bg-[var(--surface-2)]/60"
              >
                {s.name}
              </button>
            ))}
          </div>
        )}
      </div>
      {value.some(s => s.id === LEGACY_UNRECONCILED_SUBJECT_ID) && (
        <p className="text-xs text-[var(--warning)]">
          Some subjects were carried over from before portal targeting and have no
          portal id yet. Publishing is refused until the harvester baseline pull
          reconciles them.
        </p>
      )}
      <div className="flex flex-wrap gap-1.5">
        {value.map((s, i) => {
          // -1 (legacy, unreconciled) and 0 (operator-typed, resolved by name at
          // push time) are different markers with different fates — only the
          // first one blocks a push, so only it is called out here.
          const isLegacy = s.id === LEGACY_UNRECONCILED_SUBJECT_ID;
          return (
            <span
              key={`${s.id}-${s.name}-${i}`}
              title={isLegacy ? 'legacy subject — no portal id yet; run the harvester baseline pull' : `id: ${s.id}`}
              className={
                isLegacy
                  ? 'inline-flex items-center gap-1 rounded-full bg-[var(--warning)]/15 text-[var(--warning)] text-xs px-2.5 py-1'
                  : 'inline-flex items-center gap-1 rounded-full bg-[var(--brand-500)]/15 text-[var(--brand-400)] text-xs px-2.5 py-1'
              }
            >
              {s.name}
              <button
                type="button"
                onClick={() => removeSubject(i)}
                aria-label={`Remove ${s.name}`}
                className="hover:text-[var(--danger)]"
              >
                ×
              </button>
            </span>
          );
        })}
      </div>
    </div>
  );
}
