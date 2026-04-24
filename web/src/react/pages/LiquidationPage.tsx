import { useState, useDeferredValue } from 'react';
import { useLiquidationPreview, useApplyLiquidation } from '../queries/useLiquidationQueries';
import type { LiquidationPreviewItem, ConfidenceLevel } from '../../types/liquidation';
import { formatCents } from '../utils/formatters';
import { GradeBadge } from '../ui';
import { LinkDropdown } from '../ui/LinkDropdown';
import { defaultEbayUrl, defaultAltUrl, defaultCardLadderUrl, gradeToGradeKey } from '../utils/marketplaceUrls';
import StatCard from '../ui/StatCard';

function confidenceColor(level: ConfidenceLevel): string {
  switch (level) {
    case 'high': return 'text-[var(--success)]';
    case 'medium': return 'text-[var(--warning)]';
    case 'low': return 'text-orange-400';
    default: return 'text-[var(--text-muted)]';
  }
}

interface PricePillProps {
  label: string;
  cents: number;
  active: boolean;
  onClick: () => void;
}

function PricePill({ label, cents, active, onClick }: PricePillProps) {
  if (cents <= 0) return null;
  return (
    <button
      type="button"
      onClick={onClick}
      className={`text-[10px] px-1.5 py-0.5 rounded-md border transition-colors tabular-nums whitespace-nowrap ${
        active
          ? 'border-[var(--brand-500)] bg-[var(--brand-500)]/20 text-[var(--brand-400)]'
          : 'border-[var(--surface-2)] bg-[var(--surface-1)] text-[var(--text-muted)] hover:border-[var(--brand-500)]/50'
      }`}
    >
      {label} {formatCents(cents)}
    </button>
  );
}

export default function LiquidationPage() {
  const [discountWithComps, setDiscountWithComps] = useState(2.5);
  const [discountNoComps, setDiscountNoComps] = useState(10);
  const deferredWithComps = useDeferredValue(discountWithComps);
  const deferredNoComps = useDeferredValue(discountNoComps);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [finalPrices, setFinalPrices] = useState<Record<string, number>>({});
  const [finalPriceInputs, setFinalPriceInputs] = useState<Record<string, string>>({});
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

  const getFinalPrice = (item: LiquidationPreviewItem): number =>
    finalPrices[item.purchaseId] ?? item.suggestedPriceCents;

  const setPillPrice = (id: string, cents: number) => {
    setFinalPrices(prev => ({ ...prev, [id]: cents }));
    setFinalPriceInputs(prev => ({ ...prev, [id]: cents > 0 ? (cents / 100).toFixed(2) : '' }));
  };

  const handleInputChange = (id: string, val: string) => {
    setFinalPriceInputs(prev => ({ ...prev, [id]: val }));
  };

  const handleInputBlur = (id: string) => {
    const val = finalPriceInputs[id] ?? '';
    if (val === '' || val === '.') {
      setFinalPrices(prev => ({ ...prev, [id]: 0 }));
      setFinalPriceInputs(prev => ({ ...prev, [id]: '' }));
      return;
    }
    const parts = val.split('.');
    const d = parseInt(parts[0] || '0', 10);
    const frac = (parts[1] || '0').slice(0, 2).padEnd(2, '0');
    const cents = d * 100 + parseInt(frac, 10);
    if (!isNaN(cents) && cents >= 0) {
      setFinalPrices(prev => ({ ...prev, [id]: cents }));
      setFinalPriceInputs(prev => ({ ...prev, [id]: cents > 0 ? (cents / 100).toFixed(2) : '' }));
    }
  };

  const acceptItem = (id: string) => {
    setSelected(prev => new Set(prev).add(id));
  };

  const acceptAllSuggested = () => {
    const priceable = items.filter(i => i.suggestedPriceCents > 0 || getFinalPrice(i) > 0);
    setSelected(prev => new Set([...prev, ...priceable.map(i => i.purchaseId)]));
  };

  const handleApply = () => {
    const applyItems = Array.from(selected)
      .map(id => {
        const item = items.find(i => i.purchaseId === id);
        return item ? { purchaseId: id, newPriceCents: getFinalPrice(item) } : null;
      })
      .filter((x): x is NonNullable<typeof x> => x !== null && x.newPriceCents > 0);
    applyMutation.mutate(applyItems, {
      onSuccess: () => {
        setShowConfirm(false);
        setSelected(new Set());
        setFinalPrices({});
        setFinalPriceInputs({});
      },
    });
  };

  const applyableCount = Array.from(selected).filter(id => {
    const item = items.find(i => i.purchaseId === id);
    return item && getFinalPrice(item) > 0;
  }).length;

  const summary = data?.summary;

  return (
    <div className="max-w-7xl mx-auto px-4 pb-16">
      <h1 className="text-[22px] font-bold text-[var(--text)] tracking-tight mb-6">Reprice</h1>

      <div className="mb-6 p-4 rounded-xl bg-[var(--surface-1)] border border-[var(--surface-2)]">
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-6">
          <DiscountSlider label="With comps" value={discountWithComps} onChange={setDiscountWithComps} />
          <DiscountSlider label="Without comps" value={discountNoComps} onChange={setDiscountNoComps} />
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

      {summary && (
        <div className="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-7 gap-3 mb-6">
          <StatCard label="Total Cards" value={String(summary.totalCards)} />
          <StatCard label="With Comps" value={String(summary.withComps)} color="green" />
          <StatCard label="Without Comps" value={String(summary.withoutComps)} />
          <StatCard label="No Data" value={String(summary.noData)} color={summary.noData > 0 ? 'red' : undefined} />
          <StatCard label="Current Value" value={formatCents(summary.totalCurrentValueCents)} />
          <StatCard label="Suggested Value" value={formatCents(summary.totalSuggestedValueCents)} />
          <StatCard label="Below Cost" value={String(summary.belowCostCount)} color={summary.belowCostCount > 0 ? 'red' : undefined} />
        </div>
      )}

      {items.length > 0 && (
        <>
          <div className="flex items-center gap-3 mb-2">
            <button type="button" onClick={selectAll} className="text-xs text-[var(--brand-500)] hover:underline">Select All</button>
            <button type="button" onClick={deselectAll} className="text-xs text-[var(--text-muted)] hover:underline">Deselect All</button>
            <button type="button" onClick={acceptAllSuggested} className="text-xs text-[var(--success)] hover:opacity-80">Accept All</button>
            <span className="text-xs text-[var(--text-muted)]">{selected.size} selected</span>
          </div>

          <div className="glass-table">
            <div className="glass-table-header flex items-center sticky top-0 z-10">
              <div className="glass-table-th flex-shrink-0 !px-1" style={{ width: '28px' }}>
                <input
                  type="checkbox"
                  aria-label="Select all"
                  checked={items.length > 0 && items.every(i => selected.has(i.purchaseId))}
                  onChange={() => items.every(i => selected.has(i.purchaseId)) ? deselectAll() : selectAll()}
                  className="rounded accent-[var(--brand-500)]"
                />
              </div>
              <div className="glass-table-th flex-1 min-w-0 text-left">Card</div>
              <div className="glass-table-th flex-shrink-0 text-center" style={{ width: '48px' }}>Gr</div>
              <div className="glass-table-th flex-shrink-0 text-center" style={{ width: '56px' }}>Conf</div>
              <div className="glass-table-th flex-shrink-0 text-center" style={{ width: '250px' }}>Price Options</div>
              <div className="glass-table-th flex-shrink-0 text-right" style={{ width: '56px' }}>Current</div>
              <div className="glass-table-th flex-shrink-0 text-right" style={{ width: '100px' }}>Final Price</div>
              <div className="glass-table-th flex-shrink-0 text-center" style={{ width: '56px' }}></div>
            </div>

            <div className="max-h-[600px] overflow-y-auto overflow-x-hidden scrollbar-dark">
              {items.map((item, index) => {
                const isSelected = selected.has(item.purchaseId);
                const currentFinal = getFinalPrice(item);
                const card = { name: item.cardName, setName: item.setName, number: item.cardNumber };
                const grade = gradeToGradeKey(item.grade);
                const links = [
                  { label: 'CardLadder', href: defaultCardLadderUrl(card, grade) },
                  { label: 'eBay', href: defaultEbayUrl(card, grade) },
                  { label: 'Alt', href: defaultAltUrl(card, grade) },
                ];

                return (
                  <div
                    key={item.purchaseId}
                    className="glass-vrow flex items-center"
                    data-stripe={index % 2 === 1}
                    data-selected={isSelected}
                    style={item.belowCost ? { background: 'rgba(248,113,113,0.05)' } : undefined}
                  >
                    <div className="glass-table-td flex-shrink-0 !px-1" style={{ width: '28px' }}>
                      <input
                        type="checkbox"
                        aria-label={`Select ${item.cardName}`}
                        checked={isSelected}
                        onChange={() => toggleSelect(item.purchaseId)}
                        className="rounded accent-[var(--brand-500)]"
                      />
                    </div>
                    <div className="glass-table-td flex-1 min-w-0" title={item.cardName}>
                      <div className="flex items-center gap-1.5 min-w-0">
                        <span className="text-[var(--text)] truncate text-sm">{item.cardName}</span>
                        <LinkDropdown links={links} stopPropagation />
                      </div>
                      <div className="text-[10px] text-[var(--text-muted)] truncate leading-tight">
                        {item.setName && <>{item.setName}</>}
                        {item.cardNumber && <> &middot; #{item.cardNumber}</>}
                        {item.certNumber && <> &middot; {item.certNumber}</>}
                      </div>
                    </div>
                    <div className="glass-table-td flex-shrink-0 text-center" style={{ width: '48px' }}>
                      <GradeBadge grader="PSA" grade={item.grade} size="sm" />
                    </div>
                    <div className={`glass-table-td flex-shrink-0 text-center text-[10px] capitalize ${confidenceColor(item.confidenceLevel)}`} style={{ width: '56px' }}>
                      {item.confidenceLevel}
                      {item.compCount > 0 && <div className="text-[var(--text-muted)]">{item.compCount}c</div>}
                    </div>
                    <div className="glass-table-td flex-shrink-0" style={{ width: '250px' }}>
                      <div className="flex flex-wrap items-center gap-1">
                        <PricePill label="Cost" cents={item.buyCostCents} active={currentFinal === item.buyCostCents} onClick={() => setPillPrice(item.purchaseId, item.buyCostCents)} />
                        <PricePill label="CL" cents={item.clValueCents} active={currentFinal === item.clValueCents} onClick={() => setPillPrice(item.purchaseId, item.clValueCents)} />
                        <PricePill label="Comp" cents={item.compPriceCents} active={currentFinal === item.compPriceCents} onClick={() => setPillPrice(item.purchaseId, item.compPriceCents)} />
                        <PricePill label="Sug" cents={item.suggestedPriceCents} active={currentFinal === item.suggestedPriceCents} onClick={() => setPillPrice(item.purchaseId, item.suggestedPriceCents)} />
                      </div>
                    </div>
                    <div className="glass-table-td flex-shrink-0 text-right text-[var(--text-muted)] tabular-nums text-xs" style={{ width: '56px' }}>
                      {item.currentReviewedPriceCents > 0 ? formatCents(item.currentReviewedPriceCents) : '—'}
                    </div>
                    <div className="glass-table-td flex-shrink-0 text-right" style={{ width: '100px' }}>
                      <input
                        type="text"
                        inputMode="decimal"
                        aria-label={`Final price for ${item.cardName}`}
                        value={finalPriceInputs[item.purchaseId] ?? (currentFinal > 0 ? (currentFinal / 100).toFixed(2) : '')}
                        onChange={e => handleInputChange(item.purchaseId, e.target.value)}
                        onBlur={() => handleInputBlur(item.purchaseId)}
                        placeholder="0.00"
                        className="w-20 px-2 py-1 rounded border border-[var(--surface-2)] bg-[var(--surface-1)] text-[var(--text)] text-right text-sm tabular-nums"
                      />
                    </div>
                    <div className="glass-table-td flex-shrink-0 text-center" style={{ width: '56px' }}>
                      {!isSelected ? (
                        <button
                          type="button"
                          onClick={() => acceptItem(item.purchaseId)}
                          className="text-xs text-[var(--success)] hover:opacity-80 whitespace-nowrap"
                        >
                          Accept
                        </button>
                      ) : (
                        <span className="text-xs text-[var(--success)]">&#10003;</span>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        </>
      )}

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

      {showConfirm && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-20 p-4">
          <div className="bg-[var(--surface)] rounded-xl border border-[var(--border)] p-6 max-w-sm w-full">
            <h2 className="text-lg font-bold text-[var(--text)] mb-2">Apply Repriced Values</h2>
            <p className="text-sm text-[var(--text-muted)] mb-6">
              This will update the reviewed price for {applyableCount} card{applyableCount !== 1 ? 's' : ''}.
              {applyableCount < selected.size && ` (${selected.size - applyableCount} skipped — no price set)`}
              {' '}Continue?
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
