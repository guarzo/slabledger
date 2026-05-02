import CardIntakeTab from './tools/CardIntakeTab';
import DHUnmatchedSection from './tools/DHUnmatchedSection';
import { PendingItemsCard } from './campaigns/PendingItemsCard';
import { SectionErrorBoundary } from '../ui';

export default function ScanPage() {
  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <div>
        <h1 className="text-[22px] font-bold text-[var(--text)] tracking-tight">Scan</h1>
        <p className="text-sm text-[var(--text-muted)]">Scan, match, and assign</p>
      </div>

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
