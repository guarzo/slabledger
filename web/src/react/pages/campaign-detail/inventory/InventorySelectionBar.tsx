import { useEffect, useMemo, useRef } from 'react';
import { createPortal } from 'react-dom';
import type { AgingItem } from '../../../../types/campaigns';
import { formatCents } from '../../../utils/formatters';
import { Button } from '../../../ui';

export interface InventorySelectionBarProps {
  selectedItems: AgingItem[];
  selected: ReadonlySet<string>;
  onRecordSale: () => void;
  onListOnDH: () => void;
  onClear: () => void;
  disabled?: boolean;
  outsideView?: number;
  onReveal?: () => void;
  showingSelected?: boolean;
  onHideSelected?: () => void;
  onHeightChange?: (height: number) => void;
}
export default function InventorySelectionBar({ selectedItems, selected, onRecordSale, onListOnDH, onClear,
  disabled = false, outsideView = 0, onReveal, showingSelected, onHideSelected, onHeightChange }: InventorySelectionBarProps) {
  const bar = useRef<HTMLElement>(null);
  const totalListCents = useMemo(() => selectedItems.reduce((sum, item) => sum + (item.purchase.clValueCents ?? 0), 0), [selectedItems]);
  useEffect(() => {
    if (!selected.size || !bar.current) { onHeightChange?.(0); return; }
    const element = bar.current;
    const measure = () => onHeightChange?.(element.getBoundingClientRect().height);
    measure(); const observer = new ResizeObserver(measure); observer.observe(element);
    return () => { observer.disconnect(); onHeightChange?.(0); };
  }, [selected.size, onHeightChange]);
  useEffect(() => {
    if (!selected.size || disabled) return;
    const escape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || event.defaultPrevented || document.querySelector('[role="dialog"], [role="alertdialog"]')) return;
      onClear();
    };
    window.addEventListener('keydown', escape);
    return () => window.removeEventListener('keydown', escape);
  }, [selected.size, disabled, onClear]);
  if (!selected.size) return null;
  const missing = [...selected].filter(id => !selectedItems.some(item => item.purchase.id === id));
  return createPortal(<section ref={bar} className="show-selection-bar" aria-label="Bulk actions for selected cards" data-expanded={missing.length > 0 || outsideView > 0 || showingSelected}>
    {showingSelected && <div className="show-actions show-selection-recovery"><span>Showing selected cards.</span><Button variant="ghost" size="sm" onClick={onHideSelected}>Back to matches</Button></div>}
    {outsideView > 0 && <div className="show-actions show-selection-recovery"><span>{outsideView} selected outside this view.</span><Button variant="ghost" size="sm" onClick={onReveal}>Reveal selected</Button></div>}
    {missing.length > 0 && <p className="show-selection-warning">{missing.length} selected {missing.length === 1 ? 'card is' : 'cards are'} no longer in inventory. Selection retained.</p>}
    <div className="show-selection-main">
      <span className="text-sm tabular-nums">{selected.size} selected{totalListCents > 0 && <> · {formatCents(totalListCents)} CL value</>}</span>
      <div className="show-actions">
        <Button variant="secondary" size="sm" onClick={onRecordSale} disabled={disabled || selectedItems.length === 0}>Record sale ({selectedItems.length})</Button>
        <Button variant="secondary" size="sm" onClick={onListOnDH} disabled={disabled || selectedItems.length === 0}>List on DH ({selectedItems.length})</Button>
        <Button variant="ghost" size="sm" disabled={disabled} onClick={onClear}>Clear</Button>
      </div>
    </div>
  </section>, document.body);
}
