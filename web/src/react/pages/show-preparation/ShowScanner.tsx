import { useEffect, useRef, useState } from 'react';
import { useIsMutating } from '@tanstack/react-query';
import { useShowList } from '../../queries/useShowPrepQueries';

export default function ShowScanner({ listId, ready, capture }: {
  listId: string; ready: boolean; capture: (cert: string, generation: number) => boolean;
}) {
  const list = useShowList(listId);
  const removing = useIsMutating({ mutationKey: ['show-prep', 'remove'], predicate: mutation => (mutation.state.variables as { id?: string } | undefined)?.id === listId });
  const [input, setInput] = useState('');
  const field = useRef<HTMLInputElement>(null);
  const generation = list.data?.list.withdrawalGeneration;
  const canScan = ready && !removing && !!list.data && !list.isError && Number.isSafeInteger(generation) && generation! >= 0;
  useEffect(() => {
    if (canScan && (document.activeElement === document.body || document.activeElement?.matches('.show-picker select'))) field.current?.focus();
  }, [canScan]);
  return <section className="show-scanner" aria-label="Scan cards for show">
    <div className="show-scanner-heading"><h2>Scan cards</h2><span className="text-xs text-[var(--text-muted)]">Enter confirms the checked slab and sticker for this show.</span></div>
    <label htmlFor="show-scan-input">Scan slab barcode</label>
    <input id="show-scan-input" ref={field} className="show-input" autoComplete="off" inputMode="text"
      value={input} disabled={!canScan} placeholder="Scan cert, then Enter" onChange={event => setInput(event.target.value)}
      onKeyDown={event => {
        if (event.key !== 'Enter' || !canScan) return;
        event.preventDefault();
        if (capture(input, generation!)) setInput('');
        field.current?.focus();
      }} />
    {list.isFetching && <p role="status">Reading show contents…</p>}
    {list.isError && <p role="alert" className="text-[var(--danger)]">Could not read show contents. Scanning paused. <button type="button" className="show-link" onClick={() => void list.refetch()}>Retry show read</button></p>}
  </section>;
}
