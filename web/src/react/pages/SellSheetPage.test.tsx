import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import type { SellSheet, SellSheetItem } from '../../types/campaigns';

function makeItem(overrides: Partial<SellSheetItem>): SellSheetItem {
  return {
    purchaseId: overrides.certNumber ?? 'p',
    certNumber: 'CERT',
    cardName: 'Card',
    setName: 'Set',
    cardNumber: '1',
    grade: 10,
    grader: 'PSA',
    cardYear: '2022',
    buyCostCents: 1000,
    costBasisCents: 1000,
    clValueCents: 5000,
    recommendation: 'sell',
    targetSellPrice: 10000,
    minimumAcceptPrice: 8000,
    ...overrides,
  };
}

const fixture: SellSheet = {
  generatedAt: '2026-05-03T00:00:00Z',
  campaignName: 'All',
  items: [
    makeItem({ certNumber: 'A', grade: 10, grader: 'PSA', cardYear: '2022', targetSellPrice: 50000 }),
    makeItem({ certNumber: 'B', grade: 9, grader: 'PSA', cardYear: '2022', targetSellPrice: 40000 }),
    makeItem({ certNumber: 'C', grade: 9, grader: 'PSA', cardYear: '1999', targetSellPrice: 30000 }),
    makeItem({ certNumber: 'D', grade: 9, grader: 'PSA', cardYear: '2021', targetSellPrice: 250000 }),
    makeItem({ certNumber: 'E', grade: 8, grader: 'PSA', cardYear: '2022', targetSellPrice: 5000 }),
  ],
  totals: {
    totalCostBasis: 0,
    totalExpectedRevenue: 0,
    totalProjectedProfit: 0,
    itemCount: 5,
    skippedItems: 0,
  },
};

vi.mock('../queries/useCampaignQueries', () => ({
  useGlobalSellSheet: () => ({ data: fixture, isLoading: false, error: null }),
}));

// Stub barcode lib used by SellSheetPrintRow
vi.mock('jsbarcode', () => ({ default: () => undefined }));

import SellSheetPage from './SellSheetPage';

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <SellSheetPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('SellSheetPage', () => {
  it('renders all seven slice labels', () => {
    renderPage();
    expect(screen.getByText('PSA 10s')).toBeInTheDocument();
    expect(screen.getByText('Modern (2020+)')).toBeInTheDocument();
    expect(screen.getByText('Vintage (pre-2020)')).toBeInTheDocument();
    expect(screen.getByText('High-Value ($1,000+)')).toBeInTheDocument();
    expect(screen.getByText('Under $1,000')).toBeInTheDocument();
    expect(screen.getByText('By Grade (local card store)')).toBeInTheDocument();
    expect(screen.getByText('Full List')).toBeInTheDocument();
  });

  it('shows print view when a non-empty slice Print button is clicked', () => {
    const printSpy = vi.spyOn(window, 'print').mockImplementation(() => undefined);
    const { unmount } = renderPage();
    const psa10Row = screen.getByText('PSA 10s').closest('li');
    expect(psa10Row).not.toBeNull();
    const printBtn = psa10Row!.querySelector('button')!;
    expect(printBtn).not.toBeDisabled();
    fireEvent.click(printBtn);
    expect(screen.getByTestId('sell-sheet-print-view')).toBeInTheDocument();
    // Unmount before restoring the spy so any pending rAF that fires
    // window.print() hits the mock, not the real (jsdom-unsupported) function.
    unmount();
    printSpy.mockRestore();
  });
});
