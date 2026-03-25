import { useState } from 'react';
import type { Purchase, Sale } from '../../../types/campaigns';
import PurchasesTab from './PurchasesTab';
import SalesTab from './SalesTab';

type SubView = 'purchases' | 'sales';

interface TransactionsTabProps {
  campaignId: string;
  campaignName: string;
  purchases: Purchase[];
  sales: Sale[];
  soldPurchaseIds: Set<string>;
}

export default function TransactionsTab({ campaignId, campaignName, purchases, sales, soldPurchaseIds }: TransactionsTabProps) {
  const [view, setView] = useState<SubView>('purchases');

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
          campaignName={campaignName}
          purchases={purchases}
          soldPurchaseIds={soldPurchaseIds}
        />
      ) : (
        <SalesTab
          sales={sales}
        />
      )}
    </div>
  );
}
