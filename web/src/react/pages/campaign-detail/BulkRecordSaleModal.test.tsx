import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import BulkRecordSaleModal from './BulkRecordSaleModal';
import { ToastProvider } from '../../contexts/ToastContext';
import type { AgingItem } from '../../../types/campaigns';

vi.mock('../../../js/api', () => ({
  api: {
    createBulkSales: vi.fn(),
  },
}));

import { api } from '../../../js/api';

function makeItem(id: string, campaignId: string, clValueCents: number): AgingItem {
  return {
    purchase: {
      id,
      campaignId,
      cardName: `Card ${id}`,
      setName: 'Set',
      certNumber: id,
      grader: 'PSA',
      gradeValue: 10,
      cardNumber: id,
      buyCostCents: 1000,
      psaSourcingFeeCents: 0,
      clValueCents,
      frontImageUrl: '',
      purchaseDate: '2026-01-01',
      receivedAt: '2026-01-02',
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    },
    daysHeld: 10,
    campaignName: `Campaign ${campaignId}`,
    currentMarket: undefined,
    signal: undefined,
    priceAnomaly: false,
  } as AgingItem;
}

function renderModal(items: AgingItem[], onSuccess = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <ToastProvider>
        <BulkRecordSaleModal open={true} onClose={vi.fn()} onSuccess={onSuccess} items={items} />
      </ToastProvider>
    </QueryClientProvider>
  );
}

describe('BulkRecordSaleModal', () => {
  beforeEach(() => {
    vi.mocked(api.createBulkSales).mockReset();
  });

  it('renders the count of selected cards in the title', () => {
    const items = [makeItem('1', 'c1', 5000), makeItem('2', 'c1', 6000)];
    renderModal(items);
    expect(screen.getByText(/Record Sale \(2 cards\)/i)).toBeInTheDocument();
  });

  it('groups items by campaignId on submit and calls api.createBulkSales once per campaign', async () => {
    vi.mocked(api.createBulkSales).mockResolvedValue({ created: 1, failed: 0 });
    const items = [
      makeItem('1', 'c1', 5000),
      makeItem('2', 'c1', 6000),
      makeItem('3', 'c2', 7000),
    ];
    renderModal(items);

    const pctInput = screen.getByLabelText(/% of CL/i) as HTMLInputElement;
    fireEvent.change(pctInput, { target: { value: '70' } });

    fireEvent.click(screen.getByRole('button', { name: /Record 3 Sales/i }));

    await waitFor(() => {
      expect(vi.mocked(api.createBulkSales)).toHaveBeenCalledTimes(2);
    });
    expect(vi.mocked(api.createBulkSales)).toHaveBeenCalledWith(
      'c1',
      expect.any(String),
      expect.any(String),
      expect.arrayContaining([
        { purchaseId: '1', salePriceCents: 3500, priceSource: 'estimated' },
        { purchaseId: '2', salePriceCents: 4200, priceSource: 'estimated' },
      ]),
    );
    expect(vi.mocked(api.createBulkSales)).toHaveBeenCalledWith(
      'c2',
      expect.any(String),
      expect.any(String),
      [{ purchaseId: '3', salePriceCents: 4900, priceSource: 'estimated' }],
    );
  });

  it('blocks submit when any row resolves to $0', async () => {
    const items = [makeItem('1', 'c1', 5000), makeItem('2', 'c1', 0)];
    renderModal(items);

    const pctInput = screen.getByLabelText(/% of CL/i) as HTMLInputElement;
    fireEvent.change(pctInput, { target: { value: '70' } });

    fireEvent.click(screen.getByRole('button', { name: /Record 2 Sales/i }));

    await waitFor(() => {
      expect(screen.getByText(/no sale price set/i)).toBeInTheDocument();
    });
    expect(vi.mocked(api.createBulkSales)).not.toHaveBeenCalled();
  });

  it('shows a live gross total that updates as the fill-all input changes', () => {
    const items = [makeItem('1', 'c1', 5000), makeItem('2', 'c1', 6000)];
    renderModal(items);

    expect(screen.getByTestId('bulk-sale-total').textContent).toMatch(/\$0\.00/);

    const pctInput = screen.getByLabelText(/% of CL/i) as HTMLInputElement;
    fireEvent.change(pctInput, { target: { value: '70' } });

    expect(screen.getByTestId('bulk-sale-total').textContent).toMatch(/\$77\.00/);
  });

  it('hides the per-row review by default and reveals it on click', () => {
    const items = [makeItem('1', 'c1', 5000), makeItem('2', 'c1', 6000)];
    renderModal(items);

    expect(screen.queryByText('Card 1')).not.toBeInTheDocument();
    expect(screen.queryByText('Card 2')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /Review prices \(2\)/i }));

    expect(screen.getByText('Card 1')).toBeInTheDocument();
    expect(screen.getByText('Card 2')).toBeInTheDocument();
  });

  it('persists per-row overrides when the fill-all percent changes', async () => {
    vi.mocked(api.createBulkSales).mockResolvedValue({ created: 2, failed: 0 });
    const items = [makeItem('1', 'c1', 5000), makeItem('2', 'c1', 6000)];
    renderModal(items);

    const pctInput = screen.getByLabelText(/% of CL/i) as HTMLInputElement;
    fireEvent.change(pctInput, { target: { value: '70' } });

    fireEvent.click(screen.getByRole('button', { name: /Review prices/i }));

    const row1Input = screen.getByLabelText(/override price for Card 1/i) as HTMLInputElement;
    fireEvent.change(row1Input, { target: { value: '25' } });

    fireEvent.change(pctInput, { target: { value: '80' } });

    fireEvent.click(screen.getByRole('button', { name: /Record 2 Sales/i }));

    await waitFor(() => {
      expect(vi.mocked(api.createBulkSales)).toHaveBeenCalledWith(
        'c1',
        expect.any(String),
        expect.any(String),
        expect.arrayContaining([
          { purchaseId: '1', salePriceCents: 2500, priceSource: 'estimated' },
          { purchaseId: '2', salePriceCents: 4800, priceSource: 'estimated' },
        ]),
      );
    });
  });

  it('reset link reverts a row override to the computed price', () => {
    const items = [makeItem('1', 'c1', 5000)];
    renderModal(items);

    const pctInput = screen.getByLabelText(/% of CL/i) as HTMLInputElement;
    fireEvent.change(pctInput, { target: { value: '70' } });

    fireEvent.click(screen.getByRole('button', { name: /Review prices/i }));

    const row1Input = screen.getByLabelText(/override price for Card 1/i) as HTMLInputElement;
    fireEvent.change(row1Input, { target: { value: '25' } });
    expect(row1Input.value).toBe('25');

    fireEvent.click(screen.getByRole('button', { name: /reset to computed/i }));
    expect(row1Input.value).toBe('35');
  });

  it('includes per-row listing details and sale reason keyed by purchaseId in the payload', async () => {
    vi.mocked(api.createBulkSales).mockResolvedValue({ created: 2, failed: 0 });
    const items = [makeItem('1', 'c1', 5000), makeItem('2', 'c1', 6000)];
    renderModal(items);

    const pctInput = screen.getByLabelText(/% of CL/i) as HTMLInputElement;
    fireEvent.change(pctInput, { target: { value: '70' } });

    fireEvent.click(screen.getByRole('button', { name: /Review prices/i }));

    fireEvent.change(screen.getByLabelText(/Original list price for Card 1/i), { target: { value: '20' } });
    fireEvent.change(screen.getByLabelText(/Price reductions for Card 1/i), { target: { value: '3' } });
    fireEvent.change(screen.getByLabelText(/Days listed for Card 1/i), { target: { value: '12' } });
    fireEvent.change(screen.getByLabelText(/Sale reason for Card 1/i), { target: { value: 'bulk_lot' } });

    fireEvent.change(screen.getByLabelText(/Days listed for Card 2/i), { target: { value: '5' } });

    fireEvent.click(screen.getByRole('button', { name: /Record 2 Sales/i }));

    await waitFor(() => {
      expect(vi.mocked(api.createBulkSales)).toHaveBeenCalledWith(
        'c1',
        expect.any(String),
        expect.any(String),
        expect.arrayContaining([
          expect.objectContaining({
            purchaseId: '1',
            originalListPriceCents: 2000,
            priceReductions: 3,
            daysListed: 12,
            saleReason: 'bulk_lot',
          }),
          expect.objectContaining({
            purchaseId: '2',
            daysListed: 5,
          }),
        ]),
      );
    });
  });

  it('clears per-row listing details when the modal closes and reopens', () => {
    const items = [makeItem('1', 'c1', 5000)];
    renderModal(items);

    fireEvent.click(screen.getByRole('button', { name: /Review prices/i }));
    fireEvent.change(screen.getByLabelText(/Days listed for Card 1/i), { target: { value: '7' } });
    expect((screen.getByLabelText(/Days listed for Card 1/i) as HTMLInputElement).value).toBe('7');

    // Simulate the user closing the modal (Cancel triggers handleClose -> reset()).
    fireEvent.click(screen.getByRole('button', { name: /Cancel/i }));

    fireEvent.click(screen.getByRole('button', { name: /Review prices/i }));
    expect((screen.getByLabelText(/Days listed for Card 1/i) as HTMLInputElement).value).toBe('');
  });

  it('keeps entered per-row values on the row that failed after a partial-failure retry', async () => {
    vi.mocked(api.createBulkSales).mockResolvedValue({
      created: 1,
      failed: 1,
      errors: [{ purchaseId: '2', error: 'bad reason' }],
    });
    const items = [makeItem('1', 'c1', 5000), makeItem('2', 'c1', 6000)];
    renderModal(items);

    const pctInput = screen.getByLabelText(/% of CL/i) as HTMLInputElement;
    fireEvent.change(pctInput, { target: { value: '70' } });

    fireEvent.click(screen.getByRole('button', { name: /Review prices/i }));
    fireEvent.change(screen.getByLabelText(/Days listed for Card 2/i), { target: { value: '9' } });

    fireEvent.click(screen.getByRole('button', { name: /Record 2 Sales/i }));

    await waitFor(() => {
      expect(screen.getByText(/Record Sale \(1 cards\)/i)).toBeInTheDocument();
    });

    // Review panel stays open across the narrowing retry.
    expect((screen.getByLabelText(/Days listed for Card 2/i) as HTMLInputElement).value).toBe('9');
  });

  it('exposes each row\'s listing-detail inputs via row-specific accessible labels', () => {
    const items = [makeItem('1', 'c1', 5000), makeItem('2', 'c1', 6000)];
    renderModal(items);

    fireEvent.click(screen.getByRole('button', { name: /Review prices/i }));

    expect(screen.getByLabelText(/Days listed for Card 1/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/Days listed for Card 2/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/Sale reason for Card 1/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/Sale reason for Card 2/i)).toBeInTheDocument();
  });

  it('tags pctOfCL rows as estimated and flat-price rows as manual', async () => {
    vi.mocked(api.createBulkSales).mockResolvedValue({ created: 1, failed: 0 });
    const items = [makeItem('1', 'c1', 5000)];
    const { unmount } = renderModal(items);

    fireEvent.change(screen.getByLabelText(/% of CL/i), { target: { value: '70' } });
    fireEvent.click(screen.getByRole('button', { name: /Record 1 Sales/i }));

    await waitFor(() => {
      expect(vi.mocked(api.createBulkSales)).toHaveBeenCalledWith(
        'c1',
        expect.any(String),
        expect.any(String),
        [expect.objectContaining({ purchaseId: '1', priceSource: 'estimated' })],
      );
    });

    unmount();
    vi.mocked(api.createBulkSales).mockReset();
    vi.mocked(api.createBulkSales).mockResolvedValue({ created: 1, failed: 0 });
    renderModal(items);

    fireEvent.click(screen.getByLabelText(/Fixed price/i));
    fireEvent.change(screen.getByLabelText(/Flat \$/i), { target: { value: '42' } });
    fireEvent.click(screen.getByRole('button', { name: /Record 1 Sales/i }));

    await waitFor(() => {
      expect(vi.mocked(api.createBulkSales)).toHaveBeenCalledWith(
        'c1',
        expect.any(String),
        expect.any(String),
        [expect.objectContaining({ purchaseId: '1', salePriceCents: 4200, priceSource: 'manual' })],
      );
    });
  });
});
