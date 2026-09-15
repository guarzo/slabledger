import { useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { useShowList, useShowLists } from '../queries/useShowPrepQueries';
import { Button } from '../ui';
import { formatCents } from '../utils/formatters';
import ShowListPicker from './show-preparation/ShowListPicker';
import ShowMember from './show-preparation/ShowMember';
import ShowRefresh from './show-preparation/ShowRefresh';
import { showError } from './show-preparation/showPrepLabels';
import './show-preparation/show-preparation.css';

function PackingList({ listId }: { listId: string }) {
  const query = useShowList(listId);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const data = query.data;
  const summary = data?.summary;
  const refreshIds = (data?.items ?? []).filter(item => selected.has(item.purchaseId)).map(item => item.purchaseId);
  return <section aria-label="Packing list" className="mt-4">
    <div className="show-actions">
      <Button variant="secondary" size="sm" disabled={query.isFetching} onClick={() => query.refetch()}>Update list status</Button>
      <span className="text-xs text-[var(--text-muted)]">Reads saved prices and availability, not new comps.</span>
    </div>
    {query.isFetching && <p role="status" className="mt-3">Loading packing list…</p>}
    {query.isError && <p role="alert" className="text-[var(--danger)] mt-3">Could not recheck list: {showError(query.error)}. {data ? 'Showing the last read; new packing and acknowledgement are disabled.' : ''} <button className="show-link" disabled={query.isFetching} onClick={() => query.refetch()}>Retry packing list</button></p>}
    {data && summary && <>
      <h2 className="text-xl mt-5 break-words">{data.list.name}</h2>
      <dl className="show-totals">
        <div><dt>Members</dt><dd>{summary.totalCount}</dd></div>
        <div><dt>Packed (includes history)</dt><dd>{summary.packedCount}</dd></div>
        <div><dt>Not received</dt><dd>{summary.notReceivedCount}</dd></div>
        <div><dt>Unavailable</dt><dd>{summary.unavailableCount}</dd></div>
        <div aria-label="Known ready-to-pack listed value"><dt>Known ready-to-pack listed value</dt><dd>{formatCents(summary.knownValueCents)}</dd></div>
        <div aria-label="Missing DH prices"><dt>Missing DH prices</dt><dd>{summary.missingPriceCount}</dd></div>
        <div aria-label="Ambiguous DH prices"><dt>Ambiguous DH prices</dt><dd>{summary.ambiguousPriceCount}</dd></div>
      </dl>
      <p className="text-xs text-[var(--text-muted)] mt-2">Known value includes only ready-to-pack members with a positive, unambiguous DH listed price. Missing and ambiguous prices are excluded, not valued at zero.</p>
      {data.items.length === 0 ? <p className="py-8 text-[var(--text-muted)]">No slabs in this list. <Link className="show-link" to="/inventory">Select cards from inventory →</Link></p> : <>
        {data.items.map(item => <ShowMember key={item.id} item={item} listId={listId} stale={query.isError || query.isFetching}
          selected={selected.has(item.purchaseId)} onSelect={() => setSelected(prev => {
            const next = new Set(prev); if (next.has(item.purchaseId)) next.delete(item.purchaseId); else next.add(item.purchaseId); return next;
          })} />)}
        <div className="mt-4"><ShowRefresh scope={`list:${listId}`} evaluations={Object.fromEntries(data.items.flatMap(item => item.evaluation ? [[item.purchaseId, item.evaluation]] : []))} purchaseIds={refreshIds} disabled={query.isFetching} /></div>
      </>}
    </>}
  </section>;
}

export default function ShowPreparationPage() {
  const [params, setParams] = useSearchParams();
  const listId = params.get('list') || '';
  const lists = useShowLists();
  return <div className="show-prep-page">
    <header>
      <div className="show-actions justify-between"><h1 className="page-title">Shows</h1><Link className="show-link" to="/inventory">Inventory →</Link></div>
      <p className="text-sm text-[var(--text-muted)]">Shortlist slabs, then pack. Lists do not change prices, listings, or sales.</p>
    </header>
    <ShowListPicker value={listId} onChange={id => setParams(id ? { list: id } : {})} allowRename />
    {listId ? <PackingList key={listId} listId={listId} /> : !!lists.data?.length && <p className="text-sm text-[var(--text-muted)] py-8">Choose a show list to review and pack its slabs.</p>}
  </div>;
}
