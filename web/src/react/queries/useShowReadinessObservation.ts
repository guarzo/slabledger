import { useCallback, useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { getShowReadiness, type InventoryEvaluations } from '../../js/api/showprep';
import type { ShowEvaluation } from '../../types/showprep';
import { getShowRefreshCoordinator } from './showRefreshCoordinator';
import { showPrepKeys } from './showPrepKeys';

type Observation = 'activation' | 'focus' | 'expiry' | 'retry';
const BOUNDARY_RECHECK_MS = 30000;

/** Read-only show-scoped clock/visibility observation. No refresh POST here. */
export function useShowReadinessObservation(active: boolean, ids: string[], evaluations: Record<string, ShowEvaluation>) {
  const qc = useQueryClient();
  const coordinator = getShowRefreshCoordinator(qc);
  const [observing, setObserving] = useState(false);
  const [observationError, setObservationError] = useState('');
  const [revision, setRevision] = useState(0);
  const pending = useRef(false);
  const alive = useRef(false);
  const lastRead = useRef(-Infinity);
  const latest = useRef({ ids, evaluations }); latest.current = { ids, evaluations };
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  const boundaries = useCallback(() => {
    const midnight = new Date(); midnight.setUTCHours(24, 0, 0, 0);
    const result = [{ key: `utc:${midnight.toISOString()}`, at: midnight.getTime(), retry: false, identityKey: '' }];
    for (const id of latest.current.ids) {
      const r = getShowReadiness(latest.current.evaluations[id]);
      if (!r || (r.state !== 'running' && r.state !== 'current')) continue;
      // Stale/interrupted are observed outcomes, not unresolved boundaries.
      const retry = r.state === 'running';
      const time = retry ? r.retryAt : r.expiresAt;
      result.push({ key: `${retry ? 'retry' : 'expiry'}:${r.identityKey}:${time}`, at: Date.parse(time), retry, identityKey: r.identityKey });
    }
    return result;
  }, []);
  const read = useCallback(async (reason: Observation, retryIdentity?: string) => {
    if (document.visibilityState === 'hidden' || pending.current || (reason === 'focus' && Date.now() - lastRead.current < 1000)) return;
    if (reason !== 'activation') lastRead.current = Date.now();
    pending.current = true; setObserving(true);
    const due = boundaries().filter(b => b.at <= Date.now());
    if (reason !== 'retry') {
      // Independent activation/focus/expiry reads can reconsider this cohort.
      // Selection/cohort changes alone do not authorize the observed identity.
      for (const id of latest.current.ids) {
        const r = getShowReadiness(latest.current.evaluations[id]);
        if (r) coordinator.readOnlyIdentities.delete(r.identityKey);
      }
    } else {
      // A coincident peer expiry is still permission to renew that peer only.
      for (const b of due) if (!b.retry) coordinator.readOnlyIdentities.delete(b.identityKey);
    }
    if (reason === 'retry' || reason === 'expiry') {
      if (retryIdentity) coordinator.readOnlyIdentities.add(retryIdentity);
      // Protect all due attempts, irrespective of which coincident timer won.
      for (const b of due) if (b.retry) coordinator.readOnlyIdentities.add(b.identityKey);
    }
    const elapsed = reason === 'activation' ? [] : due;
    for (const b of elapsed) coordinator.boundaryRecheckAt.set(b.key, Date.now() + BOUNDARY_RECHECK_MS);
    try {
      // Coincident observation reads join the aggregate rather than canceling it.
      await qc.invalidateQueries({ queryKey: showPrepKeys.evaluations }, { cancelRefetch: false, throwOnError: true });
      const failed = qc.getQueriesData<InventoryEvaluations>({ queryKey: showPrepKeys.evaluations, type: 'active' })
        .some(([, data]) => latest.current.ids.some(id => data?.errors[id]));
      if (alive.current) setObservationError(failed ? 'Could not read current price support. Retry the read.' : '');
    } catch {
      if (alive.current) setObservationError('Could not read current price support. Retry the read.');
    } finally {
      // A browser ahead of the server can read the same running/current value.
      // Follow up at a bounded cadence until metadata resolves or supersedes it.
      for (const b of elapsed) coordinator.boundaryRecheckAt.set(b.key, Date.now() + BOUNDARY_RECHECK_MS);
      pending.current = false;
      if (alive.current) { setObserving(false); setRevision(value => value + 1); }
    }
  }, [qc, coordinator, boundaries]);
  useEffect(() => { if (active) void read('activation'); }, [active, read]);
  useEffect(() => {
    if (!active) return;
    const wake = () => { if (document.visibilityState !== 'hidden') void read('focus'); };
    window.addEventListener('focus', wake); document.addEventListener('visibilitychange', wake);
    return () => { window.removeEventListener('focus', wake); document.removeEventListener('visibilitychange', wake); };
  }, [active, read]);
  useEffect(() => {
    if (!active || observing || document.visibilityState === 'hidden') return;
    const next = boundaries().map(b => ({ ...b, at: Math.max(b.at, coordinator.boundaryRecheckAt.get(b.key) ?? 0) })).sort((a, b) => a.at - b.at)[0];
    if (!next) return;
    // Never spin on past bounds or catch up missed ticks. New attempt metadata
    // supersedes old timers; unresolved state gets a cooldown, not permanent retirement.
    const timer = setTimeout(() => { void read(next.retry ? 'retry' : 'expiry', next.retry ? next.identityKey : undefined); }, Math.max(1000, Math.min(86400000, next.at - Date.now())));
    return () => clearTimeout(timer);
  }, [active, ids, evaluations, revision, observing, boundaries, coordinator, read]);
  return { observing, revision, pending, observationError, retryObservation: () => read('activation') };
}
