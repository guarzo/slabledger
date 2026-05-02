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
  needsAttentionCount?: number;
  pendingListingsCount?: number;
  hideInvoiceChip?: boolean;
}

export default function HeroStatsBar({
  health,
  capital,
  needsAttentionCount = 0,
  pendingListingsCount = 0,
  hideInvoiceChip = false,
}: HeroStatsBarProps) {
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

  const unpaidInvoiceCount = capital?.unpaidInvoiceCount ?? 0;
  const showAlerts =
    (unpaidInvoiceCount > 0 && !hideInvoiceChip) ||
    needsAttentionCount > 0 ||
    pendingListingsCount > 0;

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

      <div className={styles.divider} aria-hidden />

      <div className={styles.clusterGroup}>
        {/* Capital row */}
        <div className={styles.subCluster}>
          <Stat
            label="Deployed"
            caption="all-time"
            value={formatCents(health.totalDeployedCents ?? 0)}
          />
          <Stat
            label="Recovered"
            caption="all-time"
            value={formatCents(health.totalRecoveredCents ?? 0)}
            delta={health.totalRecoveredDelta}
          />
          <Stat
            label="At Risk"
            caption="exposed capital"
            value={formatCents(health.totalAtRiskCents ?? 0)}
            tone={(health.totalAtRiskCents ?? 0) > 0 ? 'atRisk' : undefined}
          />
          {capital && (
            <Stat
              label="Outstanding"
              caption="awaiting recovery"
              value={formatCents(capital.outstandingCents)}
              tone={capital.alertLevel === 'critical' ? 'problem'
                : capital.alertLevel === 'warning' ? 'atRisk'
                : 'waiting'}
            />
          )}
        </div>

        {/* Velocity row */}
        {capital && (
          <div className={styles.subCluster}>
            <Stat
              label="Wks to Cover"
              caption="at current pace"
              value={capital.outstandingCents === 0 && capital.recoveryRate30dCents > 0
                ? '0'
                : formatWeeksToCover(capital.weeksToCover, capital.recoveryRate30dCents > 0)}
              tone={capital.alertLevel === 'critical' ? 'problem'
                : capital.alertLevel === 'warning' ? 'atRisk'
                : capital.recoveryRate30dCents === 0 ? 'muted'
                : 'success'}
            />
            {capital.recoveryRate30dCents > 0 && (
              <div className={styles.stat}>
                <div className={styles.statLabel}>30d Recovery</div>
                <div className={styles.statCaption}>trailing 30d</div>
                <div className={styles.statValue}>
                  {formatCents(capital.recoveryRate30dCents)}
                  <TrendArrow trend={trendToArrow[capital.recoveryTrend]} />
                </div>
              </div>
            )}
          </div>
        )}
      </div>

      {showAlerts && (
        <div className={styles.alerts}>
          {unpaidInvoiceCount > 0 && !hideInvoiceChip && (
            <Link
              to="/invoices"
              className={styles.alertLink}
              aria-label={`${unpaidInvoiceCount} unpaid invoice${unpaidInvoiceCount !== 1 ? 's' : ''}`}
            >
              <StatusPill tone="warning">
                {unpaidInvoiceCount} unpaid invoice{unpaidInvoiceCount !== 1 ? 's' : ''} →
              </StatusPill>
            </Link>
          )}
          {needsAttentionCount > 0 && (
            <Link
              to="/inventory"
              className={styles.alertLink}
              aria-label={`${needsAttentionCount} needs attention`}
            >
              <StatusPill tone="warning">
                {needsAttentionCount} needs attention →
              </StatusPill>
            </Link>
          )}
          {pendingListingsCount > 0 && (
            <Link
              to="/inventory"
              className={styles.alertLink}
              aria-label={`${pendingListingsCount} pending listing${pendingListingsCount !== 1 ? 's' : ''}`}
            >
              <StatusPill tone="warning">
                {pendingListingsCount} pending listing{pendingListingsCount !== 1 ? 's' : ''} →
              </StatusPill>
            </Link>
          )}
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
  warn: styles.tWarn,
  neg: styles.tNeg,
  muted: styles.tMuted,
  success: styles.tSuccess,
  waiting: styles.tWaiting,
  atRisk: styles.tAtRisk,
  problem: styles.tProblem,
};

function Stat({ label, caption, value, tone, delta }: {
  label: string;
  caption?: string;
  value: string;
  tone?: 'warn' | 'neg' | 'muted' | 'success' | 'waiting' | 'atRisk' | 'problem';
  delta?: PortfolioDelta;
}) {
  return (
    <div className={styles.stat}>
      <div className={styles.statLabel}>{label}</div>
      {caption && <div className={styles.statCaption}>{caption}</div>}
      <div className={clsx(styles.statValue, tone && TONE_CLASS[tone])}>
        {value}
        {delta && <DeltaChip delta={delta} small />}
      </div>
    </div>
  );
}
