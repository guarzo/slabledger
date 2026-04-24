import { useRef, useCallback, useEffect } from 'react';
import { api } from '../../../js/api';
import { reportError } from '../../../js/errors';
import type { ScanCertResponse } from '../../../types/campaigns';
import type { CertRow } from './cardIntakeTypes';
import { rowAwaitingSync, scanFieldsFromResult } from './cardIntakeTypes';

export function useCardIntakePolling(
  certsRef: React.RefObject<Map<string, CertRow>>,
  certsSize: number,
  updateCert: (certNumber: string, updates: Partial<CertRow>) => void,
  resolveInBackground: (certNumber: string) => void,
) {
  const inflightPollsRef = useRef<Set<string>>(new Set());

  const applyPollResult = useCallback((certNumber: string, result: ScanCertResponse) => {
    if (result.status !== 'existing' && result.status !== 'sold') return;
    const current = certsRef.current!.get(certNumber);
    const preserveStatus = current?.status === 'imported' ? 'imported' : result.status;
    const fields = scanFieldsFromResult(result);
    updateCert(certNumber, {
      status: preserveStatus,
      ...fields,
      cardName: fields.cardName ?? current?.cardName,
    });
  }, [certsRef, updateCert]);

  const pollCert = useCallback(async (certNumber: string) => {
    if (inflightPollsRef.current.has(certNumber)) return;
    inflightPollsRef.current.add(certNumber);
    try {
      const result = await api.scanCert(certNumber);
      applyPollResult(certNumber, result);
    } catch {
      // Transient poll failure — next tick will retry.
    } finally {
      inflightPollsRef.current.delete(certNumber);
    }
  }, [applyPollResult]);

  const pollAwaitingCerts = useCallback(async () => {
    const awaiting: string[] = [];
    for (const row of certsRef.current!.values()) {
      if (rowAwaitingSync(row) && !inflightPollsRef.current.has(row.certNumber)) {
        awaiting.push(row.certNumber);
      }
    }
    if (awaiting.length === 0) return;

    awaiting.forEach(c => inflightPollsRef.current.add(c));
    try {
      const batch = await api.scanCerts(awaiting);
      for (const cert of awaiting) {
        const result = batch.results?.[cert];
        if (result) applyPollResult(cert, result);
      }
      if (batch.errors && batch.errors.length > 0) {
        reportError('scan-certs batch', new Error(
          `per-cert errors: ${batch.errors.map(e => `${e.certNumber}: ${e.error}`).join('; ')}`
        ));
      }
    } catch {
      // Transient batch failure — next tick will retry.
    } finally {
      awaiting.forEach(c => inflightPollsRef.current.delete(c));
    }
  }, [certsRef, applyPollResult]);

  // Rehydrate: on mount, fire resolve for stale 'resolving' rows, then batch poll.
  useEffect(() => {
    const current = certsRef.current!;
    for (const row of current.values()) {
      if (row.status === 'resolving') {
        void resolveInBackground(row.certNumber);
      }
    }
    void pollAwaitingCerts();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Polling loop: every 4s, refresh all rows awaiting sync.
  useEffect(() => {
    if (certsSize === 0) return;
    const interval = window.setInterval(() => { void pollAwaitingCerts(); }, 4000);
    return () => window.clearInterval(interval);
  }, [certsSize, pollAwaitingCerts]);

  return { pollCert, pollAwaitingCerts, applyPollResult, inflightPollsRef };
}
