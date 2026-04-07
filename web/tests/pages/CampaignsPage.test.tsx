import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import CampaignsPage from '../../src/react/pages/CampaignsPage';
import { ToastProvider } from '../../src/react/contexts/ToastContext';
import { api } from '../../src/js/api';

// Mock recharts to avoid SVG rendering issues in JSDOM
vi.mock('recharts', () => ({
  BarChart: () => null,
  Bar: () => null,
  LineChart: () => null,
  Line: () => null,
  ReferenceLine: () => null,
  XAxis: () => null,
  YAxis: () => null,
  Tooltip: () => null,
  ResponsiveContainer: ({ children }: { children: React.ReactNode }) => children,
  Cell: () => null,
}));

vi.mock('../../src/js/api', () => ({
  api: {
    listCampaigns: vi.fn(() => Promise.resolve([])),
    getCampaignPNL: vi.fn(() => Promise.resolve({
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
    })),
    getCapitalSummary: vi.fn(() => Promise.resolve({
      outstandingCents: 0,
      recoveryRate30dCents: 0,
      recoveryRate30dPriorCents: 0,
      weeksToCover: 99,
      recoveryTrend: 'stable' as const,
      alertLevel: 'ok' as const,
      refundedCents: 0,
      paidCents: 0,
      unpaidInvoiceCount: 0,
    })),
    getPortfolioHealth: vi.fn(() => Promise.resolve({
      campaigns: [],
      totalDeployedCents: 0,
      totalRecoveredCents: 0,
      totalAtRiskCents: 0,
      overallROI: 0,
    })),
    listInvoices: vi.fn(() => Promise.resolve([])),
    getPortfolioChannelVelocity: vi.fn(() => Promise.resolve([])),
    getCapitalTimeline: vi.fn(() => Promise.resolve({ dataPoints: [], invoiceDates: [] })),
    getWeeklyReview: vi.fn(() => Promise.resolve({
      weekStart: '',
      weekEnd: '',
      purchasesThisWeek: 0,
      purchasesLastWeek: 0,
      spendThisWeekCents: 0,
      spendLastWeekCents: 0,
      salesThisWeek: 0,
      salesLastWeek: 0,
      revenueThisWeekCents: 0,
      revenueLastWeekCents: 0,
      profitThisWeekCents: 0,
      profitLastWeekCents: 0,
      byChannel: [],
      weeksToCover: 99,
      topPerformers: [],
      bottomPerformers: [],
    })),
    createCampaign: vi.fn(),
  },
}));

const mockedApi = vi.mocked(api);

const mockCampaigns = [
  {
    id: 'c1',
    name: 'Test Campaign 1',
    sport: 'Pokemon',
    yearRange: '1999-2003',
    gradeRange: '8-10',
    priceRange: '50-500',
    clConfidence: 'high',
    buyTermsCLPct: 0.75,
    dailySpendCapCents: 10000,
    inclusionList: '',
    exclusionMode: false,
    phase: 'active' as const,
    psaSourcingFeeCents: 0,
    ebayFeePct: 0.13,
    expectedFillRate: 80,
    createdAt: '2025-01-01T00:00:00Z',
    updatedAt: '2025-01-15T00:00:00Z',
  },
  {
    id: 'c2',
    name: 'Test Campaign 2',
    sport: 'Pokemon',
    yearRange: '2020-2024',
    gradeRange: '9-10',
    priceRange: '100-1000',
    clConfidence: 'medium',
    buyTermsCLPct: 0.70,
    dailySpendCapCents: 5000,
    inclusionList: 'Charizard',
    exclusionMode: false,
    phase: 'closed' as const,
    psaSourcingFeeCents: 0,
    ebayFeePct: 0.13,
    expectedFillRate: 70,
    createdAt: '2025-02-01T00:00:00Z',
    updatedAt: '2025-02-10T00:00:00Z',
  },
];

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <ToastProvider>
          <CampaignsPage />
        </ToastProvider>
      </MemoryRouter>
    </QueryClientProvider>
  );
}

describe('CampaignsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('shows loading state initially', () => {
    mockedApi.listCampaigns.mockReturnValue(new Promise(() => {})); // never resolves
    renderPage();
    expect(screen.getByTestId('pokeball-loader')).toBeInTheDocument();
  });

  it('renders campaign list from mock data', async () => {
    mockedApi.listCampaigns.mockResolvedValue(mockCampaigns);
    renderPage();

    await waitFor(() => {
      expect(screen.getByText('Test Campaign 1')).toBeInTheDocument();
    });
    expect(screen.getByText('Test Campaign 2')).toBeInTheDocument();
  });

  it('handles empty campaign list', async () => {
    mockedApi.listCampaigns.mockResolvedValue([]);
    renderPage();

    await waitFor(() => {
      expect(screen.getByText('No campaigns yet')).toBeInTheDocument();
    });
  });

  it('cleans up on unmount without errors', () => {
    const { unmount } = renderPage();
    expect(() => unmount()).not.toThrow();
  });
});
