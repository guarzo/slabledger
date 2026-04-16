import { useState } from 'react';
import { Button } from '../../ui';
import CardIntakeSection from './CardIntakeSection';
import { LegacyCard } from './CardIntakeSection';
import SalesImportSection from './SalesImportSection';

type ExpandedCard = 'sales' | null;

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

      {expandedCard === 'sales' && <SalesImportSection />}
    </>
  );
}
