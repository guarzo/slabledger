import { ApiStatusTab } from './ApiStatusTab';
import { CardLadderTab } from './CardLadderTab';
import { MarketMoversTab } from './MarketMoversTab';
import { DHTab } from './DHTab';
import { DHPushConfigCard } from './DHPushConfigCard';
import { InstagramTab } from './InstagramTab';

export function IntegrationsTab({ enabled = true }: { enabled?: boolean }) {
  return (
    <div className="space-y-8 mt-4">
      <section>
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">API Providers</h3>
        <ApiStatusTab enabled={enabled} />
      </section>

      <hr className="border-[var(--surface-2)]" />

      <section>
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">DH</h3>
        <DHTab enabled={enabled} />
        <div className="mt-4">
          <DHPushConfigCard />
        </div>
      </section>

      <hr className="border-[var(--surface-2)]" />

      <section>
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">Card Ladder</h3>
        <CardLadderTab enabled={enabled} />
      </section>

      <hr className="border-[var(--surface-2)]" />

      <section>
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">Market Movers</h3>
        <MarketMoversTab enabled={enabled} />
      </section>

      <hr className="border-[var(--surface-2)]" />

      <section>
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">Instagram</h3>
        <InstagramTab enabled={enabled} />
      </section>
    </div>
  );
}
