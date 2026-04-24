import { Link } from 'react-router-dom';
import { clsx } from 'clsx';
import type { PortfolioHealth, CapitalSummary } from '../../../types/campaigns';
import { formatCents, formatPct, formatWeeksToCover } from '../../utils/formatters';
import { EmptyState } from '../../ui';
import { StatusPill } from '../../ui/StatusPill';
import TrendArrow from '../../ui/TrendArrow';
import styles from './HeroStatsBar.module.css';

const trendToArrow = { improving: 'up', declining: 'down', stable: 'stable' } as const;

interface HeroStatsBarProps {
  health?: PortfolioHealth;
  capital?: CapitalSummary;
}

export default function HeroStatsBar({ health, capital }: HeroStatsBarProps) {
  if (!health) return null;

  const hasActivity = health.totalDeployedCents > 0 || health.totalRecoveredCents > 0 || health.realizedROI !== 0;
  if (!hasActivity) {
    return (
      <div className="mb-6">
        <EmptyState
          icon="📊"
          title="Welcome to SlabLedger"
          description="Your portfolio dashboard will come alive once you start tracking."
          compact
          steps={['Create a campaign', 'Import PSA purchases', 'Record sales as you go']}
        />
      </div>
    );
  }

  const roi = health.realizedROI ?? 0;
  const negative = roi < 0;

  return (
    <section className={styles.hero} data-tone={negative ? 'neg' : 'pos'} aria-label="Portfolio summary">
      <div className={styles.roiBlock}>
        <div className={styles.roiLabel}>Realized ROI</div>
        <div className={styles.roiRow}>
          <span className={styles.roiValue}>{formatPct(roi)}</span>
        </div>
      </div>

      {/* Money group */}
      <div className={styles.group}>
        <Stat label="Deployed" value={formatCents(health.totalDeployedCents ?? 0)} />
        <Stat label="Recovered" value={formatCents(health.totalRecoveredCents ?? 0)} />
        <Stat label="At Risk" value={formatCents(health.totalAtRiskCents ?? 0)} tone={(health.totalAtRiskCents ?? 0) > 0 ? 'warn' : undefined} />
      </div>

      {capital && (
        <>
          <div className={styles.divider} aria-hidden />
          {/* Time group */}
          <div className={styles.group}>
            <Stat
              label="Wks to Cover"
              value={capital.outstandingCents === 0 && capital.recoveryRate30dCents > 0
                ? '0'
                : formatWeeksToCover(capital.weeksToCover, capital.recoveryRate30dCents > 0)}
              tone={capital.alertLevel === 'critical' ? 'neg' : capital.alertLevel === 'warning' ? 'warn' : undefined}
            />
            <Stat
              label="Outstanding"
              value={formatCents(capital.outstandingCents)}
              tone={capital.alertLevel === 'critical' ? 'neg' : capital.alertLevel === 'warning' ? 'warn' : undefined}
            />
            {capital.recoveryRate30dCents > 0 && (
              <div className={styles.stat}>
                <div className={styles.statLabel}>30d Recovery</div>
                <div className={styles.statValue}>
                  {formatCents(capital.recoveryRate30dCents)}
                  <TrendArrow trend={trendToArrow[capital.recoveryTrend]} />
                </div>
              </div>
            )}
          </div>
        </>
      )}

      {capital && capital.unpaidInvoiceCount > 0 && (
        <div className={styles.alerts}>
          <Link to="/invoices" className={styles.alertLink}>
            <StatusPill tone="warning">
              {capital.unpaidInvoiceCount} unpaid invoice{capital.unpaidInvoiceCount !== 1 ? 's' : ''} →
            </StatusPill>
          </Link>
        </div>
      )}
    </section>
  );
}

function Stat({ label, value, tone }: { label: string; value: string; tone?: 'warn' | 'neg' }) {
  return (
    <div className={styles.stat}>
      <div className={styles.statLabel}>{label}</div>
      <div className={clsx(styles.statValue, tone === 'warn' && styles.tWarn, tone === 'neg' && styles.tNeg)}>
        {value}
      </div>
    </div>
  );
}
