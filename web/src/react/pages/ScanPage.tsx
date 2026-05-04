import { useEffect, useState } from 'react';
import CardIntakeTab from './tools/CardIntakeTab';
import DHUnmatchedSection from './tools/DHUnmatchedSection';
import { PendingItemsCard } from './campaigns/PendingItemsCard';
import { SectionErrorBoundary } from '../ui';
import { useGlobalInventory } from '../queries/useCampaignQueries';

const WORKFLOW_STEPS = [
  { num: 1, label: 'Scan cert', detail: 'Type or scan a PSA cert number' },
  { num: 2, label: 'Match', detail: 'We resolve the card from PSA + DoubleHolo' },
  { num: 3, label: 'Assign', detail: 'Pick a campaign — buy cost is captured' },
  { num: 4, label: 'Ready to list', detail: 'Cert moves into your inventory' },
];

export default function ScanPage() {
  // Detect first-run vs returning operator. The 4-step workflow strip is
  // teaching content — load-bearing on day one, noise after the first cert
  // has been scanned. Use the global inventory count as the signal: if
  // there's any inventory, the user has been through the flow already and
  // the strip collapses to a one-line summary they can expand if they want
  // a refresher.
  const { data: inventory, isLoading: inventoryLoading } = useGlobalInventory();

  // Local state on the <details> element so user toggles persist and the
  // strip doesn't snap closed during a refetch where `inventory` briefly
  // becomes undefined. Initialised from the onboarding signal once loading
  // completes; after that the user owns it.
  const [isStripOpen, setIsStripOpen] = useState<boolean>(true);
  const [hasInitialised, setHasInitialised] = useState(false);
  useEffect(() => {
    if (inventoryLoading || hasInitialised) return;
    const hasOnboarded = (inventory?.length ?? 0) > 0;
    setIsStripOpen(!hasOnboarded);
    setHasInitialised(true);
  }, [inventoryLoading, hasInitialised, inventory]);

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <div>
        <h1 className="page-title">Scan</h1>
        <p className="text-sm text-[var(--text-muted)]">Scan, match, and assign</p>
      </div>

      {/* Open by default for first-run; collapsed for returning operators
          with the chevron-toggle pattern from the Insights page. The strip
          itself is unchanged — only its container framing differs. */}
      <details
        className="group"
        open={isStripOpen}
        onToggle={(e) => setIsStripOpen((e.target as HTMLDetailsElement).open)}
      >
        <summary className="cursor-pointer list-none flex items-center justify-between gap-2 py-2 text-sm text-[var(--text-muted)] hover:text-[var(--text)] transition-colors">
          <span className="text-[10px] font-semibold uppercase tracking-[0.14em] text-[var(--brand-400)]">
            How scan works · 4 steps
          </span>
          <span
            aria-hidden="true"
            className="text-xs transition-transform group-open:rotate-90"
          >
            ›
          </span>
        </summary>
        <ol className="mt-2 grid grid-cols-2 md:grid-cols-4 gap-3 border-y border-[var(--surface-2)] py-4">
          {WORKFLOW_STEPS.map((step) => (
            <li key={step.num} className="flex items-start gap-3 min-w-0">
              <span
                className="flex-shrink-0 inline-flex items-center justify-center w-6 h-6 rounded-full border border-[var(--brand-500)]/40 bg-[var(--brand-500)]/10 text-xs font-semibold text-[var(--brand-400)] tabular-nums"
                aria-hidden
              >
                {step.num}
              </span>
              <div className="min-w-0">
                <div className="text-sm font-medium text-[var(--text)]">{step.label}</div>
                <div className="text-xs text-[var(--text-muted)] leading-snug">{step.detail}</div>
              </div>
            </li>
          ))}
        </ol>
      </details>

      <SectionErrorBoundary sectionName="Card Intake">
        <CardIntakeTab />
      </SectionErrorBoundary>

      <SectionErrorBoundary sectionName="Pending Items">
        <PendingItemsCard />
      </SectionErrorBoundary>

      <SectionErrorBoundary sectionName="DH Unmatched">
        <DHUnmatchedSection />
      </SectionErrorBoundary>
    </div>
  );
}
