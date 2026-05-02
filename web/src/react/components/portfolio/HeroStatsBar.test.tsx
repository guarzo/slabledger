import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, it, expect } from 'vitest';
import HeroStatsBar from './HeroStatsBar';
import type { PortfolioHealth, CapitalSummary } from '../../../types/campaigns';

function baseHealth(overrides: Partial<PortfolioHealth> = {}): PortfolioHealth {
  return {
    campaigns: [],
    totalDeployedCents: 100_000,
    totalRecoveredCents: 80_000,
    totalAtRiskCents: 20_000,
    overallROI: 0.12,
    realizedROI: 0.12,
    ...overrides,
  };
}

function baseCapital(overrides: Partial<CapitalSummary> = {}): CapitalSummary {
  return {
    outstandingCents: 50_000,
    recoveryRate30dCents: 10_000,
    recoveryRate30dPriorCents: 8_000,
    weeksToCover: 5,
    recoveryTrend: 'improving',
    alertLevel: 'ok',
    unpaidInvoiceCount: 0,
    refundedCents: 0,
    paidCents: 50_000,
    nextInvoiceAmountCents: 0,
    daysUntilInvoiceDue: 0,
    nextInvoicePendingReceiptCents: 0,
    nextInvoiceSellThrough: { totalPurchaseCount: 0, soldCount: 0, totalCostCents: 0, saleRevenueCents: 0 },
    ...overrides,
  };
}

function renderBar(
  health?: PortfolioHealth,
  capital?: CapitalSummary,
  extras: { needsAttentionCount?: number; pendingListingsCount?: number } = {},
) {
  return render(
    <MemoryRouter>
      <HeroStatsBar
        health={health}
        capital={capital}
        needsAttentionCount={extras.needsAttentionCount}
        pendingListingsCount={extras.pendingListingsCount}
      />
    </MemoryRouter>,
  );
}

describe('HeroStatsBar', () => {
  describe('delta chips', () => {
    it('renders positive ROI delta with ▲', () => {
      const health = baseHealth({
        realizedROIDelta: { value: 2.4, label: 'vs last wk' },
      });
      renderBar(health);
      const chip = screen.getByText(/▲/);
      expect(chip).toBeInTheDocument();
      expect(chip.textContent).toContain('2.4%');
      expect(chip.textContent).toContain('vs last wk');
    });

    it('renders negative ROI delta with ▼', () => {
      const health = baseHealth({
        realizedROIDelta: { value: -1.8, label: '30d' },
      });
      renderBar(health);
      const chip = screen.getByText(/▼/);
      expect(chip).toBeInTheDocument();
      expect(chip.textContent).toContain('1.8%');
      expect(chip.textContent).toContain('30d');
    });

    it('formats cents delta through formatCents', () => {
      const health = baseHealth({
        totalRecoveredDelta: { value: 824_000, unit: 'cents', label: 'vs last wk' },
      });
      renderBar(health);
      const chip = screen.getByText(/▲/);
      expect(chip.textContent).toContain('$8,240.00');
      expect(chip.textContent).not.toContain('%');
    });

    it('renders zero delta without arrow', () => {
      const health = baseHealth({
        realizedROIDelta: { value: 0 },
      });
      renderBar(health);
      const chip = screen.getByText(/0\.0%/);
      expect(chip.textContent).not.toContain('▲');
      expect(chip.textContent).not.toContain('▼');
    });

    it('renders negative cents delta with ▼', () => {
      const health = baseHealth({
        totalRecoveredDelta: { value: -500_00, unit: 'cents' },
      });
      renderBar(health);
      const chip = screen.getByText(/▼/);
      expect(chip.textContent).toContain('$500.00');
    });
  });

  describe('ROI magnitude tier', () => {
    it('sets data-mag="normal" for small ROI', () => {
      renderBar(baseHealth({ realizedROI: 0.05 }));
      const section = screen.getByLabelText('Portfolio summary');
      expect(section).toHaveAttribute('data-mag', 'normal');
    });

    it('sets data-mag="big" for ROI >= 0.2', () => {
      renderBar(baseHealth({ realizedROI: 0.25 }));
      const section = screen.getByLabelText('Portfolio summary');
      expect(section).toHaveAttribute('data-mag', 'big');
    });

    it('sets data-mag="huge" for ROI >= 0.5', () => {
      renderBar(baseHealth({ realizedROI: 0.55 }));
      const section = screen.getByLabelText('Portfolio summary');
      expect(section).toHaveAttribute('data-mag', 'huge');
    });

    it('sets data-mag="huge" for large negative ROI', () => {
      renderBar(baseHealth({ realizedROI: -0.6 }));
      const section = screen.getByLabelText('Portfolio summary');
      expect(section).toHaveAttribute('data-mag', 'huge');
    });
  });

  describe('backwards compatibility', () => {
    it('renders without any deltas provided', () => {
      renderBar(baseHealth());
      expect(screen.getByText(/12\.0%/)).toBeInTheDocument();
      expect(screen.queryByText(/▲/)).not.toBeInTheDocument();
      expect(screen.queryByText(/▼/)).not.toBeInTheDocument();
    });

    it('renders with capital but no deltas', () => {
      renderBar(baseHealth(), baseCapital());
      expect(screen.getByText(/12\.0%/)).toBeInTheDocument();
      expect(screen.getByText('Wks to Cover')).toBeInTheDocument();
    });
  });

  describe('negative ROI tone', () => {
    it('sets data-tone="neg" for negative ROI', () => {
      renderBar(baseHealth({ realizedROI: -0.08 }));
      const section = screen.getByLabelText('Portfolio summary');
      expect(section).toHaveAttribute('data-tone', 'neg');
    });

    it('sets data-tone="pos" for positive ROI', () => {
      renderBar(baseHealth({ realizedROI: 0.12 }));
      const section = screen.getByLabelText('Portfolio summary');
      expect(section).toHaveAttribute('data-tone', 'pos');
    });
  });

  describe('ROI formatting', () => {
    it('prepends + on positive ROI', () => {
      renderBar(baseHealth({ realizedROI: 0.12 }));
      expect(screen.getByText(/\+12\.0%/)).toBeInTheDocument();
    });

    it('does not prepend + on negative ROI', () => {
      renderBar(baseHealth({ realizedROI: -0.08 }));
      const roiText = screen.getByText(/-8\.0%/);
      expect(roiText.textContent).not.toMatch(/\+/);
    });
  });

  describe('alert chips', () => {
    it('renders unpaid invoices chip when capital reports unpaid', () => {
      renderBar(baseHealth(), baseCapital({ unpaidInvoiceCount: 3 }));
      const link = screen.getByRole('link', { name: /3 unpaid invoices/i });
      expect(link).toHaveAttribute('href', '/invoices');
    });

    it('hides the unpaid invoice chip when hideInvoiceChip is true', () => {
      const capital = baseCapital({ unpaidInvoiceCount: 3 });
      render(
        <MemoryRouter>
          <HeroStatsBar health={baseHealth()} capital={capital} hideInvoiceChip />
        </MemoryRouter>,
      );
      // Chip itself must be absent
      expect(screen.queryByRole('link', { name: /unpaid invoice/i })).not.toBeInTheDocument();
      // And when the invoice chip is the only alert, the alerts wrapper must not render
      // an empty row — confirm by asserting NO alert link is present at all.
      expect(screen.queryByRole('link', { name: /needs attention/i })).not.toBeInTheDocument();
      expect(screen.queryByRole('link', { name: /pending listing/i })).not.toBeInTheDocument();
    });

    it('renders needs-attention chip routed to /inventory', () => {
      renderBar(baseHealth(), baseCapital(), { needsAttentionCount: 2 });
      const link = screen.getByRole('link', { name: /2 needs attention/i });
      expect(link).toHaveAttribute('href', '/inventory');
    });

    it('renders pending-listings chip routed to /inventory', () => {
      renderBar(baseHealth(), baseCapital(), { pendingListingsCount: 5 });
      const link = screen.getByRole('link', { name: /5 pending listings/i });
      expect(link).toHaveAttribute('href', '/inventory');
    });

    it('hides chips with zero counts', () => {
      renderBar(baseHealth(), baseCapital({ unpaidInvoiceCount: 0 }), {
        needsAttentionCount: 0,
        pendingListingsCount: 0,
      });
      expect(screen.queryByRole('link', { name: /unpaid invoice/i })).not.toBeInTheDocument();
      expect(screen.queryByRole('link', { name: /needs attention/i })).not.toBeInTheDocument();
      expect(screen.queryByRole('link', { name: /pending listing/i })).not.toBeInTheDocument();
    });

    it('renders all three chips together when all counts are nonzero', () => {
      renderBar(
        baseHealth(),
        baseCapital({ unpaidInvoiceCount: 1 }),
        { needsAttentionCount: 4, pendingListingsCount: 7 },
      );
      expect(screen.getByRole('link', { name: /1 unpaid invoice/i })).toBeInTheDocument();
      expect(screen.getByRole('link', { name: /4 needs attention/i })).toBeInTheDocument();
      expect(screen.getByRole('link', { name: /7 pending listings/i })).toBeInTheDocument();
    });
  });

  describe('empty state', () => {
    it('renders no-data state when health is undefined', () => {
      renderBar(undefined);
      const section = screen.getByLabelText('Portfolio summary');
      expect(section).toBeInTheDocument();
      expect(section.textContent).toContain('—');
    });

    it('renders onboarding state when no activity', () => {
      const health = baseHealth({
        totalDeployedCents: 0,
        totalRecoveredCents: 0,
        realizedROI: 0,
      });
      renderBar(health);
      expect(screen.getByText('Welcome to SlabLedger')).toBeInTheDocument();
    });
  });
});
