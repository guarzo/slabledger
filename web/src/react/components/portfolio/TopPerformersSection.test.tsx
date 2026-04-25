import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import TopPerformersSection from './TopPerformersSection';
import type { CampaignHealth, CampaignPNL } from '../../../types/campaigns';

// PNL data per campaign id; tests configure this before rendering.
const pnlByCampaign = new Map<string, CampaignPNL>();

vi.mock('../../../js/api', () => ({
  api: {
    getCampaignPNL: vi.fn().mockImplementation(async (id: string) => {
      const pnl = pnlByCampaign.get(id);
      if (!pnl) throw new Error(`no pnl for ${id}`);
      return pnl;
    }),
  },
}));

function pnl(overrides: Partial<CampaignPNL>): CampaignPNL {
  return {
    campaignId: '',
    totalSpendCents: 0,
    totalRevenueCents: 0,
    totalFeesCents: 0,
    netProfitCents: 0,
    roi: 0,
    avgDaysToSell: 0,
    totalPurchases: 0,
    totalSold: 0,
    totalUnsold: 0,
    sellThroughPct: 0,
    ...overrides,
  };
}

function health(id: string, name: string): CampaignHealth {
  return {
    campaignId: id,
    campaignName: name,
    phase: 'active',
    roi: 0,
    sellThroughPct: 0,
    avgDaysToSell: 0,
    totalPurchases: 0,
    totalUnsold: 0,
    capitalAtRiskCents: 0,
    healthStatus: 'healthy',
    healthReason: '',
  };
}

function renderSection(campaigns: CampaignHealth[]) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <TopPerformersSection campaigns={campaigns} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('TopPerformersSection', () => {
  beforeEach(() => {
    pnlByCampaign.clear();
  });

  it('renders rows sorted by net profit descending', async () => {
    pnlByCampaign.set('a', pnl({ campaignId: 'a', netProfitCents: 100_00, totalRevenueCents: 500_00, roi: 0.10 }));
    pnlByCampaign.set('b', pnl({ campaignId: 'b', netProfitCents: 800_00, totalRevenueCents: 2_000_00, roi: 0.40 }));
    pnlByCampaign.set('c', pnl({ campaignId: 'c', netProfitCents: 300_00, totalRevenueCents: 900_00, roi: 0.20 }));

    renderSection([health('a', 'Alpha'), health('b', 'Beta'), health('c', 'Gamma')]);

    await waitFor(() => expect(screen.getByText('Top Performers')).toBeInTheDocument());

    const items = await waitFor(() => {
      const list = screen.getAllByRole('listitem');
      expect(list).toHaveLength(3);
      return list;
    });

    expect(items[0]).toHaveTextContent('Beta');
    expect(items[1]).toHaveTextContent('Gamma');
    expect(items[2]).toHaveTextContent('Alpha');
  });

  it('limits to top 5 even when more campaigns are present', async () => {
    const names = ['One', 'Two', 'Three', 'Four', 'Five', 'Six', 'Seven'];
    names.forEach((n, i) => {
      pnlByCampaign.set(n, pnl({ campaignId: n, netProfitCents: (i + 1) * 100_00, totalRevenueCents: 1000_00, roi: 0.10 }));
    });

    renderSection(names.map(n => health(n, n)));

    await waitFor(() => {
      expect(screen.getAllByRole('listitem')).toHaveLength(5);
    });
  });

  it('skips campaigns with no realized activity', async () => {
    pnlByCampaign.set('idle', pnl({ campaignId: 'idle', netProfitCents: 0, totalRevenueCents: 0, roi: 0 }));
    pnlByCampaign.set('active', pnl({ campaignId: 'active', netProfitCents: 50_00, totalRevenueCents: 200_00, roi: 0.25 }));

    renderSection([health('idle', 'Idle'), health('active', 'Active')]);

    await waitFor(() => {
      const items = screen.getAllByRole('listitem');
      expect(items).toHaveLength(1);
      expect(items[0]).toHaveTextContent('Active');
    });
  });

  it('returns null when no campaigns have realized activity', async () => {
    pnlByCampaign.set('idle1', pnl({ campaignId: 'idle1' }));
    pnlByCampaign.set('idle2', pnl({ campaignId: 'idle2' }));

    const { container } = renderSection([health('idle1', 'Idle 1'), health('idle2', 'Idle 2')]);

    // Wait for the queries to settle, then assert nothing rendered.
    await waitFor(() => expect(screen.queryByText('Top Performers')).not.toBeInTheDocument());
    expect(container).toBeEmptyDOMElement();
  });

  it('renders rank prefixes and a View all link', async () => {
    pnlByCampaign.set('a', pnl({ campaignId: 'a', netProfitCents: 200_00, totalRevenueCents: 800_00, roi: 0.30 }));

    renderSection([health('a', 'Alpha')]);

    await waitFor(() => expect(screen.getByText('#1')).toBeInTheDocument());
    const viewAll = screen.getByRole('link', { name: /view all/i });
    expect(viewAll).toHaveAttribute('href', '/campaigns');
  });
});
