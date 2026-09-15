import { useEffect, useState } from 'react';
import type { ShowEvaluation } from '../../../types/showprep';
import { useShowRefreshState } from '../../queries/useShowReadiness';
import { Button } from '../../ui';

export default function ShowRefresh({ purchaseIds, disabled = false, evaluations = {}, compact = false }: {
  purchaseIds: string[]; disabled?: boolean; evaluations?: Record<string, ShowEvaluation>; compact?: boolean;
}) {
  const { coordinator, busy, blocked, phase, done, total, review, error, remaining } = useShowRefreshState();
  const [owner] = useState(() => Symbol('manual-show-refresh'));
  useEffect(() => { coordinator.attach(owner); return () => coordinator.detach(owner); }, [coordinator, owner]);
  const refresh = (ids: string[]) => coordinator.check(ids.map(purchaseId => evaluations[purchaseId] ?? { purchaseId }), owner, false);
  return <div className="text-sm">
    <div className="show-actions">
      <Button variant="secondary" size="sm" disabled={disabled || busy || blocked || purchaseIds.length === 0} onClick={() => void refresh(purchaseIds)}>
        {compact ? `Check selected (${purchaseIds.length})` : `Refresh selected evidence (${purchaseIds.length})`}
      </Button>
      {busy && <Button variant="ghost" size="sm" onClick={() => coordinator.cancel()}>Cancel refresh</Button>}
      {error && !busy && <Button variant="secondary" size="sm" disabled={disabled || blocked || remaining.length === 0} onClick={() => void refresh(remaining)}>Retry refresh</Button>}
    </div>
    {!compact && phase !== 'idle' && <p role="status" className="mt-2 text-[var(--text-muted)] tabular-nums">{done} of {total} checked{busy ? ', refreshing…' : ''} · {review.length} need review</p>}
    {!compact && review.length > 0 && <ul className="text-[var(--warning)] mt-1">{review.map((message, index) => <li key={index}>{message}</li>)}</ul>}
    {!compact && error && <p role="alert" className="text-[var(--danger)] mt-2">{error}</p>}
  </div>;
}
