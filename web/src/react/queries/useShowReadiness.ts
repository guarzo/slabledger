import { useEffect, useState, useSyncExternalStore } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { getShowReadiness } from '../../js/api/showprep';
import type { ReadinessState, ShowEvaluation } from '../../types/showprep';
import { getShowRefreshCoordinator } from './showRefreshCoordinator';
import { useShowReadinessObservation } from './useShowReadinessObservation';

export function useShowRefreshState() {
  const coordinator = getShowRefreshCoordinator(useQueryClient());
  const state = useSyncExternalStore(coordinator.subscribe, coordinator.getSnapshot, coordinator.getSnapshot);
  return { coordinator, ...state };
}
export interface ShowReadinessInput {
  active: boolean;
  cohortIds: string[];
  evaluations: Record<string, ShowEvaluation>;
  fetching: boolean;
  selectedCount: number;
}

/** Acquisition is opt-in at this feature boundary, never at the query/provider. */
export function useShowReadiness({ active, cohortIds, evaluations, fetching, selectedCount }: ShowReadinessInput) {
  const refresh = useShowRefreshState();
  const { coordinator, busy, blocked } = refresh;
  const [owner] = useState(() => Symbol('show-readiness'));
  const [visible, setVisible] = useState(() => document.visibilityState !== 'hidden');
  const { observing, revision, pending, allowAutomatic } = useShowReadinessObservation(active, cohortIds, evaluations);
  useEffect(() => { coordinator.attach(owner); return () => coordinator.detach(owner); }, [coordinator, owner]);
  useEffect(() => {
    coordinator.pause(owner, selectedCount > 0 || !visible || observing);
    return () => coordinator.pause(owner, false);
  }, [coordinator, owner, selectedCount, visible, observing]);

  useEffect(() => {
    const visibility = () => {
      const next = document.visibilityState !== 'hidden';
      if (!next) coordinator.pause(owner, true);
      setVisible(next);
    };
    document.addEventListener('visibilitychange', visibility);
    return () => document.removeEventListener('visibilitychange', visibility);
  }, [coordinator, owner]);

  useEffect(() => {
    coordinator.setCohort(owner, active ? cohortIds.flatMap(id => evaluations[id] ? [evaluations[id]] : []) : []);
  }, [active, cohortIds, evaluations, coordinator, owner]);

  useEffect(() => {
    if (!active || fetching || observing || pending.current || busy || blocked || selectedCount || !visible || document.visibilityState === 'hidden' || !allowAutomatic.current) return;
    void coordinator.check(cohortIds.flatMap(id => evaluations[id] ? [evaluations[id]] : []), owner, true);
  }, [active, fetching, observing, busy, blocked, selectedCount, visible, cohortIds, evaluations, coordinator, owner, revision, pending, allowAutomatic]);
  const values = cohortIds.map(id => evaluations[id]);
  const counts: Record<ReadinessState | 'unknown', number> = {
    not_checked: 0, current: 0, stale: 0, running: 0, interrupted: 0, failed: 0, invalid: 0, unavailable: 0, unknown: 0,
  };
  for (const value of values) counts[getShowReadiness(value)?.state ?? 'unknown']++;
  const current = counts.current;
  const retry = values.some(e => getShowReadiness(e)?.refreshEligibility === 'retry_only');
  return { ...refresh, cohortCount: cohortIds.length, currentCount: current,
    incomplete: current < cohortIds.length, retry, counts,
    missingPriceCount: values.filter(e => e && e.listedPriceCents <= 0).length,
    cancel: () => coordinator.cancel(),
    pauseSelection: (paused: boolean) => coordinator.pause(owner, paused),
    check: () => coordinator.check(cohortIds.filter(id => getShowReadiness(evaluations[id])?.state !== 'current').map(purchaseId => evaluations[purchaseId] ?? { purchaseId }), owner, false),
  };
}
