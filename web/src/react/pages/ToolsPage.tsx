import CardIntakeTab from './tools/CardIntakeTab';
import DHUnmatchedSection from './tools/DHUnmatchedSection';
import { PendingItemsCard } from './campaigns/PendingItemsCard';
import { SectionErrorBoundary } from '../ui';

export default function ToolsPage() {
  return (
    <div className="max-w-6xl mx-auto px-4">
      <div className="mb-6">
        <h1 className="text-[22px] font-bold text-[var(--text)] tracking-tight">Tools</h1>
        <p className="mt-1 text-sm text-[var(--text-muted)]">Scan, match, and assign</p>
      </div>

      <div className="space-y-6">
        <SectionErrorBoundary sectionName="Card Intake">
          <CardIntakeTab />
        </SectionErrorBoundary>

        <SectionErrorBoundary sectionName="Pending Items">
          <PendingItemsCard />
        </SectionErrorBoundary>

        <SectionErrorBoundary sectionName="Unmatched Cards">
          <DHUnmatchedSection />
        </SectionErrorBoundary>
      </div>
    </div>
  );
}
