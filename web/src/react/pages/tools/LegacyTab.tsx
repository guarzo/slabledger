import { useState } from 'react';
import { Button, SectionErrorBoundary } from '../../ui';
import ShopifySyncPage from '../ShopifySyncPage';
import CardIntakeSection from './CardIntakeSection';
import { LegacyCard } from './CardIntakeSection';
import EbayDescriptionSection from './EbayDescriptionSection';
import SalesImportSection from './SalesImportSection';

type ExpandedCard = 'ebay' | 'sales' | 'priceSync' | null;

function SyncIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
      <polyline points="23 4 23 10 17 10" />
      <polyline points="1 20 1 14 7 14" />
      <path d="M3.51 9a9 9 0 0114.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0020.49 15" />
    </svg>
  );
}

function TagIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
      <path d="M20.59 13.41l-7.17 7.17a2 2 0 01-2.83 0L2 12V2h10l8.59 8.59a2 2 0 010 2.82z" />
      <line x1="7" y1="7" x2="7.01" y2="7" />
    </svg>
  );
}

function ReceiptIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
      <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z" />
      <polyline points="14 2 14 8 20 8" />
      <line x1="16" y1="13" x2="8" y2="13" />
      <line x1="16" y1="17" x2="8" y2="17" />
      <polyline points="10 9 9 9 8 9" />
    </svg>
  );
}

export default function LegacyTab() {
  const [expandedCard, setExpandedCard] = useState<ExpandedCard>(null);

  const toggle = (card: ExpandedCard) => {
    setExpandedCard((prev) => (prev === card ? null : card));
  };

  return (
    <>
      {/* Section header */}
      <div className="mb-4">
        <h2 className="text-base font-semibold text-[var(--text)]">Legacy Tools</h2>
        <p className="text-xs text-[var(--text-muted)] mt-0.5">
          Transitional tools — will be removed after full DH migration
        </p>
      </div>

      {/* Card intake grid */}
      <div className="mb-6">
        <CardIntakeSection />
      </div>

      {/* Expandable tool cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-6">
        <LegacyCard
          icon={<SyncIcon />}
          title="Price Sync"
          description="Sync prices with Shopify store"
        >
          <Button
            size="sm"
            variant={expandedCard === 'priceSync' ? 'primary' : 'secondary'}
            fullWidth
            onClick={() => toggle('priceSync')}
            aria-expanded={expandedCard === 'priceSync'}
            aria-controls="priceSync-panel"
          >
            {expandedCard === 'priceSync' ? 'Collapse' : 'Open Price Sync'}
          </Button>
        </LegacyCard>

        <LegacyCard
          icon={<TagIcon />}
          title="eBay Export"
          description="Generate eBay bulk listing CSV"
        >
          <Button
            size="sm"
            variant={expandedCard === 'ebay' ? 'primary' : 'secondary'}
            fullWidth
            onClick={() => toggle('ebay')}
            aria-expanded={expandedCard === 'ebay'}
            aria-controls="ebay-panel"
          >
            {expandedCard === 'ebay' ? 'Collapse' : 'Open eBay Export'}
          </Button>
        </LegacyCard>

        <LegacyCard
          icon={<ReceiptIcon />}
          title="Import Sales"
          description="Import sales from order CSVs"
        >
          <Button
            size="sm"
            variant={expandedCard === 'sales' ? 'primary' : 'secondary'}
            fullWidth
            onClick={() => toggle('sales')}
            aria-expanded={expandedCard === 'sales'}
            aria-controls="sales-panel"
          >
            {expandedCard === 'sales' ? 'Collapse' : 'Open Import Sales'}
          </Button>
        </LegacyCard>
      </div>

      {/* Expanded content — full-width below grid */}
      {expandedCard === 'priceSync' && (
        <div id="priceSync-panel" role="region" aria-label="Price Sync">
          <SectionErrorBoundary sectionName="Price Sync">
            <ShopifySyncPage embedded />
          </SectionErrorBoundary>
        </div>
      )}

      {expandedCard === 'ebay' && <EbayDescriptionSection />}

      {expandedCard === 'sales' && <SalesImportSection />}
    </>
  );
}
