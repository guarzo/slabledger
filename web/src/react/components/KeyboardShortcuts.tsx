/**
 * Global keyboard shortcuts.
 *
 * - `?` opens a cheatsheet overlay.
 * - `g` then a single key navigates: `d` dashboard, `c` campaigns, `i` inventory,
 *   `t` tools (scan), `v` invoices.
 *
 * `Cmd/Ctrl-K` is intentionally NOT handled here — Header owns the real
 * CommandPalette. The cheatsheet still lists ⌘K so users know it exists.
 *
 * All bindings ignore presses while focus is in an editable surface (input,
 * textarea, contenteditable) so typing into forms isn't hijacked.
 */
import { useEffect, useRef, useState, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { Modal } from '../ui';

const NAV_BINDINGS: Array<{ key: string; path: string; label: string }> = [
  { key: 'd', path: '/', label: 'Dashboard' },
  { key: 'c', path: '/campaigns', label: 'Campaigns' },
  { key: 'i', path: '/inventory', label: 'Inventory' },
  { key: 't', path: '/scan', label: 'Tools' },
  { key: 'v', path: '/invoices', label: 'Invoices' },
];

function isMacPlatform(): boolean {
  if (typeof navigator === 'undefined') return false;
  // Prefer modern userAgentData when available; fall back to platform/userAgent.
  const uaData = (navigator as unknown as { userAgentData?: { platform?: string } }).userAgentData;
  const platform = uaData?.platform || navigator.platform || navigator.userAgent || '';
  return /mac|iphone|ipad|ipod/i.test(platform);
}

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
  const [gPending, setGPending] = useState(false);
  const gTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const clearGTimer = useCallback(() => {
    if (gTimerRef.current) {
      clearTimeout(gTimerRef.current);
      gTimerRef.current = null;
    }
  }, []);

  const closeAll = useCallback(() => {
    setShowHelp(false);
    setGPending(false);
    clearGTimer();
  }, [clearGTimer]);

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      // Skip if another handler already consumed this event.
      if (e.defaultPrevented) return;

      // ⌘K is owned by Header (real CommandPalette); don't double-toggle here.

      // While the cheatsheet is open it owns the keyboard — Modal closes it on
      // Escape — and we don't want `?` to re-toggle or `g`+key to navigate from
      // a modal.
      if (showHelp) return;

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
        clearGTimer();
        if (match) {
          e.preventDefault();
          navigate(match.path);
        }
        return;
      }

      if (e.key === 'g') {
        setGPending(true);
        clearGTimer();
        gTimerRef.current = setTimeout(() => {
          setGPending(false);
          gTimerRef.current = null;
        }, 1200);
      }
    }

    window.addEventListener('keydown', onKey);
    return () => {
      window.removeEventListener('keydown', onKey);
    };
  }, [gPending, showHelp, navigate, closeAll, clearGTimer]);

  // Clear any pending g-timer on unmount.
  useEffect(() => clearGTimer, [clearGTimer]);

  if (!showHelp && !gPending) return null;

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

      {showHelp && (
        <Modal onClose={closeAll} title="Keyboard shortcuts" size="lg" showClose>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-2 text-sm">
            <ShortcutRow keys={['?']} label="Toggle this cheatsheet" />
            <ShortcutRow keys={['Esc']} label="Close overlay" />
            <ShortcutRow keys={isMacPlatform() ? ['⌘', 'K'] : ['Ctrl', 'K']} label="Command palette" />
            <ShortcutRow keys={['/']} label="Focus search (page-local)" />
            {NAV_BINDINGS.map((b) => (
              <ShortcutRow key={b.key} keys={['g', b.key]} label={b.label} />
            ))}
          </div>
        </Modal>
      )}
    </>
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
