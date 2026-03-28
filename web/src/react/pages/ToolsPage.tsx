import { useState } from 'react';
import { Tabs } from 'radix-ui';
import { useCampaigns } from '../queries/useCampaignQueries';
import OperationsTab, { type OperationState } from './campaigns/OperationsTab';
import ShopifySyncPage from './ShopifySyncPage';
import CertEntryTab from './tools/CertEntryTab';
import EbayExportTab from './tools/EbayExportTab';
import ImportSalesTab from './tools/ImportSalesTab';
import InvoicesTab from './tools/InvoicesTab';
import type { GlobalImportResult, PSAImportResult, ExternalImportResult } from '../../types/campaigns';
import TabNavigation from '../ui/TabNavigation';
import { SectionErrorBoundary } from '../ui';

const TABS = [
  { id: 'import-export', label: 'Import / Export' },
  { id: 'cert-entry', label: 'Cert Entry' },
  { id: 'ebay-export', label: 'eBay Export' },
  { id: 'import-sales', label: 'Import Sales' },
  { id: 'invoices', label: 'Invoices' },
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

      <Tabs.Root defaultValue="import-export">
        <TabNavigation tabs={TABS} ariaLabel="Tools tabs" />

        <Tabs.Content value="import-export">
          <SectionErrorBoundary sectionName="Import / Export">
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
            <div className="mt-6">
              <ShopifySyncPage embedded />
            </div>
          </SectionErrorBoundary>
        </Tabs.Content>

        <Tabs.Content value="cert-entry">
          <SectionErrorBoundary sectionName="Cert Entry">
            <CertEntryTab />
          </SectionErrorBoundary>
        </Tabs.Content>

        <Tabs.Content value="ebay-export">
          <SectionErrorBoundary sectionName="eBay Export">
            <EbayExportTab />
          </SectionErrorBoundary>
        </Tabs.Content>

        <Tabs.Content value="import-sales">
          <SectionErrorBoundary sectionName="Import Sales">
            <ImportSalesTab />
          </SectionErrorBoundary>
        </Tabs.Content>

        <Tabs.Content value="invoices">
          <SectionErrorBoundary sectionName="Invoices">
            <InvoicesTab />
          </SectionErrorBoundary>
        </Tabs.Content>
      </Tabs.Root>
    </div>
  );
}
