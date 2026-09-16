import { useCallback, useEffect, useRef, useState } from 'react';
import { useQueryClient, type QueryClient } from '@tanstack/react-query';
import { getShowReadiness, type InventoryEvaluations } from '../../js/api/showprep';
import type { ShowEvaluation } from '../../types/showprep';
import { showPrepKeys } from './showPrepKeys';

const BOUNDARY_RECHECK_MS = 30000;
// Only read cooldowns survive route remounts, scoped to the authenticated cache.
const readCooldowns = new WeakMap<QueryClient, Map<string, number>>();

/** Focus and evidence-expiry reads only. Initial data is owned by the query. */
export function useShowReadinessObservation(ids: string[], evaluations: Record<string, ShowEvaluation>) {
  const qc = useQueryClient();
  const [observing, setObserving] = useState(false);
  const [observationError, setObservationError] = useState('');
  const [revision, setRevision] = useState(0);
  const pending = useRef(false);
  const alive = useRef(false);
  const lastRead = useRef(-Infinity);
  let boundaryRecheckAt = readCooldowns.get(qc);
  if (!boundaryRecheckAt) { boundaryRecheckAt = new Map(); readCooldowns.set(qc, boundaryRecheckAt); }
  const latest = useRef({ ids, evaluations }); latest.current = { ids, evaluations };
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  const boundaries = useCallback(() => {
    const midnight = new Date(); midnight.setUTCHours(24, 0, 0, 0);
    const result = [{ key: `utc:${midnight.toISOString()}`, at: midnight.getTime() }];
    for (const id of latest.current.ids) {
      const r = getShowReadiness(latest.current.evaluations[id]);
      if (!r || (r.state !== 'running' && r.state !== 'current')) continue;
      const time = r.state === 'running' ? r.retryAt : r.expiresAt;
      result.push({ key: `${r.identityKey}:${time}`, at: Date.parse(time) });
    }
    return result;
  }, []);
  const read = useCallback(async () => {
    if (document.visibilityState === 'hidden' || pending.current || Date.now() - lastRead.current < 1000) return;
    lastRead.current = Date.now();
    pending.current = true; setObserving(true);
    const due = boundaries().filter(b => b.at <= Date.now());
    try {
      await qc.invalidateQueries({ queryKey: showPrepKeys.evaluations }, { cancelRefetch: false, throwOnError: true });
      const failed = qc.getQueriesData<InventoryEvaluations>({ queryKey: showPrepKeys.evaluations, type: 'active' })
        .some(([, data]) => latest.current.ids.some(id => data?.errors[id]));
      if (alive.current) setObservationError(failed ? 'Could not read current price support.' : '');
    } catch {
      if (alive.current) setObservationError('Could not read current price support.');
    } finally {
      // A browser ahead of the server must not spin on an unresolved boundary.
      for (const b of due) boundaryRecheckAt.set(b.key, Date.now() + BOUNDARY_RECHECK_MS);
      pending.current = false;
      if (alive.current) { setObserving(false); setRevision(value => value + 1); }
    }
  }, [qc, boundaries, boundaryRecheckAt]);
  useEffect(() => {
    const wake = () => { void read(); };
    window.addEventListener('focus', wake); document.addEventListener('visibilitychange', wake);
    return () => { window.removeEventListener('focus', wake); document.removeEventListener('visibilitychange', wake); };
  }, [read]);
  useEffect(() => {
    if (!ids.length || observing || document.visibilityState === 'hidden') return;
    const next = boundaries().map(b => Math.max(b.at, boundaryRecheckAt.get(b.key) ?? 0)).sort((a, b) => a - b)[0];
    const timer = setTimeout(() => { void read(); }, Math.max(1000, Math.min(86400000, next - Date.now())));
    return () => clearTimeout(timer);
  }, [ids, evaluations, revision, observing, boundaries, read, boundaryRecheckAt]);
  return { observing, observationError, retryObservation: read };
}
