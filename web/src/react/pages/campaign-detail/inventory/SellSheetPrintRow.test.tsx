import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import SellSheetPrintRow from './SellSheetPrintRow';
import { mostRecentSale } from './utils';
import type { AgingItem } from '../../../../types/campaigns';
import type { Purchase } from '../../../../types/campaigns/core';
import type { CompSummary } from '../../../../types/campaigns/analytics';

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

  it('renders last-sale price and date when present', () => {
    render(<SellSheetPrintRow item={baseItem} rowNumber={1} />);
    expect(screen.getByText('$265')).toBeInTheDocument();
    expect(screen.getByText('03/12/26')).toBeInTheDocument();
  });

  it('leaves last-sale cell blank when no last sale data', () => {
    const item = { ...baseItem, currentMarket: undefined };
    const { container } = render(<SellSheetPrintRow item={item} rowNumber={1} />);
    expect(container.querySelector('[data-cell="last-sale"]')?.textContent).toBe('');
  });

  it('prefers compSummary.lastSaleCents over currentMarket.lastSoldCents for the last-sale cell', () => {
    // Construct a fixture where compSummary.lastSaleCents and
    // currentMarket.lastSoldCents differ; mostRecentSale should pick
    // compSummary first, and SellSheetPrintRow should render the
    // compSummary value (recent.cents -> lastSoldCents in the component).
    const compSummary: CompSummary = {
      gemRateId: 'gr-1',
      totalComps: 10,
      recentComps: 4,
      medianCents: 30000,
      highestCents: 40000,
      lowestCents: 20000,
      trend90d: 0,
      compsAboveCL: 0,
      compsAboveCost: 0,
      byPlatform: [],
      lastSaleDate: '2026-04-20',
      lastSaleCents: 31200,
    };
    const item: AgingItem = {
      ...baseItem,
      currentMarket: { lastSoldCents: 26500, lastSoldDate: '2026-03-12', gradePriceCents: 0 },
      compSummary,
    };

    const recent = mostRecentSale(item);
    expect(recent?.cents).toBe(compSummary.lastSaleCents);
    expect(recent?.date).toBe(compSummary.lastSaleDate);

    render(<SellSheetPrintRow item={item} rowNumber={1} />);
    // $312 comes from compSummary.lastSaleCents (31200), NOT $265 from currentMarket.lastSoldCents
    expect(screen.getByText('$312')).toBeInTheDocument();
    expect(screen.queryByText('$265')).toBeNull();
    expect(screen.getByText('04/20/26')).toBeInTheDocument();
  });

  it('falls back to currentMarket.lastSoldCents when compSummary is absent', () => {
    // No compSummary on the item — mostRecentSale should fall back to the
    // snapshot's lastSoldCents/lastSoldDate, and the row should render that.
    const item: AgingItem = {
      ...baseItem,
      compSummary: undefined,
      currentMarket: { lastSoldCents: 26500, lastSoldDate: '2026-03-12', gradePriceCents: 0 },
    };

    const recent = mostRecentSale(item);
    expect(recent?.cents).toBe(item.currentMarket?.lastSoldCents);
    expect(recent?.date).toBe(item.currentMarket?.lastSoldDate);

    render(<SellSheetPrintRow item={item} rowNumber={1} />);
    expect(screen.getByText('$265')).toBeInTheDocument();
    expect(screen.getByText('03/12/26')).toBeInTheDocument();
  });

  it('renders the row number and an empty Agreed $ cell', () => {
    const { container } = render(<SellSheetPrintRow item={baseItem} rowNumber={7} />);
    expect(screen.getByText('7')).toBeInTheDocument();
    const agreed = container.querySelector('[data-cell="agreed"]');
    expect(agreed?.textContent ?? '').toBe('');
  });
});
