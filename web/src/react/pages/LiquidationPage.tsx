import { useState, useCallback } from 'react';
import { useLiquidationPreview, useApplyLiquidation } from '../queries/useLiquidationQueries';
import type { LiquidationPreviewItem, ConfidenceLevel } from '../../types/liquidation';

function dollars(cents: number | null | undefined): string {
  return cents != null && cents >= 0 ? `$${(cents / 100).toFixed(2)}` : '—';
}

function confidenceColor(level: ConfidenceLevel): string {
  switch (level) {
    case 'high': return 'text-green-400';
    case 'medium': return 'text-yellow-400';
    case 'low': return 'text-orange-400';
    default: return 'text-[var(--text-muted)]';
  }
}

export default function LiquidationPage() {
  const [baseDiscountPct, setBaseDiscountPct] = useState(10);
  const [noCompDiscountPct, setNoCompDiscountPct] = useState(20);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [finalPrices, setFinalPrices] = useState<Record<string, number>>({});
  const [showConfirm, setShowConfirm] = useState(false);

  const { data, isLoading, error, fetchPreview } = useLiquidationPreview();
  const applyMutation = useApplyLiquidation();

  const handlePreview = useCallback(() => {
    fetchPreview(baseDiscountPct, noCompDiscountPct);
    setSelected(new Set());
    setFinalPrices({});
  }, [fetchPreview, baseDiscountPct, noCompDiscountPct]);

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
      const item = items.find(i => i.purchaseId === id);
      if (!item) return prev;
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
    const dollars = parseInt(parts[0] || '0', 10);
    const frac = (parts[1] || '0').slice(0, 2).padEnd(2, '0');
    const cents = dollars * 100 + parseInt(frac, 10);
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
        fetchPreview(baseDiscountPct, noCompDiscountPct);
      },
    });
  };

  const summary = data?.summary;

  return (
    <div className="max-w-7xl mx-auto px-4 pb-16">
      <h1 className="text-[22px] font-bold text-[var(--text)] tracking-tight mb-6">Liquidation Pricing</h1>

      {/* Controls */}
      <div className="flex flex-wrap items-end gap-4 mb-6 p-4 rounded-lg bg-[var(--surface)] border border-[var(--border)]">
        <div>
          <label className="block text-xs text-[var(--text-muted)] mb-1">Base Discount %</label>
          <input
            type="number"
            min={0}
            max={100}
            value={baseDiscountPct}
            onChange={e => setBaseDiscountPct(Number(e.target.value))}
            className="w-24 px-3 py-1.5 rounded-md border border-[var(--border)] bg-[var(--surface-2)] text-[var(--text)] text-sm"
          />
        </div>
        <div>
          <label className="block text-xs text-[var(--text-muted)] mb-1">No-Comp Discount %</label>
          <input
            type="number"
            min={0}
            max={100}
            value={noCompDiscountPct}
            onChange={e => setNoCompDiscountPct(Number(e.target.value))}
            className="w-24 px-3 py-1.5 rounded-md border border-[var(--border)] bg-[var(--surface-2)] text-[var(--text)] text-sm"
          />
        </div>
        <button
          type="button"
          onClick={handlePreview}
          disabled={isLoading}
          className="px-4 py-2 bg-[var(--brand-500)] text-white rounded-lg text-sm font-medium hover:bg-[var(--brand-600)] transition-colors disabled:opacity-50"
        >
          {isLoading ? 'Loading…' : 'Preview'}
        </button>
      </div>

      {error && (
        <div className="mb-4 p-3 rounded-lg bg-[var(--danger)]/10 border border-[var(--danger)]/20 text-sm text-[var(--danger)]">
          {error.message}
        </div>
      )}

      {/* Summary */}
      {summary && (
        <div className="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-7 gap-3 mb-6">
          {[
            { label: 'Total Cards', value: summary.totalCards },
            { label: 'With Comps', value: summary.withComps },
            { label: 'Without Comps', value: summary.withoutComps },
            { label: 'No Data', value: summary.noData },
            { label: 'Current Value', value: dollars(summary.totalCurrentValueCents) },
            { label: 'Suggested Value', value: dollars(summary.totalSuggestedValueCents) },
            { label: 'Below Cost', value: summary.belowCostCount },
          ].map(stat => (
            <div key={stat.label} className="p-3 rounded-lg bg-[var(--surface)] border border-[var(--border)]">
              <div className="text-xs text-[var(--text-muted)] mb-1">{stat.label}</div>
              <div className="text-lg font-semibold text-[var(--text)]">{stat.value}</div>
            </div>
          ))}
        </div>
      )}

      {/* Table */}
      {items.length > 0 && (
        <>
          <div className="flex items-center gap-3 mb-2">
            <button type="button" onClick={selectAll} className="text-xs text-[var(--brand-500)] hover:underline">Select All</button>
            <button type="button" onClick={deselectAll} className="text-xs text-[var(--text-muted)] hover:underline">Deselect All</button>
            <button type="button" onClick={acceptAllSuggested} className="text-xs text-green-400 hover:underline">Accept All Suggested</button>
            <span className="text-xs text-[var(--text-muted)]">{selected.size} selected</span>
          </div>

          <div className="overflow-x-auto rounded-lg border border-[var(--border)]">
            <table className="w-full text-sm">
              <thead className="bg-[var(--surface)] border-b border-[var(--border)]">
                <tr>
                  <th className="px-3 py-2 text-left w-8"></th>
                  <th className="px-3 py-2 text-left text-[var(--text-muted)] font-medium">Card</th>
                  <th className="px-3 py-2 text-center text-[var(--text-muted)] font-medium">Grade</th>
                  <th className="px-3 py-2 text-left text-[var(--text-muted)] font-medium">Campaign</th>
                  <th className="px-3 py-2 text-right text-[var(--text-muted)] font-medium">Cost</th>
                  <th className="px-3 py-2 text-right text-[var(--text-muted)] font-medium">CL Value</th>
                  <th className="px-3 py-2 text-right text-[var(--text-muted)] font-medium">Comp Price</th>
                  <th className="px-3 py-2 text-center text-[var(--text-muted)] font-medium"># Comps</th>
                  <th className="px-3 py-2 text-center text-[var(--text-muted)] font-medium">Confidence</th>
                  <th className="px-3 py-2 text-right text-[var(--text-muted)] font-medium">Gap %</th>
                  <th className="px-3 py-2 text-right text-[var(--text-muted)] font-medium">Current</th>
                  <th className="px-3 py-2 text-right text-[var(--text-muted)] font-medium">Suggested</th>
                  <th className="px-3 py-2 text-right text-[var(--text-muted)] font-medium">Final Price</th>
                  <th className="px-3 py-2 w-8"></th>
                </tr>
              </thead>
              <tbody>
                {items.map(item => {
                  const isSelected = selected.has(item.purchaseId);
                  const rowClass = item.belowCost
                    ? 'bg-[var(--danger)]/5 border-b border-[var(--border)]'
                    : 'border-b border-[var(--border)] hover:bg-[var(--surface-2)]/50';
                  return (
                    <tr key={item.purchaseId} className={rowClass}>
                      <td className="px-3 py-2">
                        <input
                          type="checkbox"
                          checked={isSelected}
                          onChange={() => toggleSelect(item.purchaseId)}
                          className="rounded"
                        />
                      </td>
                      <td className="px-3 py-2 text-[var(--text)] font-medium max-w-[200px] truncate">
                        {item.cardName}
                        <div className="text-xs text-[var(--text-muted)]">{item.certNumber}</div>
                      </td>
                      <td className="px-3 py-2 text-center text-[var(--text)]">{item.grade}</td>
                      <td className="px-3 py-2 text-[var(--text-muted)] text-xs">{item.campaignName}</td>
                      <td className="px-3 py-2 text-right text-[var(--text)]">{dollars(item.buyCostCents)}</td>
                      <td className="px-3 py-2 text-right text-[var(--text)]">{dollars(item.clValueCents)}</td>
                      <td className="px-3 py-2 text-right text-[var(--text)]">{dollars(item.compPriceCents)}</td>
                      <td className="px-3 py-2 text-center text-[var(--text-muted)]">{item.compCount}</td>
                      <td className={`px-3 py-2 text-center capitalize ${confidenceColor(item.confidenceLevel)}`}>
                        {item.confidenceLevel}
                      </td>
                      <td className="px-3 py-2 text-right text-[var(--text-muted)]">
                        {item.gapPct !== 0 ? `${item.gapPct.toFixed(1)}%` : '—'}
                      </td>
                      <td className="px-3 py-2 text-right text-[var(--text-muted)]">{dollars(item.currentReviewedPriceCents)}</td>
                      <td className="px-3 py-2 text-right text-[var(--text)]">{dollars(item.suggestedPriceCents)}</td>
                      <td className="px-3 py-2 text-right">
                        <input
                          type="number"
                          min={0}
                          step={0.01}
                          value={(getFinalPrice(item) / 100).toFixed(2)}
                          onChange={e => handleFinalPriceChange(item.purchaseId, e.target.value)}
                          className="w-24 px-2 py-1 rounded border border-[var(--border)] bg-[var(--surface-2)] text-[var(--text)] text-right text-sm"
                        />
                      </td>
                      <td className="px-3 py-2">
                        {item.suggestedPriceCents > 0 && !isSelected && (
                          <button
                            type="button"
                            onClick={() => acceptSuggested(item.purchaseId)}
                            className="text-xs text-green-400 hover:text-green-300 whitespace-nowrap"
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
            <h2 className="text-lg font-bold text-[var(--text)] mb-2">Apply Liquidation Prices</h2>
            <p className="text-sm text-[var(--text-muted)] mb-6">
              This will update the reviewed price for {selected.size} card{selected.size !== 1 ? 's' : ''}. Continue?
            </p>
            {applyMutation.error && (
              <p className="text-xs text-[var(--danger)] mb-4">{applyMutation.error.message}</p>
            )}
            {applyMutation.data && (
              <p className="text-xs text-green-400 mb-4">
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
