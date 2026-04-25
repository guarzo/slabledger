import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import { useQueries } from '@tanstack/react-query';
import type { CampaignHealth, CampaignPNL } from '../../../types/campaigns';
import { campaignPNLQueryOptions } from '../../queries/useCampaignQueries';
import { CardShell, MarginBadge, TrendArrow } from '../../ui';
import { formatPct } from '../../utils/formatters';
import styles from './TopPerformersSection.module.css';

interface TopPerformersSectionProps {
  campaigns: CampaignHealth[];
}

interface PerformerRow {
  campaignId: string;
  name: string;
  netProfitCents: number;
  roi: number;
}

const TOP_N = 5;

export default function TopPerformersSection({ campaigns }: TopPerformersSectionProps) {
  const pnlQueries = useQueries({
    queries: campaigns.map(c => campaignPNLQueryOptions(c.campaignId)),
  });

  const top = useMemo<PerformerRow[]>(() => {
    const rows: PerformerRow[] = [];
    pnlQueries.forEach((q, i) => {
      const pnl = q.data as CampaignPNL | undefined;
      const c = campaigns[i];
      if (!pnl || !c) return;
      // Skip campaigns with no realized activity — nothing to rank.
      if (pnl.totalRevenueCents <= 0 && pnl.netProfitCents === 0) return;
      rows.push({
        campaignId: c.campaignId,
        name: c.campaignName,
        netProfitCents: pnl.netProfitCents,
        roi: pnl.roi,
      });
    });
    rows.sort((a, b) => b.netProfitCents - a.netProfitCents);
    return rows.slice(0, TOP_N);
  }, [pnlQueries, campaigns]);

  if (top.length === 0) return null;

  return (
    <CardShell variant="default" padding="sm" radius="sm">
      <header className={styles.header}>
        <h2 className={styles.title}>Top Performers</h2>
        <Link to="/campaigns" className={styles.viewAll}>View all →</Link>
      </header>
      <ul className={styles.list}>
        {top.map((row, i) => (
          <PerformerListRow key={row.campaignId} row={row} rank={i + 1} />
        ))}
      </ul>
    </CardShell>
  );
}

function PerformerListRow({ row, rank }: { row: PerformerRow; rank: number }) {
  const trend = row.roi > 0 ? 'up' : row.roi < 0 ? 'down' : 'stable';
  return (
    <li>
      <Link to={`/campaigns/${row.campaignId}`} className={styles.row}>
        <span className={styles.rank}>#{rank}</span>
        <span className={styles.name} title={row.name}>{row.name}</span>
        <span className={styles.profit}>
          <MarginBadge cents={row.netProfitCents} />
        </span>
        <span className={styles.roi}>
          {row.roi >= 0 ? '+' : ''}{formatPct(row.roi)}
          <TrendArrow trend={trend} />
        </span>
      </Link>
    </li>
  );
}
