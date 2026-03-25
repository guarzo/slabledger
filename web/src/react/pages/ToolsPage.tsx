import { useState } from 'react';
import { Tabs } from 'radix-ui';
import { useCampaigns } from '../queries/useCampaignQueries';
import OperationsTab, { type OperationState } from './campaigns/OperationsTab';
import ShopifySyncPage from './ShopifySyncPage';
import type { GlobalImportResult, PSAImportResult, ExternalImportResult } from '../../types/campaigns';
import TabNavigation from '../ui/TabNavigation';
import { SectionErrorBoundary } from '../ui';

const TABS = [
  { id: 'operations', label: 'Operations' },
  { id: 'price-sync', label: 'Price Sync' },
] as const;

export default function ToolsPage() {
  const { data: allCampaigns = [] } = useCampaigns(false);
  const [operationState, setOperationState] = useState<OperationState>('idle');
  const [importResult, setImportResult] = useState<GlobalImportResult | null>(null);
  const [psaResult, setPsaResult] = useState<PSAImportResult | null>(null);
  const [externalResult, setExternalResult] = useState<ExternalImportResult | null>(null);

  return (
    <div className="max-w-6xl mx-auto px-4">
      <div className="mb-6">
        <h1 className="text-[22px] font-bold text-[var(--text)] tracking-tight">Tools</h1>
        <p className="mt-1 text-sm text-[var(--text-muted)]">Import, export, and sync operations</p>
      </div>

      <Tabs.Root defaultValue="operations">
        <TabNavigation tabs={TABS} ariaLabel="Tools tabs" />

        <Tabs.Content value="operations">
          <SectionErrorBoundary sectionName="Operations">
            <OperationsTab
              campaigns={allCampaigns}
              operationState={operationState}
              setOperationState={setOperationState}
              importResult={importResult}
              setImportResult={setImportResult}
              psaResult={psaResult}
              setPsaResult={setPsaResult}
              externalResult={externalResult}
              setExternalResult={setExternalResult}
            />
          </SectionErrorBoundary>
        </Tabs.Content>

        <Tabs.Content value="price-sync">
          <SectionErrorBoundary sectionName="Price Sync">
            <ShopifySyncPage embedded />
          </SectionErrorBoundary>
        </Tabs.Content>
      </Tabs.Root>
    </div>
  );
}
