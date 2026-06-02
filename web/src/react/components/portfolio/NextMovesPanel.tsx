import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import {
  useCapitalSummary,
  useGlobalInventory,
} from '../../queries/useCampaignQueries';
import { computeInventoryMeta } from '../../pages/campaign-detail/inventory/inventoryCalcs';
import { formatCents } from '../../utils/formatters';

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

  const capital = capitalQuery.data;
  const inventory = inventoryQuery.data;

  // Aggregate loading/error/timestamps across all signal sources. A
  // panel that claims "All clear" while one of these is still fetching or
  // errored would mislead the operator.
  const isInitialLoading =
    (capitalQuery.isLoading && !capitalQuery.data) ||
    (inventoryQuery.isLoading && !inventoryQuery.data);
  const isError = capitalQuery.isError || inventoryQuery.isError;
  const isAnyFetching = capitalQuery.isFetching || inventoryQuery.isFetching;
  const allTimestamps = [
    capitalQuery.dataUpdatedAt,
    inventoryQuery.dataUpdatedAt,
  ].filter((ts): ts is number => typeof ts === 'number' && ts > 0);
  const latestUpdatedAt = allTimestamps.length > 0 ? Math.max(...allTimestamps) : 0;

  const lastReviewedAt = useMemo(() => {
    const ts = latestUpdatedAt || Date.now();
    return new Date(ts).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
  }, [latestUpdatedAt]);

  const refetchAll = () => {
    void capitalQuery.refetch();
    void inventoryQuery.refetch();
  };

  const moves = useMemo<MoveRow[]>(() => {
    const out: MoveRow[] = [];

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
  }, [capital, inventory]);

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
        <ol className="divide-y divide-[rgba(255,255,255,0.03)]">
          {moves.map((move, idx) => (
            <li key={move.key} data-topmost={idx === 0 ? 'true' : undefined} className="next-move-row">
              <Link
                to={move.to}
                className="group grid grid-cols-[2.25rem_1fr_auto] items-baseline gap-3 py-2.5 text-sm transition-colors hover:bg-[rgba(255,255,255,0.02)] focus-ring rounded-sm"
              >
                {/* Captain's-log counter — mono so 01/02/03 align tightly
                    with the action chip on the right. The tone-coloured
                    triangle preserves the prior severity signal. */}
                <span className="flex items-center gap-1.5 pl-0.5 text-[10px] text-[var(--text-subtle)] tabular-nums tracking-[0.08em]">
                  <span className={toneClass[move.tone]} aria-hidden="true">▸</span>
                  <span>{String(idx + 1).padStart(2, '0')}</span>
                </span>
                <span className="text-[var(--text)] min-w-0 truncate">{move.copy}</span>
                <span className="text-xs text-[var(--text-muted)] group-hover:text-[var(--text)] transition-colors whitespace-nowrap">
                  {move.cta} ›
                </span>
              </Link>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}
