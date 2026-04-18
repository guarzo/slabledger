import { AIStatusTab } from './AIStatusTab';
import { DHStatsPanel } from './DHStatsPanel';
import { MMStatsPanel, CLStatsPanel, PSAStatsPanel } from './ProviderStatsPanel';
import { IntegrationHealthStrip } from './IntegrationHealthStrip';

export function StatsTab({ enabled = true }: { enabled?: boolean }) {
  return (
    <div className="space-y-8 mt-4">
      <IntegrationHealthStrip enabled={enabled} />
      <section>
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">AI Usage</h3>
        <AIStatusTab enabled={enabled} />
      </section>
      <hr className="border-[var(--surface-2)]" />
      <section>
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">DoubleHolo</h3>
        <DHStatsPanel enabled={enabled} />
      </section>
      <hr className="border-[var(--surface-2)]" />
      <section>
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">Market Movers</h3>
        <MMStatsPanel enabled={enabled} />
      </section>
      <hr className="border-[var(--surface-2)]" />
      <section>
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">Card Ladder</h3>
        <CLStatsPanel enabled={enabled} />
      </section>
      <hr className="border-[var(--surface-2)]" />
      <section>
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">PSA Sync</h3>
        <PSAStatsPanel enabled={enabled} />
      </section>
    </div>
  );
}
