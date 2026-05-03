/**
 * StatusIndicator - Small dot in the header that shows API health.
 * Green = all healthy, Yellow = degraded, Red = any source down.
 *
 * Tap/click opens a popover with the health label, last refresh time, and a
 * list of providers — and a link through to /admin. The popover gives mobile
 * users (no hover) a way to read the dot's meaning instead of just seeing a
 * colored circle.
 */
import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import type { ProviderStatus } from '../../types/apiStatus';
import { useAdminApiUsage } from '../queries/useAdminQueries';
import { useAuth } from '../contexts/AuthContext';

type HealthLevel = 'healthy' | 'degraded' | 'down';

function computeHealth(providers: ProviderStatus[]): HealthLevel {
  if (providers.length === 0) return 'healthy';

  const anyBlocked = providers.some(p => p.blocked);
  const anyExhausted = providers.some(p => {
    if (p.today.limit == null) return false;
    const remaining = p.today.remaining != null
      ? p.today.remaining
      : Math.max(p.today.limit - p.today.calls, 0);
    return remaining <= 0;
  });
  if (anyBlocked || anyExhausted) return 'down';

  const anyHighError = providers.some(p => p.today.calls > 0 && p.today.successRate < 90);
  const anyLowBudget = providers.some(p => {
    if (p.today.limit == null || p.today.limit === 0) return p.today.limit === 0;
    const remaining = p.today.remaining != null
      ? p.today.remaining
      : Math.max(p.today.limit - p.today.calls, 0);
    return remaining / p.today.limit < 0.2;
  });
  if (anyHighError || anyLowBudget) return 'degraded';

  return 'healthy';
}

const healthColors: Record<HealthLevel, string> = {
  healthy: 'bg-[var(--success)]',
  degraded: 'bg-[var(--warning)]',
  down: 'bg-[var(--danger)]',
};

const healthLabels: Record<HealthLevel, string> = {
  healthy: 'All API sources healthy',
  degraded: 'Some API sources degraded',
  down: 'API source down or budget exhausted',
};

function providerHealth(p: ProviderStatus): HealthLevel {
  if (p.blocked) return 'down';
  if (p.today.limit != null) {
    const remaining = p.today.remaining != null
      ? p.today.remaining
      : Math.max(p.today.limit - p.today.calls, 0);
    if (remaining <= 0) return 'down';
    if (p.today.limit > 0 && remaining / p.today.limit < 0.2) return 'degraded';
  }
  if (p.today.calls > 0 && p.today.successRate < 90) return 'degraded';
  return 'healthy';
}

export default function StatusIndicator() {
  const { user, loading: authLoading } = useAuth();
  const isAdmin = !authLoading && !!user?.is_admin;
  const { data, isLoading, isError, dataUpdatedAt } = useAdminApiUsage({ enabled: isAdmin });
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    function onDoc(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') setOpen(false);
    }
    document.addEventListener('mousedown', onDoc);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onDoc);
      document.removeEventListener('keydown', onKey);
    };
  }, [open]);

  if (isLoading || !isAdmin) return null;

  const providers = data?.providers ?? [];
  const health = isError ? 'degraded' : computeHealth(providers);
  const label = isError ? 'Failed to load API status' : healthLabels[health];
  const lastChecked = dataUpdatedAt
    ? new Date(dataUpdatedAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    : null;

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen(v => !v)}
        className="relative flex items-center justify-center w-8 h-8 rounded-full hover:bg-[var(--surface-2)]/60 transition-colors"
        aria-label={label}
        aria-expanded={open}
        aria-haspopup="true"
        title={label}
      >
        <span className={`w-2.5 h-2.5 rounded-full ${healthColors[health]}`} />
        {health !== 'healthy' && (
          <span className={`absolute w-2.5 h-2.5 rounded-full ${healthColors[health]} animate-ping opacity-75`} />
        )}
      </button>

      {open && (
        <div
          role="dialog"
          aria-label="API health"
          className="absolute right-0 mt-2 w-64 rounded-lg border border-[var(--surface-2)] bg-[var(--surface-1)] shadow-xl p-3 z-50"
        >
          <div className="flex items-center gap-2 mb-2">
            <span className={`w-2 h-2 rounded-full ${healthColors[health]}`} />
            <span className="text-xs font-semibold text-[var(--text)]">{label}</span>
          </div>
          {lastChecked && (
            <div className="text-[10px] uppercase tracking-wider text-[var(--text-muted)] mb-2">
              Checked {lastChecked}
            </div>
          )}
          {providers.length > 0 ? (
            <ul className="space-y-1 mb-2">
              {providers.map(p => {
                const h = providerHealth(p);
                return (
                  <li key={p.name} className="flex items-center justify-between text-xs">
                    <span className="flex items-center gap-2">
                      <span className={`w-1.5 h-1.5 rounded-full ${healthColors[h]}`} />
                      <span className="text-[var(--text)]">{p.name}</span>
                    </span>
                    <span className="text-[var(--text-muted)] tabular-nums">
                      {p.today.calls}
                      {p.today.limit != null ? ` / ${p.today.limit}` : ''}
                    </span>
                  </li>
                );
              })}
            </ul>
          ) : (
            <div className="text-xs text-[var(--text-muted)] mb-2">No providers configured.</div>
          )}
          <Link
            to="/admin"
            onClick={() => setOpen(false)}
            className="block text-xs text-[var(--info)] hover:underline"
          >
            Open Admin →
          </Link>
        </div>
      )}
    </div>
  );
}
