import { clsx } from 'clsx';
import { formatCents, formatPct } from '../../utils/formatters';
import styles from '../../components/portfolio/HeroStatsBar.module.css';

interface Props {
  totalSpentCents: number;
  totalProfitCents: number;
  totalRevenueCents: number;
  roi: number | null;
  purchaseCount: number;
  saleCount: number;
  sellThroughPct: string;
  avgDaysToSell: number | null;
}

const DASH = '—';

/**
 * Campaign-scoped hero stats. Mirrors the visual pattern of `HeroStatsBar`
 * but with campaign-specific stat semantics (Total Spent / Net Profit /
 * ROI / Sell-Through / Cards / Days to Sell). Reuses HeroStatsBar's CSS
 * module for consistency.
 */
export default function CampaignHeroStats({
  totalSpentCents,
  totalProfitCents,
  totalRevenueCents,
  roi,
  purchaseCount,
  saleCount,
  sellThroughPct,
  avgDaysToSell,
}: Props) {
  const hasPurchases = purchaseCount > 0;
  const hasSales = saleCount > 0;
  const profitTone = hasSales ? (totalProfitCents >= 0 ? 'success' : 'problem') : undefined;
  const roiTone = hasSales && roi !== null ? (roi >= 0 ? 'success' : 'problem') : undefined;

  // No-sales path: render the two stats that are actually known, drop the
  // five dashed placeholders, and let the italic line carry the state.
  if (hasPurchases && !hasSales) {
    return (
      <>
        <section className={styles.hero} aria-label="Campaign summary">
          <div className={styles.subCluster}>
            <HeroStat
              testId="stat-value-total-spent"
              label="Total Spent"
              value={formatCents(totalSpentCents)}
            />
            <HeroStat
              testId="stat-value-cards-bought"
              label="Cards Bought"
              value={String(purchaseCount)}
            />
          </div>
        </section>
        <p className="italic text-sm text-[var(--text-muted)] mt-2">
          Awaiting first sale
        </p>
      </>
    );
  }

  return (
    <section className={styles.hero} aria-label="Campaign summary">
      {/* Capital row */}
      <div className={styles.subCluster}>
        <HeroStat
          testId="stat-value-total-spent"
          label="Total Spent"
          value={formatCents(totalSpentCents)}
        />
        <HeroStat
          testId="stat-value-net-profit"
          label="Net Profit"
          value={hasSales ? formatCents(totalProfitCents) : DASH}
          tone={profitTone}
        />
        <HeroStat
          testId="stat-value-revenue"
          label="Revenue"
          value={hasSales ? formatCents(totalRevenueCents) : DASH}
        />
      </div>

      {/* Velocity row */}
      <div className={styles.subCluster}>
        <HeroStat
          testId="stat-value-roi"
          label="ROI"
          value={hasSales && roi !== null ? formatPct(roi) : DASH}
          tone={roiTone}
        />
        <HeroStat
          testId="stat-value-sell-through"
          label="Sell-Through"
          value={hasPurchases ? `${sellThroughPct}%` : DASH}
        />
        <HeroStat
          testId="stat-value-cards-bought"
          label="Cards Bought"
          value={String(purchaseCount)}
        />
        <HeroStat
          testId="stat-value-avg-days"
          label="Avg Days to Sell"
          value={hasSales && avgDaysToSell !== null ? avgDaysToSell.toFixed(1) : DASH}
        />
      </div>
    </section>
  );
}

function HeroStat({
  testId,
  label,
  value,
  tone,
}: {
  testId: string;
  label: string;
  value: string;
  tone?: 'success' | 'problem' | 'atRisk' | 'waiting';
}) {
  const TONE_CLASS: Record<string, string> = {
    success: styles.tSuccess,
    problem: styles.tProblem,
    atRisk: styles.tAtRisk,
    waiting: styles.tWaiting,
  };
  return (
    <div className={styles.stat}>
      <div className={styles.statLabel}>{label}</div>
      <div data-testid={testId} className={clsx(styles.statValue, tone && TONE_CLASS[tone])}>
        {value}
      </div>
    </div>
  );
}
