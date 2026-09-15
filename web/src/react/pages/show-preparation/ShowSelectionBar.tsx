import { useEffect, useId, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { Link } from 'react-router-dom';
import type { ShowEvaluation } from '../../../types/showprep';
import { useShowListWrites } from '../../queries/useShowPrepQueries';
import { Button } from '../../ui';
import ShowListPicker from './ShowListPicker';
import ShowRefresh from './ShowRefresh';
import { showError } from './showPrepLabels';
import './show-preparation.css';

export default function ShowSelectionBar({ selected, selectedVersions, evaluations, includeNotReceived, onClear, onAdded, disabled = false, outsideView = 0, onReveal, showingSelected, onHideSelected, modalOpen = false, onHeightChange }: {
  selected: ReadonlySet<string>; selectedVersions: Readonly<Record<string, string>>; evaluations: Record<string, ShowEvaluation>; includeNotReceived: boolean;
  onClear: () => void; onAdded: (ids: string[]) => void; disabled?: boolean; outsideView?: number; onReveal?: () => void;
  showingSelected?: boolean; onHideSelected?: () => void; modalOpen?: boolean; onHeightChange?: (height: number) => void;
}) {
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
  useEffect(() => {
    if (!selected.size || !bar.current) { onHeightChange?.(0); return; }
    const element = bar.current;
    const measure = () => onHeightChange?.(element.getBoundingClientRect().height);
    measure();
    const observer = new ResizeObserver(measure); observer.observe(element);
    return () => { observer.disconnect(); onHeightChange?.(0); };
  }, [selected.size, onHeightChange]);
  useEffect(() => {
    if (!selected.size || modalOpen || add.isPending) return;
    const escape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || event.defaultPrevented || document.querySelector('[role="dialog"], [role="alertdialog"]')) return;
      if (choosing) { returnFocus.current = true; setChoosing(false); }
      else onClear();
    };
    window.addEventListener('keydown', escape);
    return () => window.removeEventListener('keydown', escape);
  }, [selected.size, modalOpen, add.isPending, choosing, onClear]);
  useEffect(() => {
    if (choosing) picker.current?.focus();
    else if (returnFocus.current) { returnFocus.current = false; addButton.current?.focus(); }
  }, [choosing]);
  const ids = [...selected];
  const unavailable = ids.filter(id => {
    const e = evaluations[id];
    return !e?.canAdd || !(e.availability === 'ready' || (includeNotReceived && e.availability === 'not_received'));
  });
  const needsReselection = ids.filter(id => !selectedVersions[id] || selectedVersions[id] !== evaluations[id]?.version);
  const cannotAdd = disabled || modalOpen || add.isPending || ids.length === 0 || ids.length > 200 || unavailable.length > 0 || needsReselection.length > 0;
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
    {/* Hide rather than unmount: retry UUIDs and manual-run ownership survive clearing/closing. */}
    {createPortal(<section ref={bar} hidden={ids.length === 0} className="show-selection-bar" aria-label="Show selection actions" data-expanded={choosing || !!error || needsReselection.length > 0 || unavailable.length > 0 || outsideView > 0 || showingSelected}>
      {pickerOpened && <div ref={picker} hidden={!choosing} id={pickerId} tabIndex={-1} className="show-selection-destination" aria-label="Choose show destination">
        <ShowListPicker value={listId} onChange={id => { if (id !== listId) setSuccess(null); setListId(id); }} disabled={disabled || modalOpen || add.isPending} />
        <div className="show-actions justify-between"><p className="show-selection-note">Shortlist only. No price, listing, or sale changes.</p>
          <Button variant="ghost" size="sm" disabled={modalOpen || add.isPending} onClick={closeDestination}>Close destination</Button></div>
      </div>}
      {showingSelected && <div className="show-actions show-selection-recovery"><span>Showing selected cards.</span><Button variant="ghost" size="sm" onClick={onHideSelected}>Back to matches</Button></div>}
      {outsideView > 0 && <div className="show-actions show-selection-recovery"><span>{outsideView} selected outside this view.</span><Button variant="ghost" size="sm" onClick={onReveal}>Reveal selected</Button></div>}
      {ids.length > 200 && <p className="text-[var(--warning)]">Select at most 200 cards per add. Selection is unchanged.</p>}
      {unavailable.length > 0 && <p className="show-selection-warning">{unavailable.length} selected cards are unavailable, not evaluated, or require “Include not received”. Selection retained.</p>}
      {needsReselection.length > 0 && <p className="show-selection-warning">Selected data changed or was not observed. Review and reselect these cards before adding: {needsReselection.map(id => evaluations[id]?.certNumber || id).join(', ')}. Selection retained.</p>}
      {error && <p role="alert" className="text-[var(--danger)]">{error} Selection retained. Review updated data, then retry adding.</p>}
      <div className="show-selection-main">
        <strong className="tabular-nums">{ids.length} selected for show</strong>
        <Button className="show-selection-clear" variant="ghost" size="sm" disabled={modalOpen || add.isPending} onClick={() => { setChoosing(false); onClear(); }}>Clear selection</Button>
        <div className="show-actions">
          <Button ref={addButton} size="sm" aria-expanded={choosing} aria-controls={pickerId}
            disabled={cannotAdd || (choosing && !listId)} onClick={() => void addSelected()}>{add.isPending ? 'Adding…' : `Add selected to show (${ids.length})`}</Button>
          <ShowRefresh purchaseIds={ids} evaluations={evaluations} disabled={disabled || modalOpen || add.isPending} compact />
        </div>
      </div>
    </section>, document.body)}
  </>;
}
