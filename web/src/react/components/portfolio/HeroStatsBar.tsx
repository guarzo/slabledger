import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import { clsx } from 'clsx';
import type { PortfolioHealth, PortfolioDelta, CapitalSummary } from '../../../types/campaigns';
import { formatCents, formatPct, formatWeeksToCover } from '../../utils/formatters';
import { StatusPill } from '../../ui';
import TrendArrow from '../../ui/TrendArrow';
import styles from './HeroStatsBar.module.css';

const trendToArrow = { improving: 'up', declining: 'down', stable: 'stable' } as const;

interface HeroStatsBarProps {
  health?: PortfolioHealth;
  capital?: CapitalSummary;
  needsAttentionCount?: number;
  pendingListingsCount?: number;
  hideInvoiceChip?: boolean;
  /** ms-epoch of when the underlying data was last fetched. Drives the
      "as of HH:MM" freshness line under the ROI headline. */
  asOfMs?: number;
}

export default function HeroStatsBar({
  health,
  capital,
  needsAttentionCount = 0,
  pendingListingsCount = 0,
  hideInvoiceChip = false,
  asOfMs,
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
      <section className={styles.hero} aria-label="Portfolio summary">
        <div className={styles.roiBlock}>
          <div className={styles.roiLabel}>Realized ROI</div>
          <div className={styles.roiRow}>
            <span className={clsx(styles.roiValue, styles.tMuted)}>—</span>
          </div>
          <div className={styles.freshness}>
            No deployed capital yet — start a campaign to populate this dashboard.
          </div>
        </div>
        <div className={styles.emptyActions}>
          <Link to="/campaigns" className={styles.emptyCta}>
            Create a campaign →
          </Link>
          <Link to="/inventory" className={styles.emptyCtaMuted}>
            Import PSA purchases
          </Link>
        </div>
      </section>
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
      {/* Headline row: the ROI is the dashboard's primary number; alerts
          float top-right so they're visible without competing with the
          serif headline below. */}
      <div className={styles.headlineRow}>
        <div className={styles.roiBlock}>
          <div className={styles.roiLabel}>Realized ROI</div>
          <div className={styles.roiRow}>
            <span className={styles.roiValue}>
              {roi >= 0 ? '+' : ''}{formatPct(roi)}
            </span>
            {health.realizedROIDelta && <DeltaChip delta={health.realizedROIDelta} />}
          </div>
          <FreshnessLine asOfMs={asOfMs} delta={health.realizedROIDelta} />
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
      </div>

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
              labelTitle="Weeks needed to fully recover the outstanding capital at the current 30-day recovery pace."
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
                <abbr
                  className={styles.statLabel}
                  title="Total revenue recovered (sales) in the trailing 30 days."
                  style={{ textDecoration: 'none', cursor: 'help', display: 'block' }}
                >
                  30d Recovery
                </abbr>
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

    </section>
  );
}

/** "as of HH:MM · {delta label}" line under the ROI headline. Driven by
    the React Query dataUpdatedAt timestamp from the page so the operator
    can see at a glance that the snapshot is current; the delta label
    (e.g. "since last login") rides along when the API supplies one.
    Renders nothing when no real timestamp is available — falling back to
    the current render time would fabricate a freshness signal in
    precisely the cases where the operator most needs it to be honest. */
function FreshnessLine({ asOfMs, delta }: { asOfMs?: number; delta?: PortfolioDelta }) {
  const stamp = useMemo(() => {
    if (!asOfMs || asOfMs <= 0) return null;
    return new Date(asOfMs).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
  }, [asOfMs]);
  if (!stamp) return null;
  return (
    <div className={styles.freshness}>
      as of {stamp}
      {delta?.label && (
        <span className={styles.freshDelta}>{delta.label}</span>
      )}
    </div>
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

function Stat({ label, labelTitle, caption, value, tone, delta }: {
  label: string;
  labelTitle?: string;
  caption?: string;
  value: string;
  tone?: 'warn' | 'neg' | 'muted' | 'success' | 'waiting' | 'atRisk' | 'problem';
  delta?: PortfolioDelta;
}) {
  return (
    <div className={styles.stat}>
      {labelTitle ? (
        <abbr
          className={styles.statLabel}
          title={labelTitle}
          style={{ textDecoration: 'none', cursor: 'help', display: 'block' }}
        >
          {label}
        </abbr>
      ) : (
        <div className={styles.statLabel}>{label}</div>
      )}
      {caption && <div className={styles.statCaption}>{caption}</div>}
      <div className={clsx(styles.statValue, tone && TONE_CLASS[tone])}>
        {value}
        {delta && <DeltaChip delta={delta} small />}
      </div>
    </div>
  );
}
