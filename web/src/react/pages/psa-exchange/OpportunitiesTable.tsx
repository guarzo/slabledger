import { useState } from 'react';
import { clsx } from 'clsx';
import { GradeBadge } from '../../ui';
import type { PsaExchangeOpportunity } from '../../../types/psaExchange';
import SortableHeader from './SortableHeader';
import SignalCell from './SignalCell';
import {
  daysToSell,
  formatDollar,
  type OpportunityGroup,
  type SortDir,
  type SortKey,
} from './utils';

interface OpportunitiesTableProps {
  rows: PsaExchangeOpportunity[];
  groups: OpportunityGroup[] | null;
  sortKey: SortKey;
  sortDir: SortDir;
  onSort: (key: SortKey) => void;
  topDecileScore: number;
}

const COLUMN_COUNT = 6;

function deltaLabel(target: number, value: number): string {
  if (value <= 0) return '';
  const delta = target - value;
  const pct = (delta / value) * 100;
  const sign = delta >= 0 ? '+' : '−';
  const absDelta = Math.abs(delta);
  return `${sign}${formatDollar(absDelta)} (${sign}${Math.abs(pct).toFixed(0)}%)`;
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
          {/* Six columns shown at every breakpoint. Cert, Comp, Days/sale,
              Vel/mo, Conf, Pop are folded into the Card cell or the Signal
              popover (see SignalCell.tsx). Sortability for those keys is
              dropped here; the underlying applySort() in utils.ts still
              accepts them if invoked externally. */}
          <tr>
            <th scope="col" aria-label="Image" className="w-12 p-2"></th>
            <SortableHeader label="Card" sortKey="description" currentKey={sortKey} currentDir={sortDir} onSort={onSort} />
            <SortableHeader label="PSA Value" sortKey="listPrice" currentKey={sortKey} currentDir={sortDir} onSort={onSort} align="right" />
            <SortableHeader label="Target" sortKey="targetOffer" currentKey={sortKey} currentDir={sortDir} onSort={onSort} align="right" />
            <SortableHeader label="Signal" sortKey="edgeAtOffer" currentKey={sortKey} currentDir={sortDir} onSort={onSort} align="right" />
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
  const delta = deltaLabel(row.targetOffer, row.listPrice);

  return (
    <tr
      className={clsx(
        'border-b border-[var(--surface-2)]/40 hover:bg-[var(--surface-2)]/40 transition-colors',
        zebra && !isMember && 'bg-[var(--surface-1)]/40',
        isMember && 'bg-[var(--surface-1)]/60',
      )}
    >
      <td className={clsx('w-12 p-2', isMember && 'pl-8')}>
        <div className="h-12 w-9 rounded-sm overflow-hidden bg-[var(--surface-2)]/40">
          {row.frontImage && (
            <img src={row.frontImage} alt="" className="h-full w-full object-cover" loading="lazy" />
          )}
        </div>
      </td>
      <td className="p-2 max-w-[36rem]">
        <div className={clsx('flex items-center gap-2 min-w-0', isMember && 'text-xs text-[var(--text-muted)]')}>
          <span className="truncate">{row.description || row.name}</span>
          {grade > 0 && !isMember && <GradeBadge grade={grade} />}
        </div>
        <div className="mt-0.5 flex items-center gap-2 text-[11px] leading-none">
          <span aria-label={`Cert #${row.cert}`} className="font-mono text-[var(--text-muted)] tabular-nums select-text">{row.cert}</span>
          {row.mayTakeAtList && !isMember && (
            <span className="px-1.5 py-0.5 rounded-md bg-[var(--success)]/15 text-[var(--success)]">
              PSA value &lt; target
            </span>
          )}
        </div>
      </td>
      <td className="p-2 text-right tabular-nums">{formatDollar(row.listPrice)}</td>
      <td className="p-2 text-right tabular-nums">
        <div>{formatDollar(row.targetOffer)}</div>
        {delta && (
          <div
            aria-label={`Target vs PSA value: ${delta}`}
            className="text-[10px] text-[var(--text-muted)] leading-none mt-0.5"
          >
            {delta}
          </div>
        )}
      </td>
      <td className="p-2">
        <div className="flex justify-end">
          <SignalCell
            edgeAtOffer={row.edgeAtOffer}
            daysToSellValue={days}
            velocityMonth={row.velocityMonth}
            confidence={row.confidence}
            comp={row.comp}
            population={row.population}
          />
        </div>
      </td>
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
              {members.length} listings · {lowList === highList ? formatDollar(lowList) : `${formatDollar(lowList)}–${formatDollar(highList)}`}
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
