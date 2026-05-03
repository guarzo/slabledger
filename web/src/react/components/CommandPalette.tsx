import { useEffect, useMemo, useRef, useState } from 'react';
import { Dialog } from 'radix-ui';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';
import { paletteItems, type NavItem } from './navConfig';

interface CommandPaletteProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

function score(item: NavItem, query: string): number {
  if (!query) return 1;
  const q = query.toLowerCase();
  const haystacks = [item.label, item.shortLabel, item.description ?? ''].map((s) => s.toLowerCase());
  let best = 0;
  for (const h of haystacks) {
    if (h === q) best = Math.max(best, 100);
    else if (h.startsWith(q)) best = Math.max(best, 60);
    else if (h.includes(q)) best = Math.max(best, 30);
  }
  return best;
}

export default function CommandPalette({ open, onOpenChange }: CommandPaletteProps) {
  const navigate = useNavigate();
  const { user } = useAuth();
  const isAdmin = !!user?.is_admin;
  const inputRef = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState('');
  const [activeIndex, setActiveIndex] = useState(0);

  const results = useMemo(() => {
    const items = paletteItems(isAdmin);
    if (!query.trim()) return items;
    return items
      .map((item) => ({ item, s: score(item, query) }))
      .filter((r) => r.s > 0)
      .sort((a, b) => b.s - a.s)
      .map((r) => r.item);
  }, [query, isAdmin]);

  useEffect(() => {
    if (open) {
      setQuery('');
      setActiveIndex(0);
      const id = window.setTimeout(() => inputRef.current?.focus(), 0);
      return () => window.clearTimeout(id);
    }
    return undefined;
  }, [open]);

  useEffect(() => {
    setActiveIndex(0);
  }, [query]);

  const goToResult = (item: NavItem) => {
    onOpenChange(false);
    navigate(item.path);
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setActiveIndex((i) => Math.min(i + 1, Math.max(0, results.length - 1)));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setActiveIndex((i) => Math.max(0, i - 1));
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const item = results[activeIndex];
      if (item) goToResult(item);
    }
  };

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-[var(--surface-overlay)] data-[state=open]:animate-[fadeIn_150ms_ease-out]" />
        <Dialog.Content
          className="fixed left-1/2 top-[18vh] -translate-x-1/2 z-50 w-[min(560px,calc(100%-2rem))] bg-[var(--surface-1)]/90 backdrop-blur-xl border border-[var(--surface-2)] rounded-[var(--radius-xl)] shadow-[var(--shadow-3)] flex flex-col overflow-hidden data-[state=open]:animate-[fadeIn_150ms_ease-out]"
          aria-label="Command palette"
        >
          <Dialog.Title className="sr-only">Jump to a page</Dialog.Title>
          <Dialog.Description className="sr-only">
            Type to filter destinations, then press Enter to navigate.
          </Dialog.Description>

          <div className="flex items-center gap-3 px-5 py-4 border-b border-[var(--surface-2)]">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-[var(--text-subtle)]" aria-hidden="true">
              <circle cx="11" cy="11" r="7" />
              <path d="m20 20-3.5-3.5" />
            </svg>
            <input
              ref={inputRef}
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={onKeyDown}
              placeholder="Jump to a page…"
              className="flex-1 bg-transparent text-[var(--text)] placeholder-[var(--text-subtle)] text-base outline-none"
              aria-autocomplete="list"
              aria-controls="command-palette-results"
              aria-activedescendant={results[activeIndex] ? `cmd-item-${activeIndex}` : undefined}
            />
            <span className="text-2xs uppercase tracking-wider text-[var(--text-subtle)] font-mono">Esc</span>
          </div>

          <ul id="command-palette-results" role="listbox" className="max-h-[60vh] overflow-y-auto py-1">
            {results.length === 0 ? (
              <li className="px-5 py-4 text-sm text-[var(--text-muted)]">No matches</li>
            ) : (
              results.map((item, idx) => {
                const isActive = idx === activeIndex;
                return (
                  <li
                    key={item.path}
                    id={`cmd-item-${idx}`}
                    role="option"
                    aria-selected={isActive}
                  >
                    <button
                      type="button"
                      onMouseEnter={() => setActiveIndex(idx)}
                      onClick={() => goToResult(item)}
                      className={`w-full flex items-center justify-between gap-4 px-5 py-2.5 text-left transition-colors ${
                        isActive ? 'bg-[var(--brand-500)]/15' : 'hover:bg-[var(--surface-2)]/40'
                      }`}
                    >
                      <span className="flex items-baseline gap-3 min-w-0">
                        <span className="text-sm font-medium text-[var(--text)] truncate">{item.label}</span>
                        {item.description && (
                          <span className="text-xs text-[var(--text-subtle)] truncate">{item.description}</span>
                        )}
                      </span>
                      <span className="text-2xs uppercase tracking-wider text-[var(--text-subtle)] font-mono whitespace-nowrap">
                        {item.path}
                      </span>
                    </button>
                  </li>
                );
              })
            )}
          </ul>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
