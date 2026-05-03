/**
 * Campaign Detail Page
 *
 * Shows campaign overview, transactions, tuning, and settings in tabs.
 */
import { useState } from 'react';
import { useParams, Link, useNavigate } from 'react-router-dom';
import { Tabs } from 'radix-ui';
import PokeballLoader from '../PokeballLoader';
import { Breadcrumb, SectionErrorBoundary, TabNavigation } from '../ui';
import OverviewTab from './campaign-detail/OverviewTab';
import TransactionsTab from './campaign-detail/TransactionsTab';
import SettingsTab from './campaign-detail/SettingsTab';
import TuningTab from './campaign-detail/TuningTab';
import type { Campaign } from '../../types/campaigns';
import { campaignTabs, type CampaignTabId } from '../utils/campaignConstants';
import { useCampaign, useUpdateCampaign, usePurchases, useSales } from '../queries/useCampaignQueries';
import { useCampaignDerived } from '../hooks/useCampaignDerived';

export default function CampaignDetailPage() {
  const { id } = useParams<{ id: string }>();
  const campaignId = id ?? '';
  const navigate = useNavigate();
  const [tab, setTab] = useState<CampaignTabId>('overview');

  const { data: campaign, isLoading: campaignLoading, refetch: refetchCampaign } = useCampaign(campaignId);
  const updateCampaign = useUpdateCampaign(campaignId);
  const { data: purchases = [], isLoading: purchasesLoading } = usePurchases(campaignId);
  const { data: sales = [], isLoading: salesLoading } = useSales(campaignId);

  const { soldPurchaseIds, unsoldPurchases, totalSpent, totalRevenue, totalProfit, sellThrough } =
    useCampaignDerived(purchases, sales);

  const loading = campaignLoading || purchasesLoading || salesLoading;

  if (!campaignId) {
    return (
      <div className="max-w-6xl mx-auto px-4 text-center py-16 text-[var(--text-muted)]">Campaign not found.</div>
    );
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[50vh]">
        <PokeballLoader />
      </div>
    );
  }

  if (!campaign) {
    return (
      <div className="max-w-6xl mx-auto px-4 text-center py-16 text-[var(--text-muted)]">
        Campaign not found. <Link to="/campaigns" className="text-[var(--brand-500)] underline">Back to campaigns</Link>
      </div>
    );
  }

  const tabs = campaignTabs;

  return (
    <div className="max-w-[1600px] mx-auto px-4">
      <Breadcrumb items={[{ label: 'Campaigns', href: '/campaigns' }]} />

      <div className="mb-6">
        <h1 className="page-title">{campaign.name}</h1>
        <p className="text-xs text-[var(--text-muted)] font-mono mt-1 tabular-nums">
          {campaign.sport} · {campaign.yearRange} · PSA {campaign.gradeRange}
        </p>
      </div>

      <Tabs.Root value={tab} onValueChange={(v) => setTab(v as CampaignTabId)}>
        <TabNavigation<CampaignTabId>
          tabs={tabs}
          counts={{ transactions: purchases.length + sales.length }}
          ariaLabel="Campaign sections"
        />

        <Tabs.Content value="overview" className="outline-none">
          <SectionErrorBoundary sectionName="Overview">
            <OverviewTab
              campaignId={campaignId}
              totalSpent={totalSpent}
              totalRevenue={totalRevenue}
              totalProfit={totalProfit}
              sellThrough={sellThrough}
              purchaseCount={purchases.length}
              saleCount={sales.length}
              unsoldCount={unsoldPurchases.length}
              dailySpendCapCents={campaign.dailySpendCapCents}
              expectedFillRate={campaign.expectedFillRate}
            />
          </SectionErrorBoundary>
        </Tabs.Content>

        <Tabs.Content value="transactions" className="outline-none">
          <SectionErrorBoundary sectionName="Transactions">
            <TransactionsTab
              campaignId={campaignId}
              purchases={purchases}
              sales={sales}
              soldPurchaseIds={soldPurchaseIds}
            />
          </SectionErrorBoundary>
        </Tabs.Content>

        <Tabs.Content value="tuning" className="outline-none">
          <SectionErrorBoundary sectionName="Tuning">
            <TuningTab campaignId={campaignId} campaign={campaign} onUpdate={async (updated: Campaign) => {
              await updateCampaign.mutateAsync(updated);
            }} />
          </SectionErrorBoundary>
        </Tabs.Content>

        <Tabs.Content value="settings" className="outline-none">
          <SectionErrorBoundary sectionName="Settings">
            <SettingsTab
              campaign={campaign}
              onUpdate={() => refetchCampaign()}
              onDelete={() => navigate('/campaigns')}
            />
          </SectionErrorBoundary>
        </Tabs.Content>
      </Tabs.Root>
    </div>
  );
}
