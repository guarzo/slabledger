import { AIStatusTab } from './AIStatusTab';
import { AIPricingTab } from './AIPricingTab';

export function AITab({ enabled = true }: { enabled?: boolean }) {
  return (
    <div className="space-y-8 mt-4">
      <section>
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">AI Usage</h3>
        <AIStatusTab enabled={enabled} />
      </section>

      <hr className="border-[var(--surface-2)]" />

      <section>
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">Price Overrides</h3>
        <AIPricingTab enabled={enabled} />
      </section>
    </div>
  );
}
