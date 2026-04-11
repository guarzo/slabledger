import { render, screen } from '@testing-library/react';
import MobileCard from './MobileCard';
import type { AgingItem } from '../../../../types/campaigns';

// Minimal fixture for AgingItem
const createMockItem = (overrides?: Partial<AgingItem>): AgingItem => ({
  purchase: {
    id: '1',
    campaignId: 'camp1',
    cardName: 'Test Card',
    setName: 'Test Set',
    certNumber: '12345',
    grader: 'PSA',
    gradeValue: 9,
    cardNumber: '001',
    buyCostCents: 10000,
    psaSourcingFeeCents: 500,
    clValueCents: 0,
    frontImageUrl: 'https://example.com/card.jpg',
    purchaseDate: '2026-03-31',
    receivedAt: undefined,
    createdAt: '2026-03-31T00:00:00Z',
    updatedAt: '2026-03-31T00:00:00Z',
  },
  daysHeld: 5,
  campaignName: 'Test Campaign',
  currentMarket: {
    medianCents: 15000,
    gradePriceCents: 14000,
    conservativeCents: 13000,
    optimisticCents: 17000,
    lastSoldCents: 15500,
    lastSoldDate: '2026-04-10',
    p10Cents: 12000,
    p90Cents: 18000,
    avg7DayCents: 14500,
    lowestListCents: 14500,
    activeListings: 3,
    salesLast30d: 12,
    salesLast90d: 35,
    dailyVelocity: 0.4,
    monthlyVelocity: undefined,
    trend30d: 0.05,
    confidence: 0.85,
    sourcePrices: [],
  },
  signal: undefined,
  priceAnomaly: false,
  ...overrides,
});

describe('MobileCard', () => {
  describe('Green dot indicator for in-hand items', () => {
    it('does not render green dot when receivedAt is not set', () => {
      const item = createMockItem();
      render(
        <MobileCard
          item={item}
          selected={false}
          onToggle={() => {}}
          onRecordSale={() => {}}
        />
      );

      const dots = screen.queryAllByTestId('in-hand-indicator');
      expect(dots).toHaveLength(0);
    });

    it('renders green dot when receivedAt is set', () => {
      const item = createMockItem({
        purchase: {
          ...createMockItem().purchase,
          receivedAt: '2026-04-05T10:30:00Z',
        },
      });

      render(
        <MobileCard
          item={item}
          selected={false}
          onToggle={() => {}}
          onRecordSale={() => {}}
        />
      );

      const greenDot = screen.getByTestId('in-hand-indicator');
      expect(greenDot).toBeInTheDocument();
    });

    it('sets correct styles on green dot', () => {
      const item = createMockItem({
        purchase: {
          ...createMockItem().purchase,
          receivedAt: '2026-04-05T10:30:00Z',
        },
      });

      render(
        <MobileCard
          item={item}
          selected={false}
          onToggle={() => {}}
          onRecordSale={() => {}}
        />
      );

      const greenDot = screen.getByTestId('in-hand-indicator');

      expect(greenDot).toBeInTheDocument();
      expect(greenDot).toHaveStyle({
        width: '10px',
        height: '10px',
        borderRadius: '50%',
      });
    });

    it('displays formatted date in tooltip', () => {
      const item = createMockItem({
        purchase: {
          ...createMockItem().purchase,
          receivedAt: '2026-04-05T10:30:00Z',
        },
      });

      render(
        <MobileCard
          item={item}
          selected={false}
          onToggle={() => {}}
          onRecordSale={() => {}}
        />
      );

      const greenDot = screen.getByTestId('in-hand-indicator');

      expect(greenDot).toHaveAttribute('title', expect.stringContaining('In hand since'));
      expect(greenDot).toHaveAttribute('title', expect.stringContaining('Apr'));
      expect(greenDot).toHaveAttribute('title', expect.stringContaining('5'));
      expect(greenDot).toHaveAttribute('title', expect.stringContaining('2026'));
    });
  });
});
