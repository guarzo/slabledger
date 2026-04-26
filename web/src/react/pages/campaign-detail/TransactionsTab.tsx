import { useState } from 'react';
import type { Purchase, Sale } from '../../../types/campaigns';
import PurchasesTab from './PurchasesTab';
import SalesTab from './SalesTab';
import { EmptyState } from '../../ui';

type SubView = 'purchases' | 'sales';

interface TransactionsTabProps {
  campaignId: string;
  purchases: Purchase[];
  sales: Sale[];
  soldPurchaseIds: Set<string>;
}

export default function TransactionsTab({ campaignId, purchases, sales, soldPurchaseIds }: TransactionsTabProps) {
  const [view, setView] = useState<SubView>('purchases');

  if (purchases.length === 0 && sales.length === 0) {
    return (
      <EmptyState
        icon="🛒"
        title="No transactions yet"
        description="Record your first purchase or sale to see it here."
      />
    );
  }

  // When the campaign has purchases but no sales, show "no sales yet" inside
  // the Sales sub-view with a last-action breadcrumb.
  // (The Purchases sub-view will render normally because purchases.length > 0.)
  const showNoSalesEmpty = view === 'sales' && sales.length === 0;
  const lastPurchase = showNoSalesEmpty && purchases.length > 0
    ? purchases.reduce((latest, p) => (p.purchaseDate > latest.purchaseDate ? p : latest))
    : null;
  const lastActionText = lastPurchase
    ? `Last purchase ${new Date(lastPurchase.purchaseDate).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}`
    : undefined;

  return (
    <div>
      <div className="flex gap-2 mb-4" role="tablist">
        <button
          type="button"
          id="purchases-tab"
          role="tab"
          aria-selected={view === 'purchases'}
          aria-controls="tabpanel-purchases"
          onClick={() => setView('purchases')}
          className={`px-4 py-2 text-sm font-medium rounded-lg transition-colors ${
            view === 'purchases'
              ? 'bg-[var(--brand-500)] text-white'
              : 'bg-[var(--surface-2)] text-[var(--text-muted)] hover:text-[var(--text)]'
          }`}
        >
          Purchases ({purchases.length})
        </button>
        <button
          type="button"
          id="sales-tab"
          role="tab"
          aria-selected={view === 'sales'}
          aria-controls="tabpanel-sales"
          onClick={() => setView('sales')}
          className={`px-4 py-2 text-sm font-medium rounded-lg transition-colors ${
            view === 'sales'
              ? 'bg-[var(--brand-500)] text-white'
              : 'bg-[var(--surface-2)] text-[var(--text-muted)] hover:text-[var(--text)]'
          }`}
        >
          Sales ({sales.length})
        </button>
      </div>

      {view === 'purchases' ? (
        <PurchasesTab
          campaignId={campaignId}
          purchases={purchases}
          soldPurchaseIds={soldPurchaseIds}
        />
      ) : showNoSalesEmpty ? (
        <div id="tabpanel-sales" role="tabpanel" aria-labelledby="sales-tab">
          <EmptyState
            icon="🛒"
            title="No sales yet"
            description="Record your first sale to see P&L for this campaign."
            lastAction={lastActionText}
            compact
          />
        </div>
      ) : (
        <SalesTab
          sales={sales}
        />
      )}
    </div>
  );
}
