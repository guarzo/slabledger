import { useCallback, useState } from 'react';
import { SALE_COST_VISIBLE_KEY, SALE_DEFAULT_DISCOUNT_KEY } from './sale-types';

interface SaleToolbarProps {
  discountPct: number;
  onDiscountChange: (pct: number) => void;
  costVisible: boolean;
  onCostVisibleChange: (visible: boolean) => void;
}

export function SaleToolbar({ discountPct, onDiscountChange, costVisible, onCostVisibleChange }: SaleToolbarProps) {
  const [discountInput, setDiscountInput] = useState(String(discountPct));

  const handleDiscountBlur = useCallback(() => {
    const val = Math.max(0, Math.min(100, Number(discountInput) || 0));
    setDiscountInput(String(val));
    onDiscountChange(val);
    localStorage.setItem(SALE_DEFAULT_DISCOUNT_KEY, String(val));
  }, [discountInput, onDiscountChange]);

  const handleCostToggle = useCallback(() => {
    const next = !costVisible;
    onCostVisibleChange(next);
    localStorage.setItem(SALE_COST_VISIBLE_KEY, String(next));
  }, [costVisible, onCostVisibleChange]);

  return (
    <div className="flex items-center gap-3 rounded-md border border-zinc-800 bg-zinc-900/50 px-3 py-2">
      <span className="text-xs text-zinc-400">Default discount:</span>
      <div className="flex items-center gap-1">
        <input
          type="number"
          min={0}
          max={100}
          value={discountInput}
          onChange={e => setDiscountInput(e.target.value)}
          onBlur={handleDiscountBlur}
          onKeyDown={e => e.key === 'Enter' && (e.target as HTMLInputElement).blur()}
          className="w-14 rounded border border-indigo-600 bg-zinc-900 px-2 py-1 text-center text-sm text-white"
        />
        <span className="text-xs text-zinc-500">%</span>
      </div>
      <span className="text-xs text-zinc-600">of Comp Value</span>
      <span className="ml-auto text-xs text-zinc-500">Channel: Local</span>
      <button
        onClick={handleCostToggle}
        className="ml-2 text-zinc-500 hover:text-zinc-300 transition-colors"
        title={costVisible ? 'Hide cost & profit' : 'Show cost & profit'}
      >
        {costVisible ? (
          <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
            <circle cx="12" cy="12" r="3" />
          </svg>
        ) : (
          <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94" />
            <path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19" />
            <line x1="1" y1="1" x2="23" y2="23" />
          </svg>
        )}
      </button>
    </div>
  );
}
