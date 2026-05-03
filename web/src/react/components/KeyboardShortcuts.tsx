/**
 * Global keyboard shortcuts.
 *
 * - `?` opens a cheatsheet overlay.
 * - `g` then a single key navigates: `d` dashboard, `c` campaigns, `i` inventory,
 *   `n` insights, `t` tools (scan), `s` sell sheet.
 * - `Cmd/Ctrl-K` opens a stub command palette (placeholder for future search).
 *
 * All bindings ignore presses while focus is in an editable surface (input,
 * textarea, contenteditable) so typing into forms isn't hijacked.
 */
import { useEffect, useState, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';

const NAV_BINDINGS: Array<{ key: string; path: string; label: string }> = [
  { key: 'd', path: '/', label: 'Dashboard' },
  { key: 'c', path: '/campaigns', label: 'Campaigns' },
  { key: 'i', path: '/inventory', label: 'Inventory' },
  { key: 'n', path: '/insights', label: 'Insights' },
  { key: 't', path: '/scan', label: 'Tools' },
  { key: 's', path: '/sell-sheet', label: 'Sell Sheet' },
  { key: 'v', path: '/invoices', label: 'Invoices' },
];

function isEditable(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName;
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true;
  if (target.isContentEditable) return true;
  return false;
}

export default function KeyboardShortcuts() {
  const navigate = useNavigate();
  const [showHelp, setShowHelp] = useState(false);
  const [showPalette, setShowPalette] = useState(false);
  const [gPending, setGPending] = useState(false);

  const closeAll = useCallback(() => {
    setShowHelp(false);
    setShowPalette(false);
    setGPending(false);
  }, []);

  useEffect(() => {
    let gTimer: ReturnType<typeof setTimeout> | null = null;

    function onKey(e: KeyboardEvent) {
      // Cmd/Ctrl-K — palette stub. Allow even in inputs.
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setShowPalette((v) => !v);
        setShowHelp(false);
        return;
      }

      if (e.key === 'Escape') {
        closeAll();
        return;
      }

      if (isEditable(e.target)) return;
      if (e.metaKey || e.ctrlKey || e.altKey) return;

      if (e.key === '?') {
        e.preventDefault();
        setShowHelp((v) => !v);
        return;
      }

      if (gPending) {
        const match = NAV_BINDINGS.find((b) => b.key === e.key.toLowerCase());
        setGPending(false);
        if (gTimer) clearTimeout(gTimer);
        if (match) {
          e.preventDefault();
          navigate(match.path);
        }
        return;
      }

      if (e.key === 'g') {
        setGPending(true);
        gTimer = setTimeout(() => setGPending(false), 1200);
      }
    }

    window.addEventListener('keydown', onKey);
    return () => {
      window.removeEventListener('keydown', onKey);
      if (gTimer) clearTimeout(gTimer);
    };
  }, [gPending, navigate, closeAll]);

  if (!showHelp && !showPalette && !gPending) return null;

  return (
    <>
      {gPending && (
        <div
          className="fixed bottom-4 left-1/2 -translate-x-1/2 z-50 px-3 py-1.5 rounded-md bg-[var(--surface-2)] border border-[var(--border-subtle)] text-xs font-mono text-[var(--text-muted)]"
          role="status"
          aria-live="polite"
        >
          g…
        </div>
      )}

      {showPalette && (
        <Overlay onClose={closeAll} title="Command palette">
          <p className="text-sm text-[var(--text-muted)]">
            Search is not wired up yet. Use <Kbd>?</Kbd> for keyboard shortcuts or
            <Kbd>g</Kbd> then a nav key.
          </p>
        </Overlay>
      )}

      {showHelp && (
        <Overlay onClose={closeAll} title="Keyboard shortcuts">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-2 text-sm">
            <ShortcutRow keys={['?']} label="Toggle this cheatsheet" />
            <ShortcutRow keys={['Esc']} label="Close overlay" />
            <ShortcutRow keys={['⌘', 'K']} label="Command palette" />
            <ShortcutRow keys={['/']} label="Focus search (page-local)" />
            {NAV_BINDINGS.map((b) => (
              <ShortcutRow key={b.key} keys={['g', b.key]} label={b.label} />
            ))}
          </div>
        </Overlay>
      )}
    </>
  );
}

function Overlay({ children, onClose, title }: { children: React.ReactNode; onClose: () => void; title: string }) {
  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center pt-24 px-4 bg-black/60"
      role="dialog"
      aria-modal="true"
      aria-label={title}
      onClick={onClose}
    >
      <div
        className="w-full max-w-lg rounded-xl bg-[var(--surface-1)] border border-[var(--surface-2)] p-5 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-baseline justify-between mb-3">
          <h2 className="text-sm font-semibold uppercase tracking-wider text-[var(--text-muted)]">{title}</h2>
          <button
            onClick={onClose}
            className="text-xs text-[var(--text-muted)] hover:text-[var(--text)]"
            aria-label="Close"
          >
            Esc
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}

function ShortcutRow({ keys, label }: { keys: string[]; label: string }) {
  return (
    <div className="flex items-center justify-between gap-3 py-1">
      <span className="text-[var(--text)]">{label}</span>
      <span className="flex items-center gap-1">
        {keys.map((k, i) => (
          <span key={i} className="flex items-center gap-1">
            {i > 0 && <span className="text-[var(--text-muted)] text-xs">then</span>}
            <Kbd>{k}</Kbd>
          </span>
        ))}
      </span>
    </div>
  );
}

function Kbd({ children }: { children: React.ReactNode }) {
  return (
    <kbd className="px-1.5 py-0.5 rounded border border-[var(--surface-2)] bg-[var(--surface-2)]/40 text-[11px] font-mono text-[var(--text)]">
      {children}
    </kbd>
  );
}
