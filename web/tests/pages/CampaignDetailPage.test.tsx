import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import CampaignDetailPage from '../../src/react/pages/CampaignDetailPage';
import { api } from '../../src/js/api';

vi.mock('../../src/js/api', () => ({
  api: {
    getCampaign: vi.fn(() => Promise.reject(new Error('Not found'))),
    listPurchases: vi.fn(() => Promise.resolve([])),
    listSales: vi.fn(() => Promise.resolve([])),
    updateCampaign: vi.fn(),
  },
}));

const mockedApi = vi.mocked(api);

const mockCampaign = {
  id: 'c1',
  name: 'Detail Test Campaign',
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
};

function renderPage(campaignId = 'c1') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[`/campaigns/${campaignId}`]}>
        <Routes>
          <Route path="/campaigns/:id" element={<CampaignDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  );
}

describe('CampaignDetailPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('shows loading state initially', () => {
    mockedApi.getCampaign.mockReturnValue(new Promise(() => {})); // never resolves
    mockedApi.listPurchases.mockReturnValue(new Promise(() => {}));
    mockedApi.listSales.mockReturnValue(new Promise(() => {}));
    renderPage();
    expect(screen.getByTestId('pokeball-loader')).toBeInTheDocument();
  });

  it('renders campaign details when data loads', async () => {
    mockedApi.getCampaign.mockResolvedValue(mockCampaign);
    mockedApi.listPurchases.mockResolvedValue([]);
    mockedApi.listSales.mockResolvedValue([]);
    renderPage('c1');

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Detail Test Campaign' })).toBeInTheDocument();
    });
  });

  it('shows not found message when campaign is missing', async () => {
    mockedApi.getCampaign.mockRejectedValue(new Error('Not found'));
    mockedApi.listPurchases.mockResolvedValue([]);
    mockedApi.listSales.mockResolvedValue([]);
    renderPage('nonexistent');

    await waitFor(() => {
      expect(screen.getByText(/Campaign not found/)).toBeInTheDocument();
    });
  });

  it('cleans up on unmount without errors', () => {
    const { unmount } = renderPage();
    expect(() => unmount()).not.toThrow();
  });
});
