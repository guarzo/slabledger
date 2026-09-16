import { getShowReadiness } from '../../js/api/showprep';
import type { ReadinessState, ShowEvaluation } from '../../types/showprep';
import { useShowReadinessObservation } from './useShowReadinessObservation';

/** Coverage of saved evidence. Selection and filters are not observation inputs. */
export function useShowReadiness(ids: string[], evaluations: Record<string, ShowEvaluation>) {
  const observation = useShowReadinessObservation(ids, evaluations);
  const values = ids.map(id => evaluations[id]);
  const counts: Record<ReadinessState | 'unknown', number> = {
    not_checked: 0, current: 0, stale: 0, running: 0, interrupted: 0, failed: 0, invalid: 0, unavailable: 0, unknown: 0,
  };
  let needsMatchingCount = 0;
  for (const value of values) {
    const readiness = getShowReadiness(value);
    counts[readiness?.state ?? 'unknown']++;
    if (readiness?.state === 'unavailable' && !readiness.identityKey && (value.availability === 'ready' || value.availability === 'not_received')) needsMatchingCount++;
  }
  return { ...observation, cohortCount: ids.length, currentCount: counts.current, needsMatchingCount,
    incomplete: counts.current < ids.length, counts,
    missingPriceCount: values.filter(e => e && e.listedPriceCents <= 0).length };
}
