/**
 * AdminPage - Admin panel with consolidated tabs.
 * Only accessible to admin users.
 */
import { useState } from 'react';
import { Tabs } from 'radix-ui';
import { Breadcrumb, SectionErrorBoundary, TabNavigation } from '../ui';
import type { Tab } from '../ui';
import { UsersTab } from './admin/UsersTab';
import { PricingTab } from './admin/PricingTab';
import { IntegrationsTab } from './admin/IntegrationsTab';

const adminTabs: readonly Tab<string>[] = [
  { id: 'integrations', label: 'Integrations' },
  { id: 'pricing', label: 'Pricing' },
  { id: 'users', label: 'Users' },
] as const;

const TAB_SUBTITLES: Record<string, string> = {
  integrations: 'Monitor integration health, run syncs, and configure external services.',
  pricing: 'Review price flags and overrides.',
  users: 'Manage allowed users and view registered accounts.',
};

export default function AdminPage() {
  const [activeTab, setActiveTab] = useState('integrations');

  const activeTabLabel = adminTabs.find((t) => t.id === activeTab)?.label ?? activeTab;

  return (
    <div className="max-w-6xl mx-auto px-4 sm:px-6">
      <Breadcrumb items={[
        { label: 'Admin', href: '/admin' },
        { label: activeTabLabel },
      ]} />
      <div className="mb-6">
        <h1 className="page-title">Admin</h1>
        <p className="text-sm text-[var(--text-muted)] mt-1 max-w-2xl">
          {TAB_SUBTITLES[activeTab] ?? ''}
        </p>
      </div>

      <Tabs.Root value={activeTab} onValueChange={setActiveTab}>
        <TabNavigation tabs={adminTabs} ariaLabel="Admin tabs" />

        <Tabs.Content value="integrations">
          <SectionErrorBoundary sectionName="Integrations">
            <IntegrationsTab enabled={activeTab === 'integrations'} />
          </SectionErrorBoundary>
        </Tabs.Content>
        <Tabs.Content value="pricing">
          <SectionErrorBoundary sectionName="Pricing">
            <PricingTab enabled={activeTab === 'pricing'} />
          </SectionErrorBoundary>
        </Tabs.Content>
        <Tabs.Content value="users">
          <SectionErrorBoundary sectionName="Users">
            <UsersTab enabled={activeTab === 'users'} />
          </SectionErrorBoundary>
        </Tabs.Content>
      </Tabs.Root>
    </div>
  );
}
