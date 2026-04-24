import { useState, useDeferredValue } from 'react';
import { useLiquidationPreview, useApplyLiquidation } from '../queries/useLiquidationQueries';
import type { LiquidationPreviewItem, ConfidenceLevel } from '../../types/liquidation';
import StatCard from '../ui/StatCard';

function dollars(cents: number | null | undefined): string {
  return cents != null && cents >= 0 ? `$${(cents / 100).toFixed(2)}` : '—';
}

function confidenceColor(level: ConfidenceLevel): string {
  switch (level) {
    case 'high': return 'text-[var(--success)]';
    case 'medium': return 'text-[var(--warning)]';
    case 'low': return 'text-orange-400';
    default: return 'text-[var(--text-muted)]';
  }
}

export default function LiquidationPage() {
  const [discountWithComps, setDiscountWithComps] = useState(2.5);
  const [discountNoComps, setDiscountNoComps] = useState(10);
  const deferredWithComps = useDeferredValue(discountWithComps);
  const deferredNoComps = useDeferredValue(discountNoComps);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [finalPrices, setFinalPrices] = useState<Record<string, number>>({});
  const [showConfirm, setShowConfirm] = useState(false);

  const { data, isLoading, error } = useLiquidationPreview(deferredWithComps, deferredNoComps);
  const applyMutation = useApplyLiquidation();

  const items: LiquidationPreviewItem[] = data?.items ?? [];

  const toggleSelect = (id: string) => {
    setSelected(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const selectAll = () => setSelected(new Set(items.map(i => i.purchaseId)));
  const deselectAll = () => setSelected(new Set());

  const acceptSuggested = (id: string) => {
    setSelected(prev => new Set(prev).add(id));
    setFinalPrices(prev => {
      const { [id]: _, ...rest } = prev;
      return rest;
    });
  };

  const acceptAllSuggested = () => {
    const priceable = items.filter(i => i.suggestedPriceCents > 0);
    const priceableIds = new Set(priceable.map(i => i.purchaseId));
    setSelected(prev => new Set([...prev, ...priceableIds]));
    setFinalPrices(prev => {
      const next = { ...prev };
      for (const id of priceableIds) delete next[id];
      return next;
    });
  };

  const getFinalPrice = (item: LiquidationPreviewItem): number =>
    finalPrices[item.purchaseId] ?? item.suggestedPriceCents;

  const handleFinalPriceChange = (id: string, val: string) => {
    const parts = val.split('.');
    const d = parseInt(parts[0] || '0', 10);
    const frac = (parts[1] || '0').slice(0, 2).padEnd(2, '0');
    const cents = d * 100 + parseInt(frac, 10);
    if (!isNaN(cents) && cents >= 0) {
      setFinalPrices(prev => ({ ...prev, [id]: cents }));
    }
  };

  const handleApply = () => {
    const applyItems = Array.from(selected)
      .map(id => {
        const item = items.find(i => i.purchaseId === id);
        return item ? { purchaseId: id, newPriceCents: getFinalPrice(item) } : null;
      })
      .filter((x): x is NonNullable<typeof x> => x !== null);
    applyMutation.mutate(applyItems, {
      onSuccess: () => {
        setShowConfirm(false);
        setSelected(new Set());
        setFinalPrices({});
      },
    });
  };

  const summary = data?.summary;

  return (
    <div className="max-w-7xl mx-auto px-4 pb-16">
      <h1 className="text-[22px] font-bold text-[var(--text)] tracking-tight mb-6">Reprice</h1>

      {/* Discount controls */}
      <div className="mb-6 p-4 rounded-xl bg-[var(--surface-1)] border border-[var(--surface-2)]">
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-6">
          <DiscountSlider
            label="With comps"
            value={discountWithComps}
            onChange={setDiscountWithComps}
          />
          <DiscountSlider
            label="Without comps"
            value={discountNoComps}
            onChange={setDiscountNoComps}
          />
        </div>
      </div>

      {isLoading && !data && (
        <div className="text-sm text-[var(--text-muted)] py-8 text-center">Loading inventory…</div>
      )}

      {error && (
        <div className="mb-4 p-3 rounded-lg bg-[var(--danger)]/10 border border-[var(--danger)]/20 text-sm text-[var(--danger)]">
          {error.message}
        </div>
      )}

      {/* Summary */}
      {summary && (
        <div className="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-7 gap-3 mb-6">
          <StatCard label="Total Cards" value={String(summary.totalCards)} />
          <StatCard label="With Comps" value={String(summary.withComps)} color="green" />
          <StatCard label="Without Comps" value={String(summary.withoutComps)} />
          <StatCard label="No Data" value={String(summary.noData)} color={summary.noData > 0 ? 'red' : undefined} />
          <StatCard label="Current Value" value={dollars(summary.totalCurrentValueCents)} />
          <StatCard label="Suggested Value" value={dollars(summary.totalSuggestedValueCents)} />
          <StatCard label="Below Cost" value={String(summary.belowCostCount)} color={summary.belowCostCount > 0 ? 'red' : undefined} />
        </div>
      )}

      {/* Table */}
      {items.length > 0 && (
        <>
          <div className="flex items-center gap-3 mb-2">
            <button type="button" onClick={selectAll} className="text-xs text-[var(--brand-500)] hover:underline">Select All</button>
            <button type="button" onClick={deselectAll} className="text-xs text-[var(--text-muted)] hover:underline">Deselect All</button>
            <button type="button" onClick={acceptAllSuggested} className="text-xs text-[var(--success)] hover:opacity-80">Accept All Suggested</button>
            <span className="text-xs text-[var(--text-muted)]">{selected.size} selected</span>
          </div>

          <div className="glass-table">
            <table className="w-full text-sm">
              <thead>
                <tr className="glass-table-header">
                  <th className="glass-table-th w-8"></th>
                  <th className="glass-table-th text-left">Card</th>
                  <th className="glass-table-th text-center">Grade</th>
                  <th className="glass-table-th text-left">Campaign</th>
                  <th className="glass-table-th text-right">Cost</th>
                  <th className="glass-table-th text-right">CL Value</th>
                  <th className="glass-table-th text-right">Comp Price</th>
                  <th className="glass-table-th text-center"># Comps</th>
                  <th className="glass-table-th text-center">Confidence</th>
                  <th className="glass-table-th text-right">Gap %</th>
                  <th className="glass-table-th text-right">Current</th>
                  <th className="glass-table-th text-right">Suggested</th>
                  <th className="glass-table-th text-right">Final Price</th>
                  <th className="glass-table-th w-8"></th>
                </tr>
              </thead>
              <tbody>
                {items.map(item => {
                  const isSelected = selected.has(item.purchaseId);
                  return (
                    <tr key={item.purchaseId} className={`glass-table-row ${item.belowCost ? 'bg-[var(--danger)]/5' : ''}`}>
                      <td className="glass-table-td">
                        <input
                          type="checkbox"
                          checked={isSelected}
                          onChange={() => toggleSelect(item.purchaseId)}
                          className="rounded"
                        />
                      </td>
                      <td className="glass-table-td text-[var(--text)] font-medium max-w-[200px] truncate">
                        {item.cardName}
                        <div className="text-[10px] text-[var(--text-muted)]">{item.certNumber}</div>
                      </td>
                      <td className="glass-table-td text-center text-[var(--text)]">{item.grade}</td>
                      <td className="glass-table-td text-[var(--text-muted)] text-xs">{item.campaignName}</td>
                      <td className="glass-table-td text-right text-[var(--text)]">{dollars(item.buyCostCents)}</td>
                      <td className="glass-table-td text-right text-[var(--text)]">{dollars(item.clValueCents)}</td>
                      <td className="glass-table-td text-right text-[var(--text)]">{dollars(item.compPriceCents)}</td>
                      <td className="glass-table-td text-center text-[var(--text-muted)]">{item.compCount}</td>
                      <td className={`glass-table-td text-center capitalize ${confidenceColor(item.confidenceLevel)}`}>
                        {item.confidenceLevel}
                      </td>
                      <td className="glass-table-td text-right text-[var(--text-muted)]">
                        {item.gapPct !== 0 ? `${item.gapPct.toFixed(1)}%` : '—'}
                      </td>
                      <td className="glass-table-td text-right text-[var(--text-muted)]">{dollars(item.currentReviewedPriceCents)}</td>
                      <td className="glass-table-td text-right text-[var(--text)]">{dollars(item.suggestedPriceCents)}</td>
                      <td className="glass-table-td text-right">
                        <input
                          type="number"
                          min={0}
                          step={0.01}
                          value={(getFinalPrice(item) / 100).toFixed(2)}
                          onChange={e => handleFinalPriceChange(item.purchaseId, e.target.value)}
                          className="w-24 px-2 py-1 rounded border border-[var(--surface-2)] bg-[var(--surface-1)] text-[var(--text)] text-right text-sm"
                        />
                      </td>
                      <td className="glass-table-td">
                        {item.suggestedPriceCents > 0 && !isSelected && (
                          <button
                            type="button"
                            onClick={() => acceptSuggested(item.purchaseId)}
                            className="text-xs text-[var(--success)] hover:opacity-80 whitespace-nowrap"
                          >
                            Accept
                          </button>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </>
      )}

      {/* Apply bar */}
      {selected.size > 0 && (
        <div className="fixed bottom-0 left-0 right-0 p-4 bg-[var(--surface)] border-t border-[var(--border)] flex items-center justify-between gap-4 z-10">
          <span className="text-sm text-[var(--text)]">{selected.size} card{selected.size !== 1 ? 's' : ''} selected</span>
          <button
            type="button"
            onClick={() => setShowConfirm(true)}
            className="px-5 py-2 bg-[var(--brand-500)] text-white rounded-lg text-sm font-medium hover:bg-[var(--brand-600)] transition-colors"
          >
            Apply Prices
          </button>
        </div>
      )}

      {/* Confirm dialog */}
      {showConfirm && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-20 p-4">
          <div className="bg-[var(--surface)] rounded-xl border border-[var(--border)] p-6 max-w-sm w-full">
            <h2 className="text-lg font-bold text-[var(--text)] mb-2">Apply Repriced Values</h2>
            <p className="text-sm text-[var(--text-muted)] mb-6">
              This will update the reviewed price for {selected.size} card{selected.size !== 1 ? 's' : ''}. Continue?
            </p>
            {applyMutation.error && (
              <p className="text-xs text-[var(--danger)] mb-4">{applyMutation.error.message}</p>
            )}
            {applyMutation.data && (
              <p className="text-xs text-[var(--success)] mb-4">
                Applied {applyMutation.data.applied} price{applyMutation.data.applied !== 1 ? 's' : ''}.
                {applyMutation.data.failed > 0 && ` ${applyMutation.data.failed} failed.`}
              </p>
            )}
            <div className="flex gap-3 justify-end">
              <button
                type="button"
                onClick={() => setShowConfirm(false)}
                className="px-4 py-2 rounded-lg border border-[var(--border)] text-[var(--text-muted)] text-sm hover:bg-[var(--surface-2)] transition-colors"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={handleApply}
                disabled={applyMutation.isPending}
                className="px-4 py-2 bg-[var(--brand-500)] text-white rounded-lg text-sm font-medium hover:bg-[var(--brand-600)] transition-colors disabled:opacity-50"
              >
                {applyMutation.isPending ? 'Applying…' : 'Confirm'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function DiscountSlider({ label, value, onChange }: { label: string; value: number; onChange: (v: number) => void }) {
  const id = `discount-${label.toLowerCase().replace(/\s+/g, '-')}`;
  return (
    <div>
      <div className="flex items-center justify-between mb-2">
        <label htmlFor={id} className="text-xs font-medium text-[var(--text-muted)]">{label}</label>
        <span className="text-sm font-semibold text-[var(--text)] tabular-nums">{value.toFixed(1)}% below CL</span>
      </div>
      <input
        id={id}
        type="range"
        min={0}
        max={25}
        step={0.5}
        value={value}
        onChange={e => onChange(parseFloat(e.target.value))}
        className="w-full h-1.5 rounded-full appearance-none cursor-pointer bg-[var(--surface-2)] accent-[var(--brand-500)]"
      />
      <div className="flex justify-between text-[10px] text-[var(--text-muted)] mt-1">
        <span>0%</span>
        <span>25%</span>
      </div>
    </div>
  );
}
