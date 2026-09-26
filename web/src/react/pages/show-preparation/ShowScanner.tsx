import { useEffect, useMemo, useRef, useState } from 'react';
import { useGlobalInventory } from '../../queries/useCampaignQueries';
import { useShowEvaluations, useShowList, useShowListWrites } from '../../queries/useShowPrepQueries';
import { Button } from '../../ui';
import type { ShowEvaluation } from '../../../types/showprep';
import { showError } from './showPrepLabels';

interface Scan { cert: string; purchaseId?: string; cardName?: string; version?: string; ambiguous?: boolean }
const EMPTY_EVALUATIONS: Record<string, ShowEvaluation> = {};

export default function ShowScanner({ listId }: { listId: string }) {
  const inventory = useGlobalInventory();
  const list = useShowList(listId);
  const { add } = useShowListWrites();
  const [input, setInput] = useState('');
  const [scans, setScans] = useState<Scan[]>([]);
  const [notice, setNotice] = useState('');
  const [error, setError] = useState('');
  const field = useRef<HTMLInputElement>(null);
  const ids = useMemo(() => scans.flatMap(scan => scan.purchaseId ? [scan.purchaseId] : []), [scans]);
  const evaluations = useShowEvaluations(ids);
  const current = evaluations.data?.evaluations ?? EMPTY_EVALUATIONS;
  const members = new Set(list.data?.items.map(item => item.purchaseId) ?? []);
  const canScan = !!inventory.data && !inventory.isError && !!list.data && !list.isError && !add.isPending;

  useEffect(() => {
    if (canScan && (document.activeElement === document.body || document.activeElement?.matches('.show-picker select'))) field.current?.focus();
  }, [canScan]);

  // A subsequent read must not silently acknowledge a changed evaluation.
  useEffect(() => {
    setScans(previous => previous.map(scan => scan.purchaseId && !scan.version && current[scan.purchaseId]
      ? { ...scan, version: current[scan.purchaseId].version } : scan));
  }, [current]);

  function capture() {
    const cert = input.trim();
    setInput('');
    field.current?.focus();
    if (!cert || !inventory.data) return;
    if (scans.some(scan => scan.cert === cert)) { setNotice(`${cert} already scanned.`); return; }
    if (scans.length >= 200) { setNotice('Review or remove scans before scanning more than 200 cards.'); return; }
    const matches = inventory.data.filter(item => item.purchase.certNumber === cert);
    const saved = list.data?.items.filter(item => item.certNumber === cert) ?? [];
    const ambiguous = new Set([...matches.map(item => item.purchase.id), ...saved.map(item => item.purchaseId)]).size > 1;
    const match = !ambiguous ? matches[0]?.purchase : undefined;
    const member = !ambiguous ? saved[0] : undefined;
    setScans(previous => [...previous, { cert, purchaseId: match?.id ?? member?.purchaseId, cardName: match?.cardName ?? member?.cardName,
      version: match ? current[match.id]?.version : undefined, ambiguous }]);
    setNotice(ambiguous ? `${cert}: multiple matches.` : match || member ? `${cert}: scanned.` : `${cert}: no inventory match.`);
    setError('');
  }

  function status(scan: Scan) {
    if (scan.ambiguous) return 'Multiple inventory matches';
    if (!scan.purchaseId) return 'No inventory match';
    if (members.has(scan.purchaseId)) return 'Already on this show list';
    const value = current[scan.purchaseId];
    if (!value) return evaluations.data?.errors[scan.purchaseId] ?? 'Checking availability…';
    if (value.certNumber !== scan.cert) return 'Identity changed. Remove and rescan to review.';
    if (!value.canAdd || !['ready', 'not_received'].includes(value.availability)) return `Unavailable: ${value.availability}`;
    if (!scan.version) return 'Checking availability…';
    if (scan.version !== value.version) return 'Evaluation changed. Remove and rescan to review.';
    return value.availability === 'not_received' ? 'Ready to plan (not received)' : 'Ready to add';
  }
  const ready = scans.filter(scan => status(scan).startsWith('Ready to'));
  async function addReady() {
    if (!ready.length || !canScan || evaluations.isFetching) return;
    setError('');
    try {
      await add.mutateAsync({ id: listId, items: ready.map(scan => ({ purchaseId: scan.purchaseId!, evaluationVersion: scan.version! })) });
      setScans(previous => previous.filter(scan => !ready.includes(scan)));
      setNotice(`Added ${ready.length} ${ready.length === 1 ? 'card' : 'cards'} to show list.`);
      field.current?.focus();
    } catch (err) {
      setError(`${showError(err)} Queue retained. Review current data before retrying.`);
      void list.refetch();
      void evaluations.refetch();
      void inventory.refetch();
    }
  }

  return <section className="show-scanner" aria-label="Scan cards for show">
    <div className="show-scanner-heading"><h2>Scan cards</h2><span className="text-xs text-[var(--text-muted)]">Scan cert barcode, then review before adding.</span></div>
    <label htmlFor="show-scan-input">Scan slab barcode</label>
    <input id="show-scan-input" ref={field} className="show-input" autoComplete="off" inputMode="numeric"
      value={input} disabled={!canScan} placeholder="Scan cert, then Enter" onChange={e => setInput(e.target.value)}
      onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); capture(); } }} />
    {inventory.isLoading || list.isLoading ? <p role="status">Loading inventory and show list…</p> : null}
    {inventory.isError || list.isError ? <p role="alert" className="text-[var(--danger)]">Could not load scan data. <button type="button" className="show-link" onClick={() => { void inventory.refetch(); void list.refetch(); }}>Retry</button></p> : null}
    {notice && <p role="status" className="text-sm text-[var(--text-muted)]">{notice}</p>}
    {ids.some(id => evaluations.data?.errors[id]) && <p className="text-sm text-[var(--warning)]">Some evaluations unavailable. <button type="button" className="show-link" disabled={evaluations.isFetching} onClick={() => void evaluations.refetch()}>Retry evaluations</button></p>}
    {scans.length > 0 && <>
      <div className="show-scanner-heading show-scanner-review"><h3>Review scans <span className="tabular-nums">({scans.length})</span></h3><Button variant="ghost" size="sm" disabled={add.isPending} onClick={() => { setScans([]); setError(''); field.current?.focus(); }}>Clear queue</Button></div>
      <ul className="show-scan-rows">{scans.map(scan => <li key={scan.cert} className="show-scan-row">
        <span className="tabular-nums">{scan.cert}</span><span className="show-scan-name">{scan.cardName ?? 'Unmatched cert'}</span>
        <span className={status(scan).startsWith('Ready to') ? 'text-[var(--success)]' : 'text-[var(--warning)]'}>{status(scan)}</span>
        <Button variant="ghost" size="sm" disabled={add.isPending} aria-label={`Remove scan ${scan.cert}`} onClick={() => { setScans(previous => previous.filter(item => item !== scan)); field.current?.focus(); }}>Remove</Button>
      </li>)}</ul>
      {error && <p role="alert" className="text-sm text-[var(--danger)]">{error}</p>}
      <Button size="sm" disabled={!ready.length || !canScan || evaluations.isFetching} onClick={() => void addReady()}>{add.isPending ? 'Adding…' : `Add ${ready.length} to show`}</Button>
    </>}
  </section>;
}
