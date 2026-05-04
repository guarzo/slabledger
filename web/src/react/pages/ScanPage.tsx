import CardIntakeTab from './tools/CardIntakeTab';
import DHUnmatchedSection from './tools/DHUnmatchedSection';
import { PendingItemsCard } from './campaigns/PendingItemsCard';
import { SectionErrorBoundary } from '../ui';

const WORKFLOW_STEPS = [
  { num: 1, label: 'Scan cert', detail: 'Type or scan a PSA cert number' },
  { num: 2, label: 'Match', detail: 'We resolve the card from PSA + DoubleHolo' },
  { num: 3, label: 'Assign', detail: 'Pick a campaign — buy cost is captured' },
  { num: 4, label: 'Ready to list', detail: 'Cert moves into your inventory' },
];

export default function ScanPage() {
  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <div>
        <h1 className="page-title">Scan</h1>
        <p className="text-sm text-[var(--text-muted)]">Scan, match, and assign</p>
      </div>

      <ol className="grid grid-cols-2 md:grid-cols-4 gap-3 border-y border-[var(--surface-2)] py-4">
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
