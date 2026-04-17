import { CardLadderTab } from './CardLadderTab';
import { DHTab } from './DHTab';
import { MarketMoversTab } from './MarketMoversTab';
import { PSASyncTab } from './PSASyncTab';
import { useCardLadderStatus, useDHStatus, useMarketMoversStatus, usePSASyncStatus } from '../../queries/useAdminQueries';

function StatusBadge({ connected, label }: { connected: boolean; label: string }) {
  return (
    <span className="flex items-center gap-1.5 text-xs">
      <span className={`w-1.5 h-1.5 rounded-full ${connected ? 'bg-[var(--success)]' : 'bg-[var(--text-muted)]'}`} />
      <span className={connected ? 'text-[var(--success)]' : 'text-[var(--text-muted)]'}>{label}</span>
    </span>
  );
}

export function IntegrationsTab({ enabled = true }: { enabled?: boolean }) {
  const { data: dhStatus } = useDHStatus({ enabled });
  const { data: clStatus } = useCardLadderStatus({ enabled });
  const { data: mmStatus } = useMarketMoversStatus({ enabled });
  const { data: psaStatus } = usePSASyncStatus({ enabled });

  const dhHealthy = dhStatus?.api_health ? dhStatus.api_health.success_rate >= 0.95 : false;
  const clConnected = clStatus?.configured ?? false;
  const mmConnected = mmStatus?.configured ?? false;
  const psaConfigured = psaStatus?.configured ?? false;

  return (
    <div className="space-y-8 mt-4">
      <section>
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-base font-semibold text-[var(--text)]">DoubleHolo</h3>
          <StatusBadge connected={dhHealthy} label={dhHealthy ? 'Healthy' : 'Unknown'} />
        </div>
        <DHTab enabled={enabled} />
      </section>

      <hr className="border-[var(--surface-2)]" />

      <section>
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-base font-semibold text-[var(--text)]">Card Ladder</h3>
          <StatusBadge connected={clConnected} label={clConnected ? 'Connected' : 'Not connected'} />
        </div>
        <CardLadderTab enabled={enabled} />
      </section>

      <hr className="border-[var(--surface-2)]" />

      <section>
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-base font-semibold text-[var(--text)]">Market Movers</h3>
          <StatusBadge connected={mmConnected} label={mmConnected ? 'Connected' : 'Not connected'} />
        </div>
        <MarketMoversTab enabled={enabled} />
      </section>

      <hr className="border-[var(--surface-2)]" />

      <section>
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-base font-semibold text-[var(--text)]">PSA Sheets Sync</h3>
          <StatusBadge connected={psaConfigured} label={psaConfigured ? 'Configured' : 'Not configured'} />
        </div>
        <PSASyncTab enabled={enabled} />
      </section>
    </div>
  );
}
