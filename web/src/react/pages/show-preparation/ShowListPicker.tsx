import { useEffect, useRef, useState } from 'react';
import { useShowLists, useShowListWrites } from '../../queries/useShowPrepQueries';
import { Button } from '../../ui';
import { showError } from './showPrepLabels';

export default function ShowListPicker({ value, onChange, allowRename = false, disabled = false }: {
  value: string; onChange: (id: string) => void; allowRename?: boolean; disabled?: boolean;
}) {
  const lists = useShowLists();
  const { create, rename } = useShowListWrites();
  const [name, setName] = useState('');
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState(false);
  const [renameValue, setRenameValue] = useState('');
  const [error, setError] = useState('');
  const attempt = useRef<{ id: string; name: string } | null>(null);
  const newButton = useRef<HTMLButtonElement>(null);
  const select = useRef<HTMLSelectElement>(null);
  const nameInput = useRef<HTMLInputElement>(null);
  const busy = disabled || create.isPending || rename.isPending;
  const selected = lists.data?.find(list => list.id === value);
  const empty = lists.data?.length === 0;
  const createVisible = creating || empty;
  useEffect(() => { if (creating || editing) nameInput.current?.focus(); }, [creating, editing]);
  function validName(input: string) { return input.trim().length > 0 && [...input.trim()].length <= 120; }
  function cancelEditor() {
    setCreating(false); setEditing(false); setError('');
    // Keep the name and UUID: cancelling disclosure is not a new creation intent.
    if (creating) newButton.current?.focus(); else select.current?.focus();
  }
  async function createList() {
    const trimmed = name.trim();
    if (!validName(trimmed)) { setError('Use a name of 1–120 characters.'); return; }
    setError('');
    try {
      if (attempt.current?.name !== trimmed) attempt.current = { id: crypto.randomUUID(), name: trimmed };
      const list = await create.mutateAsync(attempt.current!);
      onChange(list.id); setName(''); attempt.current = null; setCreating(false);
      select.current?.focus();
    } catch (err) { setError(showError(err)); }
  }
  async function renameList() {
    if (!validName(renameValue)) { setError('Use a name of 1–120 characters.'); return; }
    setError('');
    try { await rename.mutateAsync({ id: value, name: renameValue.trim() }); setEditing(false); select.current?.focus(); }
    catch (err) { setError(showError(err)); }
  }
  return <div className="show-picker" onKeyDown={event => {
    if (event.key === 'Escape' && (creating || editing)) {
      event.preventDefault(); event.stopPropagation(); if (!busy) cancelEditor();
    }
  }}>
    {lists.isLoading && <p role="status">Loading saved lists…</p>}
    {lists.isError && <p role="alert">Could not load lists: {showError(lists.error)} <button className="show-link" disabled={lists.isFetching} onClick={() => lists.refetch()}>Retry lists</button></p>}
    {empty && <div className="show-empty-lists">
      <h2>{allowRename ? 'No show lists yet' : 'Name your first show'}</h2>
      <p className="text-[var(--text-muted)]">{allowRename ? 'Create a list, then select slabs from Inventory.' : 'Create a destination for the selected slabs.'}</p>
    </div>}
    <div className="show-list-controls">
      {!empty && lists.data && <>
        <label>Show list
          <select ref={select} className="show-input" value={value} disabled={busy} onChange={e => { onChange(e.target.value); setEditing(false); setError(''); }}>
            <option value="">Choose a show list</option>
            {lists.data.map(list => <option key={list.id} value={list.id}>{list.name}</option>)}
          </select>
        </label>
        <Button ref={newButton} variant="ghost" size="sm" disabled={busy} aria-expanded={creating} onClick={() => { setCreating(!creating); setEditing(false); setError(''); }}>New list</Button>
        {allowRename && selected && !editing && !creating && <Button variant="ghost" size="sm" disabled={busy} onClick={() => { setRenameValue(selected.name); setEditing(true); }}>Rename show list</Button>}
      </>}
      {createVisible && <form onSubmit={e => { e.preventDefault(); void createList(); }}>
        <label>New show name <input ref={nameInput} className="show-input" value={name} disabled={busy} onChange={e => setName(e.target.value)} placeholder="September card show" /></label>
        <Button type="submit" variant="primary" size="sm" disabled={busy || !validName(name)}>{create.isPending ? 'Creating…' : 'Create show list'}</Button>
        {!empty && <Button variant="ghost" size="sm" disabled={busy} onClick={cancelEditor}>Cancel new list</Button>}
      </form>}
      {allowRename && editing && <form onSubmit={e => { e.preventDefault(); void renameList(); }}>
        <label>Show name <input ref={nameInput} className="show-input" value={renameValue} disabled={busy} onChange={e => setRenameValue(e.target.value)} /></label>
        <Button type="submit" size="sm" disabled={busy || !validName(renameValue)}>Save name</Button>
        <Button variant="ghost" size="sm" disabled={busy} onClick={cancelEditor}>Cancel rename</Button>
      </form>}
    </div>
    {error && <p role="alert" className="text-sm text-[var(--danger)]">{error}</p>}
  </div>;
}
