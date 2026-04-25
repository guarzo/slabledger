import { CardLadderTab } from './CardLadderTab';
import { DHTab } from './DHTab';
import { MarketMoversTab } from './MarketMoversTab';
import { PSASyncTab } from './PSASyncTab';
import { useCardLadderStatus, useDHStatus, useMarketMoversStatus, usePSASyncStatus } from '../../queries/useAdminQueries';
import SalesImportSection from '../tools/SalesImportSection';
import { StatusPill } from '../../ui';

const SECTION_HEADER = 'text-sm font-semibold uppercase tracking-wider text-[var(--text-muted)] mb-3';

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
        <div className="flex items-center justify-between mb-3">
          <h3 className={SECTION_HEADER}>DoubleHolo</h3>
          {dhHealthy ? (
            <StatusPill tone="success">Healthy</StatusPill>
          ) : (
            <StatusPill tone="neutral">Unknown</StatusPill>
          )}
        </div>
        <DHTab enabled={enabled} />
      </section>

      <hr className="border-[var(--surface-2)]" />

      <section>
        <div className="flex items-center justify-between mb-3">
          <h3 className={SECTION_HEADER}>Card Ladder</h3>
          {clConnected ? (
            <StatusPill tone="success">Connected</StatusPill>
          ) : (
            <StatusPill tone="danger">Not connected</StatusPill>
          )}
        </div>
        <CardLadderTab enabled={enabled} />
      </section>

      <hr className="border-[var(--surface-2)]" />

      <section>
        <div className="flex items-center justify-between mb-3">
          <h3 className={SECTION_HEADER}>Market Movers</h3>
          {mmConnected ? (
            <StatusPill tone="success">Connected</StatusPill>
          ) : (
            <StatusPill tone="danger">Not connected</StatusPill>
          )}
        </div>
        <MarketMoversTab enabled={enabled} />
      </section>

      <hr className="border-[var(--surface-2)]" />

      <section>
        <div className="flex items-center justify-between mb-3">
          <h3 className={SECTION_HEADER}>PSA Sheets Sync</h3>
          {psaConfigured ? (
            <StatusPill tone="success">Configured</StatusPill>
          ) : (
            <StatusPill tone="danger">Not configured</StatusPill>
          )}
        </div>
        <PSASyncTab enabled={enabled} />
      </section>

      <hr className="border-[var(--surface-2)]" />

      <section>
        <div className="mb-3">
          <h3 className={SECTION_HEADER}>Import Sales</h3>
          <p className="text-xs text-[var(--text-muted)] mt-0.5">Import sales from order CSVs.</p>
        </div>
        <SalesImportSection />
      </section>
    </div>
  );
}
