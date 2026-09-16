import { useLayoutEffect, useRef } from 'react';
import type { useShowReadiness } from '../../queries/useShowReadiness';
import type { ShowPrepWorkerStatus } from '../../../types/showprepWorker';
import { Button } from '../../ui';

type Coverage = Pick<ShowPrepWorkerStatus, 'eligibleIdentities' | 'currentIdentities' | 'missingIdentities' | 'staleIdentities' | 'failedIdentities' | 'eligibleCards' | 'currentCards' | 'unresolvedCards'>;

/** Quiet, server-counted fleet coverage, not a selected-cohort progress task. */
export default function ShowReadinessLine({ readiness: r, coverage: c, coverageError = false, holdFootprint = false }: {
  readiness: ReturnType<typeof useShowReadiness>;
  coverage?: Coverage;
  coverageError?: boolean;
  holdFootprint?: boolean;
}) {
  const element = useRef<HTMLDivElement>(null);
  const height = useRef(0);
  useLayoutEffect(() => {
    if (!holdFootprint || height.current === 0) height.current = element.current?.getBoundingClientRect().height ?? 0;
  });
  // Capture the already-rendered notice, not selection itself. A quiet view
  // stays quiet on the first checkbox; the parent keys this by explicit view.
  const heldHeight = holdFootprint ? height.current : 0;
  const incomplete = c && c.currentCards < c.eligibleCards;
  if (!incomplete && !coverageError && !r.missingPriceCount && !r.observationError && !heldHeight) return null;
  return <div ref={element} style={heldHeight ? { minHeight: heldHeight } : undefined} className="show-readiness show-actions" aria-label="Comp data coverage">
    {coverageError ? <span>Evidence coverage unavailable</span> : c && (incomplete || heldHeight > 0) && <>
      <span>All inventory: {c.currentIdentities}/{c.eligibleIdentities} identities current</span>
      <span>{c.currentCards}/{c.eligibleCards} cards with current evidence</span>
      <span>{c.missingIdentities} missing · {c.staleIdentities} stale · {c.failedIdentities} failed identities</span>
      {c.unresolvedCards > 0 && <span>{c.unresolvedCards} unresolved {c.unresolvedCards === 1 ? 'card' : 'cards'}</span>}
    </>}
    {r.missingPriceCount > 0 && <span>{r.missingPriceCount} no DH price in this view</span>}
    {r.observationError && <><span role="alert">{r.observationError}</span><Button size="sm" variant="secondary" disabled={r.observing} onClick={() => void r.retryObservation()}>Retry price support read</Button></>}
  </div>;
}
