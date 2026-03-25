/**
 * StatusIndicator - Small dot in the header that shows API health.
 * Green = all healthy, Yellow = degraded, Red = any source down.
 * Clicks through to /status page.
 */
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

export default function StatusIndicator() {
  const { user, loading: authLoading } = useAuth();
  const isAdmin = !authLoading && !!user?.is_admin;
  const { data, isLoading, isError } = useAdminApiUsage({ enabled: isAdmin });

  // Only show to admins
  if (isLoading || !isAdmin) return null;

  const health = isError ? 'degraded' : computeHealth(data?.providers ?? []);

  return (
    <Link
      to="/admin"
      className="relative flex items-center justify-center w-8 h-8 rounded-full hover:bg-[var(--surface-2)]/60 transition-colors"
      aria-label={healthLabels[health]}
      title={healthLabels[health]}
    >
      <span className={`w-2.5 h-2.5 rounded-full ${healthColors[health]}`} />
      {health !== 'healthy' && (
        <span className={`absolute w-2.5 h-2.5 rounded-full ${healthColors[health]} animate-ping opacity-75`} />
      )}
    </Link>
  );
}
