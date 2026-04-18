import { useCardLadderStatus, useDHStatus, useMarketMoversStatus, usePSASyncStatus } from '../../queries/useAdminQueries';

type TileStatus = 'healthy' | 'warning' | 'unknown';

interface TileProps {
  label: string;
  status: TileStatus;
  metric: string;
  detail?: string;
}

function StatusDot({ status }: { status: TileStatus }) {
  const color =
    status === 'healthy' ? 'var(--success)' :
    status === 'warning' ? 'var(--warning)' :
    'var(--text-muted)';
  return (
    <span
      aria-hidden="true"
      className="inline-block rounded-full"
      style={{ width: 8, height: 8, background: color }}
    />
  );
}

function Tile({ label, status, metric, detail }: TileProps) {
  return (
    <div className="rounded-xl bg-[var(--surface-1)] border border-[var(--surface-2)] px-4 py-3 flex items-center gap-3 min-w-0">
      <StatusDot status={status} />
      <div className="min-w-0 flex-1">
        <div className="text-[11px] font-semibold text-[var(--text-muted)] uppercase tracking-wider truncate">{label}</div>
        <div className="text-sm font-semibold text-[var(--text)] tabular-nums truncate">{metric}</div>
        {detail && <div className="text-[10px] text-[var(--text-muted)] truncate">{detail}</div>}
      </div>
    </div>
  );
}

export function IntegrationHealthStrip({ enabled = true }: { enabled?: boolean }) {
  const { data: dh } = useDHStatus({ enabled });
  const { data: cl } = useCardLadderStatus({ enabled });
  const { data: mm } = useMarketMoversStatus({ enabled });
  const { data: psa } = usePSASyncStatus({ enabled });

  const dhHealth = dh?.api_health;
  const dhStatus: TileStatus = !dh ? 'unknown' : (dhHealth && dhHealth.success_rate < 95 ? 'warning' : 'healthy');
  const dhMetric = dhHealth ? `${dhHealth.success_rate.toFixed(1)}% API` : '—';
  const dhDetail = dhHealth ? `${dhHealth.total_calls.toLocaleString()} calls · ${dhHealth.failures} failures` : undefined;

  const clStatus: TileStatus = !cl ? 'unknown' : (cl.configured ? 'healthy' : 'warning');
  const clMapped = cl?.cardsMapped ?? 0;
  const clMetric = cl?.configured ? `${clMapped} mapped` : 'Not configured';
  const clStale = cl?.priceStats?.staleCount ?? 0;
  const clDetail = clStale > 0 ? `${clStale} stale (>7d)` : undefined;

  const mmPriced = mm?.priceStats?.withMMPrice ?? 0;
  const mmTotal = mm?.priceStats?.unsoldTotal ?? 0;
  const mmStale = mm?.priceStats?.staleCount ?? 0;
  const mmStatus: TileStatus = !mm ? 'unknown' : (mm.configured ? (mmStale > 0 ? 'warning' : 'healthy') : 'warning');
  const mmMetric = mm?.configured ? `${mmPriced}/${mmTotal} priced` : 'Not configured';
  const mmDetail = mmStale > 0 ? `${mmStale} stale` : undefined;

  const psaStatus: TileStatus = !psa ? 'unknown' : (psa.configured ? 'healthy' : 'warning');
  const psaPending = psa?.pendingCount ?? 0;
  const psaMetric = psa?.configured ? `${psa.interval || 'configured'}` : 'Not configured';
  const psaDetail = psaPending > 0 ? `${psaPending} pending` : undefined;

  return (
    <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
      <Tile label="DoubleHolo" status={dhStatus} metric={dhMetric} detail={dhDetail} />
      <Tile label="Card Ladder" status={clStatus} metric={clMetric} detail={clDetail} />
      <Tile label="Market Movers" status={mmStatus} metric={mmMetric} detail={mmDetail} />
      <Tile label="PSA Sync" status={psaStatus} metric={psaMetric} detail={psaDetail} />
    </div>
  );
}
