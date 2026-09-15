import { useEffect, useId, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { Link } from 'react-router-dom';
import type { AgingItem } from '../../../../types/campaigns';
import type { ShowEvaluation } from '../../../../types/showprep';
import { useShowListWrites } from '../../../queries/useShowPrepQueries';
import { formatCents } from '../../../utils/formatters';
import { Button } from '../../../ui';
import ShowListPicker from '../../show-preparation/ShowListPicker';
import { showError } from '../../show-preparation/showPrepLabels';

export interface InventorySelectionBarProps {
  selectedItems: AgingItem[];
  selected: ReadonlySet<string>;
  selectedVersions: Readonly<Record<string, string>>;
  evaluations: Record<string, ShowEvaluation>;
  onRecordSale: () => void;
  onListOnDH: () => void;
  onClear: () => void;
  onAdded: (ids: string[]) => void;
  disabled?: boolean;
  outsideView?: number;
  onReveal?: () => void;
  showingSelected?: boolean;
  onHideSelected?: () => void;
  onHeightChange?: (height: number) => void;
}

export default function InventorySelectionBar({ selectedItems, selected, selectedVersions, evaluations,
  onRecordSale, onListOnDH, onClear, onAdded, disabled = false, outsideView = 0, onReveal,
  showingSelected, onHideSelected, onHeightChange }: InventorySelectionBarProps) {
  const [listId, setListId] = useState('');
  const [choosing, setChoosing] = useState(false);
  const [pickerOpened, setPickerOpened] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState<{ count: number; listId: string } | null>(null);
  const { add } = useShowListWrites();
  const bar = useRef<HTMLElement>(null);
  const addButton = useRef<HTMLButtonElement>(null);
  const picker = useRef<HTMLDivElement>(null);
  const pickerId = useId();
  const returnFocus = useRef(false);
  const totalListCents = useMemo(() => selectedItems.reduce((sum, i) => sum + (i.purchase.clValueCents ?? 0), 0), [selectedItems]);
  useEffect(() => {
    if (!selected.size || !bar.current) { onHeightChange?.(0); return; }
    const element = bar.current;
    const measure = () => onHeightChange?.(element.getBoundingClientRect().height);
    measure();
    const observer = new ResizeObserver(measure); observer.observe(element);
    return () => { observer.disconnect(); onHeightChange?.(0); };
  }, [selected.size, onHeightChange]);
  useEffect(() => {
    if (!selected.size || disabled || add.isPending) return;
    const escape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || event.defaultPrevented || document.querySelector('[role="dialog"], [role="alertdialog"]')) return;
      if (choosing) { returnFocus.current = true; setChoosing(false); }
      else onClear();
    };
    window.addEventListener('keydown', escape);
    return () => window.removeEventListener('keydown', escape);
  }, [selected.size, disabled, add.isPending, choosing, onClear]);
  useEffect(() => {
    if (choosing) picker.current?.focus();
    else if (returnFocus.current) { returnFocus.current = false; addButton.current?.focus(); }
  }, [choosing]);
  const ids = [...selected];
  const unavailable = ids.filter(id => {
    const e = evaluations[id];
    return !e?.canAdd || !['ready', 'not_received'].includes(e.availability);
  });
  const needsReselection = ids.filter(id => !selectedVersions[id] || selectedVersions[id] !== evaluations[id]?.version);
  const busy = disabled || add.isPending;
  const cannotAdd = busy || ids.length === 0 || ids.length > 200 || unavailable.length > 0 || needsReselection.length > 0;
  function closeDestination() { returnFocus.current = true; setChoosing(false); }
  async function addSelected() {
    if (cannotAdd) return;
    if (!choosing) { setPickerOpened(true); setChoosing(true); return; }
    if (!listId) return;
    setError(''); setSuccess(null);
    try {
      await add.mutateAsync({ id: listId, items: ids.map(purchaseId => ({ purchaseId, evaluationVersion: selectedVersions[purchaseId] })) });
      setSuccess({ count: ids.length, listId }); setChoosing(false); onAdded(ids);
    } catch (err) { setError(showError(err)); }
  }
  return <>
    {success && <p role="status" className="show-add-success text-[var(--success)]">
      Added {success.count} {success.count === 1 ? 'card' : 'cards'} to show list.{' '}
      <Link className="show-link" to={`/shows?list=${success.listId}`}>Open packing list →</Link>
    </p>}
    {/* Keep the picker mounted after opening so an uncertain create retains its retry UUID. */}
    {createPortal(<section ref={bar} hidden={ids.length === 0} className="show-selection-bar" aria-label="Bulk actions for selected cards" data-expanded={choosing || !!error || needsReselection.length > 0 || unavailable.length > 0 || outsideView > 0 || showingSelected}>
      {pickerOpened && <div ref={picker} hidden={!choosing} id={pickerId} tabIndex={-1} className="show-selection-destination" aria-label="Choose show destination">
        <ShowListPicker value={listId} onChange={id => { if (id !== listId) setSuccess(null); setListId(id); }} disabled={busy} />
        <div className="show-actions justify-between"><p className="show-selection-note">Shortlist only. No price, listing, or sale changes. Not-received cards can be planned, not packed.</p>
          <Button variant="ghost" size="sm" disabled={busy} onClick={closeDestination}>Close destination</Button></div>
      </div>}
      {showingSelected && <div className="show-actions show-selection-recovery"><span>Showing selected cards.</span><Button variant="ghost" size="sm" onClick={onHideSelected}>Back to matches</Button></div>}
      {outsideView > 0 && <div className="show-actions show-selection-recovery"><span>{outsideView} selected outside this view.</span><Button variant="ghost" size="sm" onClick={onReveal}>Reveal selected</Button></div>}
      {ids.length > 200 && <p className="text-[var(--warning)]">Select at most 200 cards per add. Selection is unchanged.</p>}
      {unavailable.length > 0 && <p className="show-selection-warning">{unavailable.length} selected cards are unavailable or not evaluated. Selection retained.</p>}
      {needsReselection.length > 0 && <p className="show-selection-warning">Selected data changed or was not observed. Review and reselect these cards before adding: {needsReselection.map(id => evaluations[id]?.certNumber || id).join(', ')}. Selection retained.</p>}
      {error && <p role="alert" className="text-[var(--danger)]">{error} Selection retained. Review updated data, then retry adding.</p>}
      <div className="show-selection-main">
        <span className="text-sm tabular-nums">{ids.length} selected{totalListCents > 0 && <> · {formatCents(totalListCents)} list</>}</span>
        <div className="show-actions">
          <Button ref={addButton} size="sm" aria-expanded={choosing} aria-controls={pickerId}
            disabled={cannotAdd || (choosing && !listId)} onClick={() => void addSelected()}>{add.isPending ? 'Adding…' : `Add to show (${ids.length})`}</Button>
          <Button variant="secondary" size="sm" onClick={onRecordSale} disabled={busy || selectedItems.length === 0}>Record sale ({selectedItems.length})</Button>
          <Button variant="secondary" size="sm" onClick={onListOnDH} disabled={busy || selectedItems.length === 0}>List on DH ({selectedItems.length})</Button>
          <Button variant="ghost" size="sm" disabled={busy} onClick={() => { setChoosing(false); onClear(); }}>Clear</Button>
        </div>
      </div>
    </section>, document.body)}
  </>;
}
