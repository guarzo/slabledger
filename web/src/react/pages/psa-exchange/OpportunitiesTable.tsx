import { useState } from 'react';
import { clsx } from 'clsx';
import { GradeBadge } from '../../ui';
import type { PsaExchangeOpportunity } from '../../../types/psaExchange';
import SortableHeader from './SortableHeader';
import {
  confidenceColorClass,
  daysBucketClass,
  daysToSell,
  edgeBucketClass,
  velocityBucketClass,
  type OpportunityGroup,
  type SortDir,
  type SortKey,
} from './utils';

interface OpportunitiesTableProps {
  // When `groups` is non-null, renders grouped view; otherwise renders flat rows.
  rows: PsaExchangeOpportunity[];
  groups: OpportunityGroup[] | null;
  sortKey: SortKey;
  sortDir: SortDir;
  onSort: (key: SortKey) => void;
  topDecileScore: number;
}

const COLUMN_COUNT = 13;

const dollar = (n: number) =>
  n.toLocaleString('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 0 });
const pct = (n: number) => `${(n * 100).toFixed(1)}%`;

function formatDays(d: number): string {
  if (!Number.isFinite(d)) return '—';
  if (d < 1) return '<1d';
  if (d < 10) return `${d.toFixed(1)}d`;
  return `${Math.round(d)}d`;
}

export default function OpportunitiesTable({
  rows,
  groups,
  sortKey,
  sortDir,
  onSort,
  topDecileScore,
}: OpportunitiesTableProps) {
  if (rows.length === 0) {
    return (
      <div className="p-6 text-center text-sm text-[var(--text-muted)]">
        No PSA-Exchange opportunities match the current filters.
      </div>
    );
  }

  return (
    <div className="overflow-x-auto rounded-md border border-[var(--surface-2)]">
      <table className="w-full text-sm border-collapse">
        <thead className="sticky top-0 z-10 bg-[var(--surface-1)] border-b border-[var(--surface-2)]">
          <tr>
            <th scope="col" aria-label="Image" className="w-12 p-2"></th>
            <SortableHeader label="Cert" sortKey="cert" currentKey={sortKey} currentDir={sortDir} onSort={onSort} />
            <SortableHeader label="Description" sortKey="description" currentKey={sortKey} currentDir={sortDir} onSort={onSort} />
            <SortableHeader label="Grade" sortKey="grade" currentKey={sortKey} currentDir={sortDir} onSort={onSort} />
            <SortableHeader label="PSA Value" sortKey="listPrice" currentKey={sortKey} currentDir={sortDir} onSort={onSort} align="right" />
            <SortableHeader label="Target" sortKey="targetOffer" currentKey={sortKey} currentDir={sortDir} onSort={onSort} align="right" />
            <SortableHeader label="Comp" sortKey="comp" currentKey={sortKey} currentDir={sortDir} onSort={onSort} align="right" />
            <SortableHeader label="Edge" sortKey="edgeAtOffer" currentKey={sortKey} currentDir={sortDir} onSort={onSort} align="right" />
            <SortableHeader label="Days/sale" sortKey="daysToSell" currentKey={sortKey} currentDir={sortDir} onSort={onSort} align="right" />
            <SortableHeader label="Vel/mo" sortKey="velocityMonth" currentKey={sortKey} currentDir={sortDir} onSort={onSort} align="right" />
            <SortableHeader label="Conf" sortKey="confidence" currentKey={sortKey} currentDir={sortDir} onSort={onSort} align="right" />
            <SortableHeader label="Pop" sortKey="population" currentKey={sortKey} currentDir={sortDir} onSort={onSort} align="right" />
            <SortableHeader label="Score" sortKey="score" currentKey={sortKey} currentDir={sortDir} onSort={onSort} align="right" />
          </tr>
        </thead>
        <tbody>
          {groups
            ? groups.map((g) => (
                <GroupRow key={g.key} group={g} topDecileScore={topDecileScore} />
              ))
            : rows.map((r, idx) => (
                <DataRow key={r.cert} row={r} topDecileScore={topDecileScore} zebra={idx % 2 === 1} />
              ))}
        </tbody>
      </table>
    </div>
  );
}

interface DataRowProps {
  row: PsaExchangeOpportunity;
  topDecileScore: number;
  zebra: boolean;
  isMember?: boolean;
}

function DataRow({ row, topDecileScore, zebra, isMember = false }: DataRowProps) {
  const isTopDecile = row.score >= topDecileScore;
  const days = daysToSell(row);
  const grade = Number(row.grade) || 0;

  return (
    <tr
      className={clsx(
        'border-b border-[var(--surface-2)]/40 hover:bg-[var(--surface-2)]/40 transition-colors',
        zebra && !isMember && 'bg-[var(--surface-1)]/40',
        isMember && 'bg-[var(--surface-1)]/60',
      )}
    >
      <td
        className={clsx(
          'w-12 p-2',
          isMember && 'pl-8',
        )}
      >
        <div className="h-12 w-9 rounded-sm overflow-hidden bg-[var(--surface-2)]/40">
          {row.frontImage && (
            <img src={row.frontImage} alt="" className="h-full w-full object-cover" loading="lazy" />
          )}
        </div>
      </td>
      <td className="p-2 font-mono text-xs tabular-nums text-[var(--text-muted)]">{row.cert}</td>
      <td className="p-2 max-w-[26rem]">
        <div className={clsx('truncate', isMember && 'text-xs text-[var(--text-muted)]')}>
          {row.description || row.name}
        </div>
        {row.mayTakeAtList && !isMember && (
          <span className="inline-block mt-1 text-[10px] px-1.5 py-0.5 rounded-md bg-[var(--success)]/15 text-[var(--success)]">
            PSA value &lt; target
          </span>
        )}
      </td>
      <td className="p-2">{grade > 0 && <GradeBadge grade={grade} />}</td>
      <td className="p-2 text-right tabular-nums">{dollar(row.listPrice)}</td>
      <td className="p-2 text-right tabular-nums">{dollar(row.targetOffer)}</td>
      <td className="p-2 text-right tabular-nums">{dollar(row.comp)}</td>
      <td className={clsx('p-2 text-right tabular-nums', edgeBucketClass(row.edgeAtOffer))}>
        {pct(row.edgeAtOffer)}
      </td>
      <td className={clsx('p-2 text-right tabular-nums', daysBucketClass(days))}>{formatDays(days)}</td>
      <td className={clsx('p-2 text-right tabular-nums', velocityBucketClass(row.velocityMonth))}>
        {row.velocityMonth}
      </td>
      <td className={clsx('p-2 text-right tabular-nums', confidenceColorClass(row.confidence))}>
        {row.confidence}
      </td>
      <td className="p-2 text-right tabular-nums text-[var(--text-muted)]">{row.population || '—'}</td>
      <td
        className={clsx(
          'p-2 text-right tabular-nums',
          isTopDecile ? 'text-[var(--brand-400)] font-semibold' : 'text-[var(--text-muted)]',
        )}
        title={isTopDecile ? 'Top decile score' : undefined}
      >
        {row.score.toFixed(3)}
      </td>
    </tr>
  );
}

function GroupRow({ group, topDecileScore }: { group: OpportunityGroup; topDecileScore: number }) {
  const [expanded, setExpanded] = useState(false);
  const { primary, members } = group;
  const hasOthers = members.length > 1;
  const lowList = Math.min(...members.map((m) => m.listPrice));
  const highList = Math.max(...members.map((m) => m.listPrice));

  return (
    <>
      <DataRow row={primary} topDecileScore={topDecileScore} zebra={false} />
      {hasOthers && (
        <tr className="bg-[var(--surface-1)]/30 border-b border-[var(--surface-2)]/40">
          <td colSpan={COLUMN_COUNT} className="px-2 pb-2">
            <button
              type="button"
              onClick={() => setExpanded((v) => !v)}
              className="text-[11px] text-[var(--text-muted)] hover:text-[var(--brand-400)] inline-flex items-center gap-1"
              aria-expanded={expanded}
            >
              <span aria-hidden="true">{expanded ? '▾' : '▸'}</span>
              {members.length} listings · {lowList === highList ? dollar(lowList) : `${dollar(lowList)}–${dollar(highList)}`}
            </button>
          </td>
        </tr>
      )}
      {expanded &&
        members
          .filter((m) => m.cert !== primary.cert)
          .map((m) => <DataRow key={m.cert} row={m} topDecileScore={topDecileScore} zebra={false} isMember />)}
    </>
  );
}
