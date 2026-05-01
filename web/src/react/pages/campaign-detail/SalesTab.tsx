import type { Sale } from '../../../types/campaigns';
import { formatCents } from '../../utils/formatters';
import { useMediaQuery } from '../../hooks/useMediaQuery';
import { saleChannelLabels } from '../../utils/campaignConstants';
import { EmptyState, CardShell } from '../../ui';

interface SalesTabProps {
  sales: Sale[];
}

function SaleMobileCard({ sale }: { sale: Sale }) {
  return (
    <CardShell variant="data" padding="sm">
      <div className="flex items-center justify-between mb-2">
        <span className="text-sm font-medium text-[var(--text)]">{saleChannelLabels[sale.saleChannel] || sale.saleChannel}</span>
        <span className={`text-sm font-semibold tabular-nums ${sale.netProfitCents >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
          {formatCents(sale.netProfitCents)}
        </span>
      </div>
      <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
        <div><span className="text-[var(--text-muted)]">Price:</span> <span className="text-[var(--text)] tabular-nums">{formatCents(sale.salePriceCents)}</span></div>
        <div><span className="text-[var(--text-muted)]">Fees:</span> <span className="text-[var(--text)] tabular-nums">{formatCents(sale.saleFeeCents)}</span></div>
        <div><span className="text-[var(--text-muted)]">Date:</span> <span className="text-[var(--text)]">{sale.saleDate}</span></div>
        <div><span className="text-[var(--text-muted)]">Days:</span> <span className="text-[var(--text)] tabular-nums">{sale.daysToSell}</span></div>
        {sale.medianCents != null && (
          <div><span className="text-[var(--text-muted)]">Median:</span> <span className="text-[var(--text)] tabular-nums">{formatCents(sale.medianCents)}</span></div>
        )}
        {sale.lastSoldCents != null && sale.lastSoldCents > 0 && (
          <div><span className="text-[var(--text-muted)]">Last sold:</span> <span className="text-[var(--text)] tabular-nums">{formatCents(sale.lastSoldCents)}</span></div>
        )}
      </div>
    </CardShell>
  );
}

function SaleDesktopRow({ sale }: { sale: Sale }) {
  return (
    <tr className="glass-table-row">
      <td className="glass-table-td text-[var(--text)] font-medium">{saleChannelLabels[sale.saleChannel] || sale.saleChannel}</td>
      <td className="glass-table-td text-right text-[var(--text)] tabular-nums">{formatCents(sale.salePriceCents)}</td>
      <td className="glass-table-td text-right text-[var(--text-muted)] tabular-nums">{formatCents(sale.saleFeeCents)}</td>
      <td className={`glass-table-td text-right font-semibold tabular-nums ${sale.netProfitCents >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
        {formatCents(sale.netProfitCents)}
      </td>
      <td className="glass-table-td text-right">
        {sale.medianCents != null ? (
          <span className="text-xs text-[var(--text-muted)] tabular-nums" title={sale.lastSoldCents != null && sale.lastSoldCents > 0 ? `Last sold: ${formatCents(sale.lastSoldCents)}` : 'Last sold: N/A'}>
            {formatCents(sale.medianCents)}
          </span>
        ) : (
          <span className="text-xs text-[var(--text-muted)]">-</span>
        )}
      </td>
      <td className="glass-table-td text-center text-[var(--text-muted)] tabular-nums">{sale.daysToSell}</td>
      <td className="glass-table-td text-[var(--text-muted)]">{sale.saleDate}</td>
    </tr>
  );
}

export default function SalesTab({ sales }: SalesTabProps) {
  const isMobile = useMediaQuery('(max-width: 768px)');

  return (
    <div id="tabpanel-sales" role="tabpanel" aria-labelledby="sales">
      {sales.length === 0 ? (
        <EmptyState
          icon="💰"
          title="No sales recorded"
          description="Record sales from the Inventory page."
        />
      ) : isMobile ? (
        <div className="space-y-3">
          {sales.map(s => <SaleMobileCard key={s.id} sale={s} />)}
        </div>
      ) : (
        <div className="glass-table">
          <table className="w-full text-sm">
            <thead>
              <tr className="glass-table-header">
                <th scope="col" className="glass-table-th text-left">Channel</th>
                <th scope="col" className="glass-table-th text-right">Sale Price</th>
                <th scope="col" className="glass-table-th text-right">Fees</th>
                <th scope="col" className="glass-table-th text-right">Net Profit</th>
                <th scope="col" className="glass-table-th text-right">Mkt @ Sale</th>
                <th scope="col" className="glass-table-th text-center">Days to Sell</th>
                <th scope="col" className="glass-table-th text-left">Date</th>
              </tr>
            </thead>
            <tbody>
              {sales.map(s => <SaleDesktopRow key={s.id} sale={s} />)}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
