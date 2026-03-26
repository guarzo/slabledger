/**
 * AdminPage - Admin panel with allowlist management, user list, and API status.
 * Only accessible to admin users.
 */
import { useState } from 'react';
import { Tabs } from 'radix-ui';
import { api } from '../../js/api';
import { Button, TabNavigation } from '../ui';
import type { Tab } from '../ui';
import { AllowlistTab } from './admin/AllowlistTab';
import { UsersTab } from './admin/UsersTab';
import { ApiStatusTab } from './admin/ApiStatusTab';
import { CardDataTab } from './admin/CardDataTab';
import { MissingCardsTab } from './admin/MissingCardsTab';
import { PricingCoverageTab } from './admin/PricingCoverageTab';
import { AIPricingTab } from './admin/AIPricingTab';
import { AIStatusTab } from './admin/AIStatusTab';
import { InstagramTab } from './admin/InstagramTab';
import { PriceFlagsTab } from './admin/PriceFlagsTab';

const adminTabs: readonly Tab<string>[] = [
  { id: 'allowlist', label: 'Allowlist' },
  { id: 'users', label: 'Users' },
  { id: 'api-status', label: 'API Status' },
  { id: 'card-data', label: 'Card Data' },
  { id: 'missing-cards', label: 'Missing Cards' },
  { id: 'pricing-coverage', label: 'Pricing Coverage' },
  { id: 'ai-pricing', label: 'AI Pricing' },
  { id: 'ai-status', label: 'AI Status' },
  { id: 'instagram', label: 'Instagram' },
  { id: 'price-flags', label: 'Price Flags' },
] as const;

export default function AdminPage() {
  const [activeTab, setActiveTab] = useState('api-status');
  const [backupLoading, setBackupLoading] = useState<boolean>(false);
  const [backupError, setBackupError] = useState<string | null>(null);

  async function handleDownloadBackup() {
    setBackupLoading(true);
    setBackupError(null);
    try {
      const blob = await api.getBackup();
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `slabledger-backup-${new Date().toISOString().slice(0, 10)}.db`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (err) {
      setBackupError(err instanceof Error ? err.message : String(err));
    } finally {
      setBackupLoading(false);
    }
  }

  return (
    <div className="max-w-4xl mx-auto px-4 sm:px-6">
      <div className="mb-6">
        <div className="flex items-center justify-between">
          <h1 className="text-[22px] font-bold text-[var(--text)] tracking-tight">Admin</h1>
          <Button onClick={handleDownloadBackup} loading={backupLoading}>
            {backupLoading ? 'Downloading...' : 'Download Backup'}
          </Button>
        </div>
        {backupError && (
          <p className="text-sm text-[var(--danger)] mt-1">Backup failed: {backupError}</p>
        )}
        <p className="text-sm text-[var(--text-muted)] mt-1">
          Manage allowed users, view registered accounts, and monitor API usage.
        </p>
      </div>

      <Tabs.Root value={activeTab} onValueChange={setActiveTab}>
        <TabNavigation tabs={adminTabs} ariaLabel="Admin tabs" />

        <Tabs.Content value="allowlist"><AllowlistTab /></Tabs.Content>
        <Tabs.Content value="users"><UsersTab /></Tabs.Content>
        <Tabs.Content value="api-status"><ApiStatusTab /></Tabs.Content>
        <Tabs.Content value="card-data"><CardDataTab /></Tabs.Content>
        <Tabs.Content value="missing-cards"><MissingCardsTab /></Tabs.Content>
        <Tabs.Content value="pricing-coverage"><PricingCoverageTab enabled={activeTab === 'pricing-coverage'} /></Tabs.Content>
        <Tabs.Content value="ai-pricing"><AIPricingTab enabled={activeTab === 'ai-pricing'} /></Tabs.Content>
        <Tabs.Content value="ai-status"><AIStatusTab enabled={activeTab === 'ai-status'} /></Tabs.Content>
        <Tabs.Content value="instagram"><InstagramTab /></Tabs.Content>
        <Tabs.Content value="price-flags"><PriceFlagsTab enabled={activeTab === 'price-flags'} /></Tabs.Content>
      </Tabs.Root>
    </div>
  );
}
