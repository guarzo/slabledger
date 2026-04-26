import { render, screen } from '@testing-library/react';
import CampaignHeroStats from './CampaignHeroStats';
import heroStyles from '../../components/portfolio/HeroStatsBar.module.css';

describe('CampaignHeroStats', () => {
  it('renders all stats with values', () => {
    render(
      <CampaignHeroStats
        totalSpentCents={358596}
        totalProfitCents={0}
        totalRevenueCents={0}
        roi={null}
        purchaseCount={5}
        saleCount={0}
        sellThroughPct="0"
        avgDaysToSell={null}
      />,
    );
    expect(screen.getByText('Total Spent')).toBeInTheDocument();
    expect(screen.getByText('$3,585.96')).toBeInTheDocument();
    expect(screen.getByText('Net Profit')).toBeInTheDocument();
    expect(screen.getByText('ROI')).toBeInTheDocument();
    // ROI shows em-dash when no sales yet (purchaseCount > 0, saleCount === 0)
    expect(screen.getByTestId('stat-value-roi')).toHaveTextContent('—');
  });

  it('shows em-dash for derived metrics when no purchase activity', () => {
    render(
      <CampaignHeroStats
        totalSpentCents={0}
        totalProfitCents={0}
        totalRevenueCents={0}
        roi={null}
        purchaseCount={0}
        saleCount={0}
        sellThroughPct="0"
        avgDaysToSell={null}
      />,
    );
    expect(screen.getByTestId('stat-value-roi')).toHaveTextContent('—');
    expect(screen.getByTestId('stat-value-sell-through')).toHaveTextContent('—');
  });

  it('colors net profit green when positive', () => {
    render(
      <CampaignHeroStats
        totalSpentCents={100000}
        totalProfitCents={25000}
        totalRevenueCents={125000}
        roi={0.25}
        purchaseCount={5}
        saleCount={3}
        sellThroughPct="60"
        avgDaysToSell={4.2}
      />,
    );
    const profit = screen.getByTestId('stat-value-net-profit');
    expect(profit).toHaveClass(heroStyles.tSuccess);
  });
});
