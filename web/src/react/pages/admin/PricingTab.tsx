import { PricingCoverageTab } from './PricingCoverageTab';
import { PriceFlagsTab } from './PriceFlagsTab';
import { AIPricingTab } from './AIPricingTab';
import { SECTION_HEADER } from './shared';

export function PricingTab({ enabled = true }: { enabled?: boolean }) {
  return (
    <div className="mt-4">
      <section className="mb-10">
        <h3 className="text-base font-semibold text-[var(--text)]">Coverage</h3>
        <p className="text-xs text-[var(--text-muted)] mt-0.5 mb-5">
          How much of unsold inventory is matched, priced, and live.
        </p>
        <PricingCoverageTab enabled={enabled} />
      </section>

      <section className="mb-8">
        <h3 className={SECTION_HEADER}>Price Flags</h3>
        <PriceFlagsTab enabled={enabled} />
      </section>

      <section>
        <h3 className={SECTION_HEADER}>Price Overrides</h3>
        <AIPricingTab enabled={enabled} />
      </section>
    </div>
  );
}
