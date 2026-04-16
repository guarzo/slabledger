/**
 * AdminPage - Admin panel with consolidated tabs.
 * Only accessible to admin users.
 */
import { useState } from 'react';
import { Tabs } from 'radix-ui';
import { api } from '../../js/api';
import { Button, SectionErrorBoundary, TabNavigation, ErrorAlert } from '../ui';
import type { Tab } from '../ui';
import { UsersTab } from './admin/UsersTab';
import { PricingTab } from './admin/PricingTab';
import { StatsTab } from './admin/StatsTab';
import { IntegrationsTab } from './admin/IntegrationsTab';

const adminTabs: readonly Tab<string>[] = [
  { id: 'users', label: 'Users' },
  { id: 'pricing', label: 'Pricing' },
  { id: 'stats', label: 'Stats' },
  { id: 'integrations', label: 'Integrations' },
] as const;

const TAB_SUBTITLES: Record<string, string> = {
  users: 'Manage allowed users and view registered accounts.',
  pricing: 'Review pricing coverage, flags, and price overrides.',
  stats: 'Monitor AI usage, API health, and integration statistics.',
  integrations: 'Connect and configure external services.',
};

export default function AdminPage() {
  const [activeTab, setActiveTab] = useState('users');
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
          <Button variant="secondary" onClick={handleDownloadBackup} loading={backupLoading}>
            {backupLoading ? 'Downloading...' : 'Download Backup'}
          </Button>
        </div>
        {backupError && (
          <ErrorAlert message={`Backup failed: ${backupError}`} className="mt-1" />
        )}
        <p className="text-sm text-[var(--text-muted)] mt-1">
          {TAB_SUBTITLES[activeTab] ?? ''}
        </p>
      </div>

      <Tabs.Root value={activeTab} onValueChange={setActiveTab}>
        <TabNavigation tabs={adminTabs} ariaLabel="Admin tabs" />

        <Tabs.Content value="users">
           <SectionErrorBoundary sectionName="Users">
             <UsersTab enabled={activeTab === 'users'} />
           </SectionErrorBoundary>
         </Tabs.Content>
         <Tabs.Content value="pricing">
           <SectionErrorBoundary sectionName="Pricing">
             <PricingTab enabled={activeTab === 'pricing'} />
           </SectionErrorBoundary>
         </Tabs.Content>
        <Tabs.Content value="stats">
          <SectionErrorBoundary sectionName="Stats">
            <StatsTab enabled={activeTab === 'stats'} />
          </SectionErrorBoundary>
        </Tabs.Content>
        <Tabs.Content value="integrations">
          <SectionErrorBoundary sectionName="Integrations">
            <IntegrationsTab enabled={activeTab === 'integrations'} />
          </SectionErrorBoundary>
        </Tabs.Content>
      </Tabs.Root>
    </div>
  );
}
