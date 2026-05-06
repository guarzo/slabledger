import { useState } from 'react';
import { clsx } from 'clsx';
import { HoverCard } from 'radix-ui';
import { GradeBadge } from '../../ui';
import type { PsaExchangeOpportunity, PsaExchangePolicy } from '../../../types/psaExchange';
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
  policy: PsaExchangePolicy;
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
  policy,
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
            <th
              scope="col"
              className="p-2 text-right text-[11px] uppercase tracking-wide font-semibold"
              aria-sort={sortKey === 'targetOffer' ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'}
            >
              <span className="inline-flex items-center gap-1 justify-end">
                <button
                  type="button"
                  onClick={() => onSort('targetOffer')}
                  aria-label={`Sort by Target${sortKey === 'targetOffer' ? `, currently ${sortDir === 'asc' ? 'ascending' : 'descending'}` : ''}`}
                  className={clsx(
                    'inline-flex items-center gap-1 flex-row-reverse select-none cursor-pointer bg-transparent border-none p-0 font-inherit',
                    sortKey === 'targetOffer' ? 'text-[var(--brand-400)]' : 'text-[var(--text-muted)] hover:text-[var(--brand-400)]',
                  )}
                >
                  Target
                  <span className="text-[8px] w-2" aria-hidden="true">
                    {sortKey === 'targetOffer' ? (sortDir === 'asc' ? '▲' : '▼') : ''}
                  </span>
                </button>
                <TargetFormulaInfo policy={policy} />
              </span>
            </th>
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

function formatOfferPct(pct: number): string {
  // Preserve one decimal so a 67.5% lever doesn't display as 68%, but trim
  // trailing zeros so the common 65% / 75% case stays clean.
  const v = pct * 100;
  return Number.isInteger(v) ? `${v}%` : `${v.toFixed(1)}%`;
}

function TargetFormulaInfo({ policy }: { policy: PsaExchangePolicy }) {
  const hi = formatOfferPct(policy.highLiquidityOfferPct);
  const def = formatOfferPct(policy.defaultOfferPct);
  return (
    <HoverCard.Root openDelay={100} closeDelay={150}>
      <HoverCard.Trigger asChild>
        <button
          type="button"
          aria-label="How target is computed"
          className="inline-flex items-center justify-center w-4 h-4 rounded-full text-[10px] font-semibold text-[var(--text-muted)] border border-[var(--surface-2)] hover:text-[var(--brand-400)] hover:border-[var(--brand-400)] focus:outline-none focus:ring-1 focus:ring-[var(--brand-500)]/40"
        >
          ?
        </button>
      </HoverCard.Trigger>
      <HoverCard.Portal>
        <HoverCard.Content
          align="end"
          sideOffset={6}
          className="z-50 w-72 p-3 rounded-md bg-[var(--surface-1)] border border-[var(--surface-2)] shadow-lg text-xs leading-relaxed space-y-2 text-[var(--text)]"
        >
          <div className="font-medium">Target offer = comp × tier %</div>
          <div className="text-[var(--text-muted)]">
            <span className="text-[var(--text)]">High liquidity:</span> {hi} of comp
            <span className="text-[var(--text-muted)]"> — when velocity ≥ {policy.highLiquidityVelocity}/mo and confidence ≥ {policy.highLiquidityConfidence}.</span>
          </div>
          <div className="text-[var(--text-muted)]">
            <span className="text-[var(--text)]">Default:</span> {def} of comp <span className="text-[var(--text-muted)]">— otherwise.</span>
          </div>
          <div className="h-px bg-[var(--surface-2)]" />
          <div className="text-[var(--text-muted)]">
            Listings are pre-filtered to confidence ≥ {policy.minConfidence} and quarter-velocity ≥ {policy.minQuarterVelocity}.
            Tunable via PSA_EXCHANGE_* env vars.
          </div>
          <HoverCard.Arrow className="fill-[var(--surface-2)]" />
        </HoverCard.Content>
      </HoverCard.Portal>
    </HoverCard.Root>
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
              PSA value ≤ target
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
            tier={row.tier}
            maxOfferPct={row.maxOfferPct}
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
