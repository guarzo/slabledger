import { useRef, useState } from 'react';
import { useShowLists, useShowListWrites } from '../../queries/useShowPrepQueries';
import { Button } from '../../ui';
import { showError } from './showPrepLabels';

export default function ShowListPicker({ value, onChange, allowRename = false, disabled = false }: {
  value: string; onChange: (id: string) => void; allowRename?: boolean; disabled?: boolean;
}) {
  const lists = useShowLists();
  const { create, rename } = useShowListWrites();
  const [name, setName] = useState('');
  const [editing, setEditing] = useState(false);
  const [renameValue, setRenameValue] = useState('');
  const [error, setError] = useState('');
  const attempt = useRef<{ id: string; name: string } | null>(null);
  const busy = disabled || create.isPending || rename.isPending;
  const selected = lists.data?.find(list => list.id === value);
  function validName(input: string) { return input.trim().length > 0 && [...input.trim()].length <= 120; }
  async function createList() {
    const trimmed = name.trim();
    if (!validName(trimmed)) { setError('Use a name of 1–120 characters.'); return; }
    // Retain the caller UUID across an explicit retry of the same intent too.
    if (attempt.current?.name !== trimmed) attempt.current = { id: crypto.randomUUID(), name: trimmed };
    setError('');
    try {
      const list = await create.mutateAsync(attempt.current!);
      onChange(list.id); setName(''); attempt.current = null;
    } catch (err) { setError(showError(err)); }
  }
  async function renameList() {
    if (!validName(renameValue)) { setError('Use a name of 1–120 characters.'); return; }
    setError('');
    try { await rename.mutateAsync({ id: value, name: renameValue.trim() }); setEditing(false); }
    catch (err) { setError(showError(err)); }
  }
  return <div>
    <div className="show-list-controls">
      <label>Show list
        <select className="show-input" value={value} disabled={busy || lists.isLoading} onChange={e => { onChange(e.target.value); setEditing(false); setError(''); }}>
          <option value="">Choose a show list</option>
          {lists.data?.map(list => <option key={list.id} value={list.id}>{list.name}</option>)}
        </select>
      </label>
      <form onSubmit={e => { e.preventDefault(); void createList(); }}>
        <label>New show name <input className="show-input" value={name} disabled={busy} onChange={e => setName(e.target.value)} /></label>
        <Button type="submit" variant="secondary" size="sm" disabled={busy || !validName(name)}>{create.isPending ? 'Creating…' : 'Create show list'}</Button>
      </form>
      {allowRename && selected && !editing && <Button variant="ghost" size="sm" disabled={busy} onClick={() => { setRenameValue(selected.name); setEditing(true); }}>Rename show list</Button>}
      {allowRename && editing && <form onSubmit={e => { e.preventDefault(); void renameList(); }}>
        <label>Show name <input className="show-input" value={renameValue} disabled={busy} onChange={e => setRenameValue(e.target.value)} /></label>
        <Button type="submit" size="sm" disabled={busy || !validName(renameValue)}>Save name</Button>
        <Button variant="ghost" size="sm" disabled={busy} onClick={() => setEditing(false)}>Cancel rename</Button>
      </form>}
    </div>
    {lists.isLoading && <p role="status">Loading saved lists…</p>}
    {lists.isError && <p role="alert">Could not load lists: {showError(lists.error)} <button className="show-link" disabled={lists.isFetching} onClick={() => lists.refetch()}>Retry lists</button></p>}
    {lists.data?.length === 0 && <p className="text-sm text-[var(--text-muted)]">No saved lists. Create a named list to start planning.</p>}
    {error && <p role="alert" className="text-sm text-[var(--danger)]">{error}</p>}
  </div>;
}
