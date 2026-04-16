import { useState } from 'react';
import { Tabs } from 'radix-ui';
import { useSearchParams } from 'react-router-dom';
import { useCampaigns } from '../queries/useCampaignQueries';
import OperationsTab, { type OperationState } from './campaigns/OperationsTab';
import CardIntakeTab from './tools/CardIntakeTab';
import LegacyTab from './tools/LegacyTab';
import type { PSAImportResult } from '../../types/campaigns';
import TabNavigation from '../ui/TabNavigation';
import { SectionErrorBoundary } from '../ui';

const TABS = [
  { id: 'daily-ops', label: 'Daily Ops' },
  { id: 'card-intake', label: 'Card Intake' },
  { id: 'legacy', label: 'Legacy' },
] as const;

export default function ToolsPage() {
  const { data: allCampaigns = [] } = useCampaigns(false);
  const [searchParams] = useSearchParams();
  const initialTab = TABS.some(t => t.id === searchParams.get('tab')) ? searchParams.get('tab')! : 'daily-ops';
  const [operationState, setOperationState] = useState<OperationState>('idle');
  const [psaResult, setPsaResult] = useState<PSAImportResult | null>(null);

  return (
    <div className="max-w-6xl mx-auto px-4">
      <div className="mb-6">
        <h1 className="text-[22px] font-bold text-[var(--text)] tracking-tight">Tools</h1>
        <p className="mt-1 text-sm text-[var(--text-muted)]">Daily operations, card intake, and legacy tools</p>
      </div>

      <Tabs.Root defaultValue={initialTab}>
        <TabNavigation tabs={TABS} ariaLabel="Tools tabs" />

        <Tabs.Content value="daily-ops">
          <SectionErrorBoundary sectionName="Daily Ops">
            <OperationsTab
              campaigns={allCampaigns}
              operationState={operationState}
              setOperationState={setOperationState}
              psaResult={psaResult}
              setPsaResult={setPsaResult}
            />
          </SectionErrorBoundary>
        </Tabs.Content>

        <Tabs.Content value="card-intake">
          <SectionErrorBoundary sectionName="Card Intake">
            <CardIntakeTab />
          </SectionErrorBoundary>
        </Tabs.Content>

        <Tabs.Content value="legacy">
          <SectionErrorBoundary sectionName="Legacy">
            <LegacyTab />
          </SectionErrorBoundary>
        </Tabs.Content>
      </Tabs.Root>
    </div>
  );
}
