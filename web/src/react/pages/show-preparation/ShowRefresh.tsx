import { useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { showPrepAPI } from '../../../js/api/showprep';
import { showPrepKeys } from '../../queries/useShowPrepQueries';
import { Button } from '../../ui';
import { showError } from './showPrepLabels';

export default function ShowRefresh({ purchaseIds, disabled = false }: { purchaseIds: string[]; disabled?: boolean }) {
  const qc = useQueryClient();
  const controller = useRef<AbortController | null>(null);
  const retryIds = useRef<string[]>([]);
  const [progress, setProgress] = useState<{ done: number; total: number; review: string[] } | null>(null);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState('');
  useEffect(() => () => controller.current?.abort(), []);
  async function refresh(ids: string[]) {
    if (controller.current) return;
    const selected = [...new Set(ids)];
    if (selected.length === 0) return;
    retryIds.current = selected;
    const abort = new AbortController(); controller.current = abort;
    setRunning(true); setError('');
    let done = 0;
    const review: string[] = [];
    setProgress({ done, total: selected.length, review });
    try {
      // Sequential batches keep upstream pressure bounded and make cancellation
      // meaningful: cancelling does not leave a queue of source writes running.
      for (let i = 0; i < selected.length; i += 10) {
        if (abort.signal.aborted) throw new Error('Cancelled');
        const batch = selected.slice(i, i + 10);
        const result = await showPrepAPI.refresh(batch, abort.signal);
        for (const id of batch) {
          const value = result.evaluations.find(e => e?.purchaseId === id);
          if (!value || value.status === 'needs_review') review.push(`${value?.certNumber || id}: ${value?.reason || 'Evaluation missing from refresh response'}`);
        }
        done += batch.length;
        retryIds.current = selected.slice(i + 10);
        setProgress({ done, total: selected.length, review: [...review] });
        void qc.invalidateQueries({ queryKey: showPrepKeys.all });
      }
    } catch (err) {
      setError(abort.signal.aborted ? 'Cancelled. Completed batches remain saved; unfinished evidence needs review.' : showError(err));
    } finally {
      controller.current = null; setRunning(false);
      void qc.invalidateQueries({ queryKey: showPrepKeys.all });
    }
  }
  return <div className="text-sm">
    <div className="show-actions">
      <Button variant="secondary" size="sm" disabled={disabled || running || purchaseIds.length === 0} onClick={() => void refresh(purchaseIds)}>
        Refresh selected evidence ({purchaseIds.length})
      </Button>
      {running && <Button variant="ghost" size="sm" onClick={() => controller.current?.abort()}>Cancel refresh</Button>}
      {error && !running && <Button variant="secondary" size="sm" disabled={disabled || retryIds.current.length === 0} onClick={() => void refresh(retryIds.current)}>Retry refresh</Button>}
    </div>
    {progress && <p role="status" className="mt-2 text-[var(--text-muted)] tabular-nums">{progress.done} of {progress.total} checked{running ? ', refreshing…' : ''} · {progress.review.length} need review</p>}
    {progress && progress.review.length > 0 && <ul className="text-[var(--warning)] mt-1">{progress.review.map(message => <li key={message}>{message}</li>)}</ul>}
    {error && <p role="alert" className="text-[var(--danger)] mt-2">{error}</p>}
  </div>;
}
