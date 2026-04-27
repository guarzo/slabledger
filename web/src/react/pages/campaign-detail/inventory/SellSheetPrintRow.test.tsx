import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import SellSheetPrintRow from './SellSheetPrintRow';
import type { AgingItem } from '../../../../types/campaigns';
import type { Purchase } from '../../../../types/campaigns/core';

const basePurchase: Purchase = {
  id: 'p1',
  campaignId: 'c1',
  cardName: 'GOLEM HOLO',
  certNumber: '133487731',
  cardNumber: '76',
  setName: 'Pokemon Japanese Vending',
  grader: 'PSA',
  gradeValue: 4,
  clValueCents: 27900,
  buyCostCents: 0,
  psaSourcingFeeCents: 0,
  purchaseDate: '2026-01-01',
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
};

const baseItem: AgingItem = {
  purchase: basePurchase,
  daysHeld: 30,
  currentMarket: { lastSoldCents: 26500, lastSoldDate: '2026-03-12', gradePriceCents: 0 },
  recommendedPriceCents: 25000,
};

describe('SellSheetPrintRow', () => {
  it('renders title-cased card name and subtitle', () => {
    render(<SellSheetPrintRow item={baseItem} rowNumber={1} />);
    expect(screen.getByText('Golem Holo')).toBeInTheDocument();
    expect(screen.getByText(/Pokemon Japanese Vending · #76/)).toBeInTheDocument();
  });

  it('renders cert number and a barcode svg', () => {
    const { container } = render(<SellSheetPrintRow item={baseItem} rowNumber={1} />);
    expect(screen.getByText('133487731')).toBeInTheDocument();
    expect(container.querySelector('svg.sell-sheet-print-barcode')).not.toBeNull();
  });

  it('renders CL price without ~ when CL is present', () => {
    render(<SellSheetPrintRow item={baseItem} rowNumber={1} />);
    expect(screen.getByText('$279')).toBeInTheDocument();
  });

  it('renders ~ prefix when CL missing and recommended price is used', () => {
    const item = { ...baseItem, purchase: { ...baseItem.purchase, clValueCents: 0 } };
    render(<SellSheetPrintRow item={item} rowNumber={1} />);
    expect(screen.getByText('~$250')).toBeInTheDocument();
  });

  it('renders em-dash when both CL and recommended are missing', () => {
    const item = {
      ...baseItem,
      purchase: { ...baseItem.purchase, clValueCents: 0 },
      recommendedPriceCents: 0,
    };
    render(<SellSheetPrintRow item={item} rowNumber={1} />);
    expect(screen.getByText('—')).toBeInTheDocument();
  });

  it('does not render a last-sale cell', () => {
    const { container } = render(<SellSheetPrintRow item={baseItem} rowNumber={1} />);
    expect(container.querySelector('[data-cell="last-sale"]')).toBeNull();
  });

  it('renders the row number and an empty Agreed $ cell', () => {
    const { container } = render(<SellSheetPrintRow item={baseItem} rowNumber={7} />);
    expect(screen.getByText('7')).toBeInTheDocument();
    const agreed = container.querySelector('[data-cell="agreed"]');
    expect(agreed?.textContent ?? '').toBe('');
  });
});
