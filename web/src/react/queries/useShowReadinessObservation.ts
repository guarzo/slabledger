import { useCallback, useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { getShowReadiness } from '../../js/api/showprep';
import type { ShowEvaluation } from '../../types/showprep';
import { getShowRefreshCoordinator } from './showRefreshCoordinator';
import { showPrepKeys } from './showPrepKeys';

type Observation = 'activation' | 'focus' | 'expiry' | 'retry';

/** Read-only show-scoped clock/visibility observation. No refresh POST here. */
export function useShowReadinessObservation(active: boolean, ids: string[], evaluations: Record<string, ShowEvaluation>) {
  const qc = useQueryClient();
  const coordinator = getShowRefreshCoordinator(qc);
  const [observing, setObserving] = useState(false);
  const [revision, setRevision] = useState(0);
  const pending = useRef(false);
  const allowAutomatic = useRef(true);
  const alive = useRef(false);
  const lastRead = useRef(-Infinity);
  const latest = useRef({ ids, evaluations }); latest.current = { ids, evaluations };
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  const boundaries = useCallback(() => {
    const midnight = new Date(); midnight.setUTCHours(24, 0, 0, 0);
    const result = [{ key: `utc:${midnight.toISOString()}`, at: midnight.getTime(), retry: false }];
    for (const id of latest.current.ids) {
      const r = getShowReadiness(latest.current.evaluations[id]);
      if (!r) continue;
      // Interrupted retains retryAt for explanation, not another wakeup.
      const time = r.state === 'running' ? r.retryAt : r.expiresAt;
      if (time) result.push({ key: `${r.identityKey}:${time}`, at: Date.parse(time), retry: r.state === 'running' });
    }
    return result;
  }, []);
  const read = useCallback(async (reason: Observation) => {
    if (document.visibilityState === 'hidden' || pending.current || (reason === 'focus' && Date.now() - lastRead.current < 1000)) return;
    if (reason !== 'activation') lastRead.current = Date.now();
    pending.current = true; allowAutomatic.current = reason !== 'retry'; setObserving(true);
    if (reason !== 'activation') for (const b of boundaries()) if (b.at <= Date.now()) coordinator.observedBoundaries.add(b.key);
    try {
      // Coincident observation reads join the aggregate rather than canceling it.
      await qc.invalidateQueries({ queryKey: showPrepKeys.evaluations }, { cancelRefetch: false });
    } finally {
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
    const next = boundaries().filter(b => !coordinator.observedBoundaries.has(b.key)).sort((a, b) => a.at - b.at)[0];
    if (!next) return;
    // Current-at-exact-expiry and skewed past bounds get one delayed observation,
    // never an immediate timer loop. New attempt metadata supersedes old timers.
    const timer = setTimeout(() => { void read(next.retry ? 'retry' : 'expiry'); }, Math.max(1000, Math.min(86400000, next.at - Date.now())));
    return () => clearTimeout(timer);
  }, [active, ids, evaluations, revision, observing, boundaries, coordinator, read]);
  return { observing, revision, pending, allowAutomatic };
}
