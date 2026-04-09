import { CardLadderTab } from './CardLadderTab';
import { DHTab } from './DHTab';
import { DHPushConfigCard } from './DHPushConfigCard';
import { InstagramTab } from './InstagramTab';
import { MarketMoversTab } from './MarketMoversTab';
import { useCardLadderStatus, useDHStatus, useMarketMoversStatus } from '../../queries/useAdminQueries';
import { useInstagramStatus } from '../../queries/useSocialQueries';

function StatusBadge({ connected, label }: { connected: boolean; label: string }) {
  return (
    <span className="flex items-center gap-1.5 text-xs">
      <span className={`w-1.5 h-1.5 rounded-full ${connected ? 'bg-emerald-400' : 'bg-gray-500'}`} />
      <span className={connected ? 'text-emerald-400' : 'text-[var(--text-muted)]'}>{label}</span>
    </span>
  );
}

export function IntegrationsTab({ enabled = true }: { enabled?: boolean }) {
  const { data: dhStatus } = useDHStatus({ enabled });
  const { data: clStatus } = useCardLadderStatus({ enabled });
  const { data: igStatus } = useInstagramStatus(enabled);
  const { data: mmStatus } = useMarketMoversStatus({ enabled });

  const dhHealthy = dhStatus?.api_health ? dhStatus.api_health.success_rate >= 0.95 : false;
  const clConnected = clStatus?.configured ?? false;
  const igConnected = igStatus?.connected ?? false;
  const mmConnected = mmStatus?.configured ?? false;

  return (
    <div className="space-y-8 mt-4">
      <section>
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-base font-semibold text-[var(--text)]">DoubleHolo</h3>
          <StatusBadge connected={dhHealthy} label={dhHealthy ? 'Healthy' : 'Unknown'} />
        </div>
        <DHTab enabled={enabled} />
        <div className="mt-4">
          <DHPushConfigCard />
        </div>
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
          <h3 className="text-base font-semibold text-[var(--text)]">Instagram</h3>
          <StatusBadge connected={igConnected} label={igConnected ? 'Connected' : 'Not connected'} />
        </div>
        <InstagramTab enabled={enabled} />
      </section>

      <hr className="border-[var(--surface-2)]" />

      <section>
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-base font-semibold text-[var(--text)]">Market Movers</h3>
          <StatusBadge connected={mmConnected} label={mmConnected ? 'Connected' : 'Not connected'} />
        </div>
        <MarketMoversTab enabled={enabled} />
      </section>
    </div>
  );
}
