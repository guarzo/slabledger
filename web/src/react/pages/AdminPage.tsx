/**
 * AdminPage - Admin panel with consolidated tabs.
 * Only accessible to admin users.
 */
import { useState } from 'react';
import { Tabs } from 'radix-ui';
import { api } from '../../js/api';
import { Button, TabNavigation } from '../ui';
import type { Tab } from '../ui';
import { UsersTab } from './admin/UsersTab';
import { CardDataTab } from './admin/CardDataTab';
import { PricingTab } from './admin/PricingTab';
import { AITab } from './admin/AITab';
import { IntegrationsTab } from './admin/IntegrationsTab';

const adminTabs: readonly Tab<string>[] = [
  { id: 'users', label: 'Users' },
  { id: 'card-data', label: 'Card Data' },
  { id: 'pricing', label: 'Pricing' },
  { id: 'ai', label: 'AI' },
  { id: 'integrations', label: 'Integrations' },
] as const;

export default function AdminPage() {
  const [activeTab, setActiveTab] = useState('integrations');
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

        <Tabs.Content value="users"><UsersTab enabled={activeTab === 'users'} /></Tabs.Content>
        <Tabs.Content value="card-data"><CardDataTab enabled={activeTab === 'card-data'} /></Tabs.Content>
        <Tabs.Content value="pricing"><PricingTab enabled={activeTab === 'pricing'} /></Tabs.Content>
        <Tabs.Content value="ai"><AITab enabled={activeTab === 'ai'} /></Tabs.Content>
        <Tabs.Content value="integrations"><IntegrationsTab enabled={activeTab === 'integrations'} /></Tabs.Content>
      </Tabs.Root>
    </div>
  );
}
