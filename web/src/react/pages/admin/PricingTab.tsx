import { PricingCoverageTab } from './PricingCoverageTab';
import { PriceFlagsTab } from './PriceFlagsTab';

export function PricingTab({ enabled = true }: { enabled?: boolean }) {
  return (
    <div className="space-y-8 mt-4">
      <section>
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">Coverage</h3>
        <PricingCoverageTab enabled={enabled} />
      </section>

      <hr className="border-[var(--surface-2)]" />

      <section>
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">Price Flags</h3>
        <PriceFlagsTab enabled={enabled} />
      </section>
    </div>
  );
}
