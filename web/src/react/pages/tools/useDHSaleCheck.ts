import { useCallback, useEffect, useRef, useState } from 'react';
import type { RefObject } from 'react';
import { api } from '../../../js/api';
import type { DHSaleCheck } from '../../../types/campaigns';
import type { CertRow } from './cardIntakeTypes';

type CheckState = { loading: boolean; check?: DHSaleCheck; error?: string };
type Entry = { identity: string; state: CheckState };

function rowIdentity(row: CertRow | undefined): string | null {
  const p = row?.returnState?.purchase;
  if (!row?.purchaseId || row.returnStatusError || !p || p.id !== row.purchaseId
    || p.certNumber !== row.certNumber) return null;
  const state = row.returnState;
  return JSON.stringify([p.id, p.certNumber, p.grader ?? 'PSA', p.dhInventoryId,
    state?.expectedSaleId, state?.operation?.id, state?.operation?.state,
    state?.precedingAttempt?.id, state?.sale?.id, state?.awaitingListing]);
}

export function useDHSaleCheck(certsRef: RefObject<Map<string, CertRow>>, certs: Map<string, CertRow>) {
  const [entries, setEntries] = useState<Map<string, Entry>>(new Map());
  const versions = useRef(new Map<string, number>());
  const identities = useRef(new Map<string, string>());
  const alive = useRef(true);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);

  const clear = useCallback((cert: string) => {
    versions.current.set(cert, (versions.current.get(cert) ?? 0) + 1);
    setEntries(prev => {
      if (!prev.has(cert)) return prev;
      const next = new Map(prev);
      next.delete(cert);
      return next;
    });
  }, []);

  useEffect(() => {
    const present = new Set<string>();
    for (const [cert, row] of certs) {
      const identity = rowIdentity(row);
      if (!identity) continue;
      present.add(cert);
      if (identities.current.has(cert) && identities.current.get(cert) !== identity) clear(cert);
      identities.current.set(cert, identity);
    }
    for (const cert of identities.current.keys()) {
      if (present.has(cert)) continue;
      identities.current.delete(cert);
      clear(cert);
    }
  }, [certs, clear]);

  const check = useCallback(async (cert: string) => {
    const row = certsRef.current.get(cert);
    const identity = rowIdentity(row);
    if (!row?.purchaseId || !identity) return;
    const purchaseId = row.purchaseId;
    const version = (versions.current.get(cert) ?? 0) + 1;
    versions.current.set(cert, version);
    const current = () => alive.current && versions.current.get(cert) === version
      && rowIdentity(certsRef.current.get(cert)) === identity;
    setEntries(prev => new Map(prev).set(cert, { identity, state: { loading: true } }));
    try {
      const result = await api.getDHSaleCheck(purchaseId);
      if (!current()) return;
      const p = certsRef.current.get(cert)?.returnState?.purchase;
      if (result.resolvable && (!p || result.target.dhInventoryId !== p.dhInventoryId
        || result.target.certNumber !== p.certNumber || result.target.grader !== (p.grader ?? 'PSA'))) {
        throw new Error('DH sale check target changed; rescan before retrying.');
      }
      setEntries(prev => new Map(prev).set(cert, { identity, state: { loading: false, check: result } }));
    } catch (err) {
      if (current()) setEntries(prev => new Map(prev).set(cert, { identity, state: {
        loading: false, error: err instanceof Error ? err.message : 'DH sale check failed',
      } }));
    }
  }, [certsRef]);

  const stateFor = (cert: string, purchaseId: string): CheckState | undefined => {
    const row = certsRef.current.get(cert);
    const identity = rowIdentity(row);
    const entry = entries.get(cert);
    return row?.purchaseId === purchaseId && identity && entry?.identity === identity ? entry.state : undefined;
  };
  return { check, stateFor, clear };
}
