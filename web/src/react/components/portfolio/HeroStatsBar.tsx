import { Link } from 'react-router-dom';
import { clsx } from 'clsx';
import type { PortfolioHealth, PortfolioDelta, CapitalSummary } from '../../../types/campaigns';
import { formatCents, formatPct, formatWeeksToCover } from '../../utils/formatters';
import { EmptyState, StatusPill } from '../../ui';
import TrendArrow from '../../ui/TrendArrow';
import styles from './HeroStatsBar.module.css';

const trendToArrow = { improving: 'up', declining: 'down', stable: 'stable' } as const;

interface HeroStatsBarProps {
  health?: PortfolioHealth;
  capital?: CapitalSummary;
}

export default function HeroStatsBar({ health, capital }: HeroStatsBarProps) {
  if (!health) {
    return (
      <section className={styles.hero} aria-label="Portfolio summary">
        <div className={styles.roiBlock}>
          <div className={styles.roiLabel}>Realized ROI</div>
          <div className={styles.roiRow}>
            <span className={clsx(styles.roiValue, styles.tMuted)}>—</span>
          </div>
        </div>
      </section>
    );
  }

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
  const magnitude =
    Math.abs(roi) >= 0.5 ? 'huge'
    : Math.abs(roi) >= 0.2 ? 'big'
    : 'normal';

  return (
    <section
      className={styles.hero}
      data-tone={negative ? 'neg' : 'pos'}
      data-mag={magnitude}
      aria-label="Portfolio summary"
    >
      <div className={styles.roiBlock}>
        <div className={styles.roiLabel}>Realized ROI</div>
        <div className={styles.roiRow}>
          <span className={styles.roiValue}>
            {roi >= 0 ? '+' : ''}{formatPct(roi)}
          </span>
          {health.realizedROIDelta && <DeltaChip delta={health.realizedROIDelta} />}
        </div>
      </div>

      {/* Money group */}
      <div className={styles.group}>
        <Stat label="Deployed" value={formatCents(health.totalDeployedCents ?? 0)} />
        <Stat
          label="Recovered"
          value={formatCents(health.totalRecoveredCents ?? 0)}
          delta={health.totalRecoveredDelta}
        />
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
              tone={capital.alertLevel === 'critical' ? 'neg'
                : capital.alertLevel === 'warning' ? 'warn'
                : capital.recoveryRate30dCents === 0 ? 'muted'
                : 'success'}
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

function DeltaChip({ delta, small }: { delta: PortfolioDelta; small?: boolean }) {
  const pos = delta.value > 0;
  const neg = delta.value < 0;
  const arrow = pos ? '▲' : neg ? '▼' : '';
  const formatted =
    delta.unit === 'cents' ? formatCents(Math.abs(delta.value))
    : `${Math.abs(delta.value).toFixed(1)}`;
  const suffix = delta.unit === 'cents' ? '' : '%';
  return (
    <span className={clsx(styles.delta, pos ? styles.dPos : neg ? styles.dNeg : undefined, small && styles.dSmall)}>
      {arrow && `${arrow} `}{formatted}{suffix}
      {delta.label && <span className={styles.dMeta}> {delta.label}</span>}
    </span>
  );
}

const TONE_CLASS: Record<string, string> = {
  warn: styles.tWarn, neg: styles.tNeg, muted: styles.tMuted, success: styles.tSuccess,
};

function Stat({ label, value, tone, delta }: {
  label: string;
  value: string;
  tone?: 'warn' | 'neg' | 'muted' | 'success';
  delta?: PortfolioDelta;
}) {
  return (
    <div className={styles.stat}>
      <div className={styles.statLabel}>{label}</div>
      <div className={clsx(styles.statValue, tone && TONE_CLASS[tone])}>
        {value}
        {delta && <DeltaChip delta={delta} small />}
      </div>
    </div>
  );
}
