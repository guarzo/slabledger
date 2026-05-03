import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import {
  useCapitalSummary,
  useGlobalInventory,
} from '../../queries/useCampaignQueries';
import { useLiquidationPreview } from '../../queries/useLiquidationQueries';
import { computeInventoryMeta } from '../../pages/campaign-detail/inventory/inventoryCalcs';
import { formatCents } from '../../utils/formatters';

const DEFAULT_DISCOUNT_WITH_COMPS = 2.5;
const DEFAULT_DISCOUNT_NO_COMPS = 10;

type Tone = 'danger' | 'warning' | 'neutral';

interface MoveRow {
  key: string;
  copy: React.ReactNode;
  to: string;
  cta: string;
  tone: Tone;
}

const toneClass: Record<Tone, string> = {
  danger: 'text-[var(--danger)]',
  warning: 'text-[var(--warning)]',
  neutral: 'text-[var(--text)]',
};

export default function NextMovesPanel() {
  const capitalQuery = useCapitalSummary();
  const inventoryQuery = useGlobalInventory();
  const liquidation = useLiquidationPreview(DEFAULT_DISCOUNT_WITH_COMPS, DEFAULT_DISCOUNT_NO_COMPS);

  const capital = capitalQuery.data;
  const inventory = inventoryQuery.data;

  // Aggregate loading/error/timestamps across all three signal sources. A
  // panel that claims "All clear" while one of these is still fetching or
  // errored would mislead the operator.
  const isInitialLoading =
    (liquidation.isLoading && !liquidation.data) ||
    (capitalQuery.isLoading && !capitalQuery.data) ||
    (inventoryQuery.isLoading && !inventoryQuery.data);
  const isError = liquidation.isError || capitalQuery.isError || inventoryQuery.isError;
  const isAnyFetching = liquidation.isFetching || capitalQuery.isFetching || inventoryQuery.isFetching;
  const allTimestamps = [
    liquidation.dataUpdatedAt,
    capitalQuery.dataUpdatedAt,
    inventoryQuery.dataUpdatedAt,
  ].filter((ts): ts is number => typeof ts === 'number' && ts > 0);
  const latestUpdatedAt = allTimestamps.length > 0 ? Math.max(...allTimestamps) : 0;

  const lastReviewedAt = useMemo(() => {
    const ts = latestUpdatedAt || Date.now();
    return new Date(ts).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
  }, [latestUpdatedAt]);

  const refetchAll = () => {
    void liquidation.refetch();
    void capitalQuery.refetch();
    void inventoryQuery.refetch();
  };

  const moves = useMemo<MoveRow[]>(() => {
    const out: MoveRow[] = [];

    const belowCost = liquidation.data?.items?.filter((it) => it.belowCost).length ?? 0;
    if (belowCost > 0) {
      out.push({
        key: 'below-cost',
        copy: <><strong className="font-semibold tabular-nums">{belowCost}</strong> {belowCost === 1 ? 'card' : 'cards'} below cost</>,
        to: '/reprice',
        cta: 'Reprice',
        tone: 'danger',
      });
    }

    const readyToList = inventory ? computeInventoryMeta(inventory).tabCounts.ready_to_list : 0;
    if (readyToList > 0) {
      out.push({
        key: 'ready-to-list',
        copy: <><strong className="font-semibold tabular-nums">{readyToList}</strong> ready to push to DH</>,
        to: '/sell-sheet',
        cta: 'Sell Sheet',
        tone: 'neutral',
      });
    }

    if (capital && capital.nextInvoiceAmountCents > 0) {
      const due = capital.nextInvoiceDueDate ?? '—';
      const pending = capital.nextInvoicePendingReceiptCents;
      const overdue = capital.daysUntilInvoiceDue < 0;
      out.push({
        key: 'invoice',
        copy: (
          <>
            Invoice <strong className="font-semibold tabular-nums">{formatCents(capital.nextInvoiceAmountCents)}</strong>
            {' '}due {due}
            {pending > 0 && <> · <span className="tabular-nums">{formatCents(pending)}</span> pending</>}
          </>
        ),
        to: '/invoices',
        cta: 'Invoices',
        tone: overdue ? 'danger' : 'warning',
      });
    }

    return out.slice(0, 3);
  }, [capital, inventory, liquidation.data]);

  return (
    <section
      aria-labelledby="next-moves-heading"
      className="border-y border-[rgba(255,255,255,0.08)] py-4 mb-6"
    >
      <div className="flex items-baseline justify-between mb-2">
        <h2
          id="next-moves-heading"
          className="text-2xs uppercase tracking-wider font-semibold text-[var(--text-muted)]"
        >
          Next Moves
        </h2>
        <Link
          to="/insights"
          className="text-xs text-[var(--text-muted)] hover:text-[var(--text)] transition-colors focus-ring rounded-sm"
        >
          Insights ›
        </Link>
      </div>

      {isInitialLoading ? (
        <ul className="divide-y divide-[rgba(255,255,255,0.03)]">
          {[0, 1, 2].map((i) => (
            <li key={i} className="flex items-center justify-between py-2 text-sm">
              <span className="text-[var(--text-subtle)]">…</span>
            </li>
          ))}
        </ul>
      ) : isError ? (
        <div className="flex items-baseline gap-2 py-1 text-sm">
          <span className="text-[var(--danger)]" aria-hidden="true">▸</span>
          <span className="text-[var(--text)]">Couldn't load all signals</span>
          <button
            type="button"
            onClick={refetchAll}
            disabled={isAnyFetching}
            className="ml-auto text-xs text-[var(--text-muted)] hover:text-[var(--text)] transition-colors focus-ring rounded-sm disabled:opacity-50"
          >
            {isAnyFetching ? 'Retrying…' : 'Retry ›'}
          </button>
        </div>
      ) : moves.length === 0 ? (
        <div className="flex items-baseline gap-2 py-1 text-sm">
          <span className="text-[var(--success)]" aria-hidden="true">●</span>
          <span className="text-[var(--text)]">All clear</span>
          <span className="text-[var(--text-subtle)]">· last reviewed {lastReviewedAt}</span>
          <button
            type="button"
            onClick={refetchAll}
            disabled={isAnyFetching}
            className="ml-auto text-xs text-[var(--text-muted)] hover:text-[var(--text)] transition-colors focus-ring rounded-sm disabled:opacity-50"
          >
            {isAnyFetching ? 'Refreshing…' : 'Refresh ›'}
          </button>
        </div>
      ) : (
        <ul className="divide-y divide-[rgba(255,255,255,0.03)]">
          {moves.map((move) => (
            <li key={move.key}>
              <Link
                to={move.to}
                className="group flex items-center justify-between gap-4 py-2 text-sm transition-colors hover:bg-[rgba(255,255,255,0.02)] focus-ring rounded-sm"
              >
                <span className={`flex items-baseline gap-2 ${toneClass[move.tone]}`}>
                  <span aria-hidden="true">▸</span>
                  <span className="text-[var(--text)]">{move.copy}</span>
                </span>
                <span className="text-xs text-[var(--text-muted)] group-hover:text-[var(--text)] transition-colors whitespace-nowrap">
                  {move.cta} ›
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
