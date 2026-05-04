import { clsx } from 'clsx';
import type { CampaignPNL } from '../../../types/campaigns';
import { formatCents, formatPct } from '../../utils/formatters';
import styles from '../../components/portfolio/HeroStatsBar.module.css';

interface CampaignsPortfolioHeroProps {
  campaignCount: number;
  pnlMap: Record<string, CampaignPNL>;
}

const TONE_CLASS: Record<string, string> = {
  success: styles.tSuccess,
  neg: styles.tNeg,
  muted: styles.tMuted,
};

function Stat({ label, caption, value, tone }: {
  label: string;
  caption?: string;
  value: string;
  tone?: 'success' | 'neg' | 'muted';
}) {
  return (
    <div className={styles.stat}>
      <div className={styles.statLabel}>{label}</div>
      {caption && <div className={styles.statCaption}>{caption}</div>}
      <div className={clsx(styles.statValue, tone && TONE_CLASS[tone])}>
        {value}
      </div>
    </div>
  );
}

export default function CampaignsPortfolioHero({ campaignCount, pnlMap }: CampaignsPortfolioHeroProps) {
  const pnls = Object.values(pnlMap);
  if (pnls.length === 0) return null;

  const totalSpent = pnls.reduce((sum, p) => sum + p.totalSpendCents, 0);
  const totalRevenue = pnls.reduce((sum, p) => sum + p.totalRevenueCents, 0);
  const totalProfit = pnls.reduce((sum, p) => sum + p.netProfitCents, 0);
  const totalUnsold = pnls.reduce((sum, p) => sum + p.totalPurchases - p.totalSold, 0);
  const roi = totalSpent > 0 ? totalProfit / totalSpent : 0;
  const recoveryPct = totalSpent > 0 ? Math.max(0, Math.min(100, (totalRevenue / totalSpent) * 100)) : 0;

  const negative = roi < 0;
  // Mirror HeroStatsBar's magnitude scale so both ROI values size consistently.
  const magnitude =
    Math.abs(roi) >= 0.5 ? 'huge'
    : Math.abs(roi) >= 0.2 ? 'big'
    : 'normal';

  return (
    <section
      className={styles.hero}
      data-tone={negative ? 'neg' : 'pos'}
      data-mag={magnitude}
      aria-label="Campaigns portfolio summary"
    >
      <div className={styles.roiBlock}>
        <div className={styles.roiLabel}>P&amp;L</div>
        <div className={styles.roiRow}>
          <span className={styles.roiValue}>
            {roi >= 0 ? '+' : ''}{formatPct(roi)}
          </span>
        </div>
        <span className={styles.statCaption}>{formatCents(totalProfit)} all-time</span>
      </div>

      {/* Phase 2.1 (PR #358) refactored HeroStatsBar.module.css to drop
          the single .cluster + .divider classes in favour of a
          .clusterGroup + .subCluster pair. This consumer still uses the
          same module, so we use .subCluster directly to get the flex-wrap
          stat row. The old vertical .divider is gone — the section's
          border-bottom on .hero already provides the visual break.
          Without these renames, styles.cluster + styles.divider were
          undefined and the stats stacked vertically as plain block
          elements with no separator — caught during the closeout
          regression sweep. */}
      <div className={styles.subCluster}>
        <Stat label="Campaigns" value={`${campaignCount}`} />
        <Stat label="Invested" value={formatCents(totalSpent)} />
        <Stat label="Revenue" value={formatCents(totalRevenue)} />
        <Stat label="Unsold" value={`${totalUnsold}`} />
        <Stat
          label="Recovered"
          value={`${recoveryPct.toFixed(0)}%`}
          caption={`${formatCents(totalRevenue)} of ${formatCents(totalSpent)}`}
        />
      </div>
    </section>
  );
}
