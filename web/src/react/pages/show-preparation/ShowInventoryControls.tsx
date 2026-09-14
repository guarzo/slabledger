import { useState } from 'react';
import { Link } from 'react-router-dom';
import type { ShowEvaluation, SupportStatus } from '../../../types/showprep';
import { useShowListWrites } from '../../queries/useShowPrepQueries';
import { Button } from '../../ui';
import ShowListPicker from './ShowListPicker';
import ShowRefresh from './ShowRefresh';
import { showError, supportLabels } from './showPrepLabels';
import './show-preparation.css';

export interface ShowFilters {
  support: SupportStatus | 'all';
  selecting: boolean;
  includeNotReceived: boolean;
  evaluations: Record<string, ShowEvaluation>;
}
export function ShowInventoryFilters({ filters, setSupport, setSelecting, setIncludeNotReceived, count, pending, failed, onRetry, fetching }: {
  filters: ShowFilters; setSupport: (value: SupportStatus | 'all') => void;
  setSelecting: (value: boolean) => void; setIncludeNotReceived: (value: boolean) => void;
  count: number; pending: boolean; failed: number; onRetry: () => void; fetching: boolean;
}) {
  return <div className="show-toolbar">
    <label>Price support
      <select className="show-input" value={filters.support} onChange={e => setSupport(e.target.value as SupportStatus | 'all')}>
        <option value="all">All price support</option>
        {Object.entries(supportLabels).map(([key, label]) => <option key={key} value={key}>{label}</option>)}
      </select>
    </label>
    <Button variant={filters.selecting ? 'primary' : 'secondary'} size="sm" aria-pressed={filters.selecting} onClick={() => setSelecting(!filters.selecting)}>Show selection</Button>
    {filters.selecting && <label className="show-check"><input type="checkbox" checked={filters.includeNotReceived} onChange={e => setIncludeNotReceived(e.target.checked)} />Include not received</label>}
    <span className="text-[var(--text-muted)] tabular-nums" role="status">{count} {count === 1 ? 'card' : 'cards'} shown{pending ? ' · Loading evaluations…' : ''}</span>
    <Link className="show-link" to="/shows">Saved show lists →</Link>
    {failed > 0 && <p role="alert" className="w-full text-[var(--warning)]">{failed} evaluations unavailable; not classified as no comps. <button className="show-link" disabled={fetching} onClick={onRetry}>Retry evaluation</button></p>}
    {filters.selecting && <p className="w-full text-xs text-[var(--text-muted)]">Ready to pack by default. Evidence quality does not block manual selection. Adding and packing do not reprice, delist, reserve, or sell cards.</p>}
  </div>;
}

export function ShowSelectionActions({ selected, selectedVersions, evaluations, includeNotReceived, onClear, onAdded, disabled = false }: {
  selected: ReadonlySet<string>; selectedVersions: Readonly<Record<string, string>>; evaluations: Record<string, ShowEvaluation>; includeNotReceived: boolean;
  onClear: () => void; onAdded: (ids: string[]) => void; disabled?: boolean;
}) {
  const [listId, setListId] = useState('');
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const { add } = useShowListWrites();
  const ids = [...selected];
  const unavailable = ids.filter(id => {
    const e = evaluations[id];
    return !e?.canAdd || !(e.availability === 'ready' || (includeNotReceived && e.availability === 'not_received'));
  });
  const needsReselection = ids.filter(id => !selectedVersions[id] || selectedVersions[id] !== evaluations[id]?.version);
  async function addSelected() {
    if (!listId || ids.length === 0 || ids.length > 200 || unavailable.length > 0 || needsReselection.length > 0) return;
    setError(''); setSuccess('');
    try {
      await add.mutateAsync({ id: listId, items: ids.map(purchaseId => ({ purchaseId, evaluationVersion: selectedVersions[purchaseId] })) });
      setSuccess(`Added ${ids.length} selected cards to show list.`); onAdded(ids);
    } catch (err) { setError(showError(err)); }
  }
  return <section className="show-selection" aria-label="Show selection actions">
    <div className="show-actions"><strong className="tabular-nums">{ids.length} selected for show</strong>
      {ids.length > 0 && <Button variant="ghost" size="sm" disabled={disabled || add.isPending} onClick={onClear}>Clear selection</Button>}
    </div>
    <ShowListPicker value={listId} onChange={id => { if (id !== listId) setSuccess(''); setListId(id); }} disabled={disabled || add.isPending} />
    <div className="show-actions">
      <Button size="sm" disabled={disabled || add.isPending || !listId || ids.length === 0 || ids.length > 200 || unavailable.length > 0 || needsReselection.length > 0} onClick={() => void addSelected()}>{add.isPending ? 'Adding…' : `Add selected to show (${ids.length})`}</Button>
      <ShowRefresh purchaseIds={ids} disabled={disabled || add.isPending} />
    </div>
    {ids.length > 200 && <p className="text-[var(--warning)]">Select at most 200 cards per add. Selection is unchanged.</p>}
    {unavailable.length > 0 && <p className="text-[var(--warning)]">{unavailable.length} selected cards are unavailable, not evaluated, or require “Include not received”. Selection is retained; review before adding.</p>}
    {needsReselection.length > 0 && <p className="text-[var(--warning)]">Selected data changed or was not observed. Review and reselect these cards before adding: {needsReselection.map(id => evaluations[id]?.certNumber || id).join(', ')}. Selection retained.</p>}
    {error && <p role="alert" className="text-[var(--danger)]">{error} Selection retained. Review updated data, then retry adding.</p>}
    {success && <p role="status" className="text-[var(--success)]">{success} <Link className="show-link" to={`/shows?list=${listId}`}>Open packing list →</Link></p>}
  </section>;
}
