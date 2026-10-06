import { useCallback, useEffect, useRef, useState } from 'react';
import type { RefObject } from 'react';
import { api } from '../../../js/api';
import type { ConfirmReturnRequest, ConfirmedReturnState, ScanCertResponse } from '../../../types/campaigns';
import type { CertRow } from './cardIntakeTypes';
import { returnNeedsDiagnosis } from './cardIntakeTypes';

type ReturnTarget = { certNumber: string; purchaseId: string; request: ConfirmReturnRequest };
type UpdateCert = (cert: string, updates: Partial<CertRow>) => void;

function requestCompleted(state: ConfirmedReturnState | null, target: ReturnTarget): boolean {
  const operation = state?.operation;
  return !!operation && operation.state === 'completed'
    && state?.purchase?.id === target.purchaseId
    && operation.certNumber === target.certNumber
    && operation.dhInventoryId === state.purchase.dhInventoryId
    && operation.expectedSaleId === target.request.expectedSaleId
    && (!target.request.operationId || operation.id === target.request.operationId);
}

function projection(row: CertRow, state: ConfirmedReturnState): Partial<CertRow> {
  const p = state.purchase;
  if (!p) throw new Error('Return state has no purchase');
  const status = state.sale ? 'sold' : p.dhStatus === 'listed' ? 'existing'
    : !returnNeedsDiagnosis(state) && state.operation?.state === 'completed'
      && state.operation.dhInventoryId === p.dhInventoryId && p.dhStatus === 'in_stock' ? 'returned'
      : row.status === 'sold' || row.status === 'returned' ? 'existing' : row.status;
  return {
    returnState: state, returnLoading: false, returnStatusError: undefined,
    returnError: state.operation?.lastError?.message,
    status, cardName: p.cardName, campaignId: p.campaignId,
    dhInventoryId: p.dhInventoryId, dhCardId: p.dhCardId,
    dhStatus: p.dhStatus, dhPushStatus: p.dhPushStatus,
    listingStatus: state.sale ? undefined : p.dhStatus === 'listed' ? 'listed'
      : row.listingStatus === 'listed' ? undefined : row.listingStatus,
  };
}

export function useConfirmedReturn(
  certsRef: RefObject<Map<string, CertRow>>, certs: Map<string, CertRow>,
  updateCert: UpdateCert, applyScanResult: (cert: string, result: ScanCertResponse) => void,
) {
  const [target, setTarget] = useState<ReturnTarget | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const requested = useRef(new Set<string>());
  const versions = useRef(new Map<string, number>());
  const inflight = useRef(new Map<string, Promise<ConfirmedReturnState | null>>());
  const alive = useRef(true);

  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);

  const refresh = useCallback(async (cert: string): Promise<ConfirmedReturnState | null> => {
    const row = certsRef.current.get(cert);
    const purchaseId = row?.purchaseId;
    if (!row || !purchaseId) return null;
    const key = `${cert}:${purchaseId}`;
    const existing = inflight.current.get(key);
    if (existing) return existing;
    const version = (versions.current.get(key) ?? 0) + 1;
    versions.current.set(key, version);
    const current = () => alive.current && versions.current.get(key) === version
      && certsRef.current.get(cert)?.purchaseId === purchaseId;
    updateCert(cert, { returnLoading: true, returnStatusError: undefined });
    const request = (async () => {
      try {
        const state = await api.getConfirmedReturnState(purchaseId);
        if (!current()) return null;
        if (!state.purchase || state.purchase.id !== purchaseId || state.purchase.certNumber !== cert
          || !Object.prototype.hasOwnProperty.call(state, 'expectedSaleId')) throw new Error('Return state identity changed');
        updateCert(cert, projection(certsRef.current.get(cert) ?? row, state));
        return state;
      } catch (err) {
        if (current()) updateCert(cert, {
          returnLoading: false,
          returnStatusError: err instanceof Error ? err.message : 'Return state unavailable',
        });
        return null;
      }
    })();
    inflight.current.set(key, request);
    void request.then(() => {
      if (inflight.current.get(key) === request) inflight.current.delete(key);
    });
    return request;
  }, [certsRef, updateCert]);

  // Browser queue state is presentation only. Rehydrate every known purchase
  // from durable server state, including rows that ordinary sync polling skips.
  useEffect(() => {
    const present = new Set<string>();
    for (const row of certs.values()) {
      if (!row.purchaseId) continue;
      const key = `${row.certNumber}:${row.purchaseId}`;
      present.add(key);
      if (!requested.current.has(key)) {
        requested.current.add(key);
        void refresh(row.certNumber);
      }
    }
    for (const key of requested.current) {
      if (present.has(key)) continue;
      requested.current.delete(key);
      // A dismissed row can be rescanned with the same cert and purchase.
      // An old GET must not populate that new row when it finally resolves.
      versions.current.set(key, (versions.current.get(key) ?? 0) + 1);
      inflight.current.delete(key);
    }
    if (target && !certs.has(target.certNumber)) setTarget(null);
  }, [certs, refresh, target]);

  // Listing attempts are journaled by scan's background work. Re-read only
  // their durable projection (or a failed projection) while the row is visible;
  // never re-trigger the scan POST or the listing mutation from this timer.
  useEffect(() => {
    if (certs.size === 0) return;
    const timer = window.setInterval(() => {
      for (const row of certsRef.current.values()) {
        if (row.purchaseId && !row.returnLoading && !row.returnBusy
          && (row.returnState?.precedingAttempt?.outcome === 'open' || row.returnStatusError)) {
          void refresh(row.certNumber);
        }
      }
    }, 4000);
    return () => window.clearInterval(timer);
  }, [certs.size, certsRef, refresh]);

  const start = useCallback(async (cert: string) => {
    if (submitting) {
      updateCert(cert, { returnError: 'Another return is still running. Wait for completion, then retry.' });
      return;
    }
    const state = await refresh(cert);
    const row = certsRef.current.get(cert);
    if (!state || !row?.purchaseId || returnNeedsDiagnosis(state)) return;
    const operation = state.operation;
    const retry = operation?.state === 'pending';
    if (operation?.state === 'completed' && !state.sale) return;
    setTarget({
      certNumber: cert, purchaseId: row.purchaseId,
      request: {
        returnConfirmed: true,
        expectedSaleId: retry ? operation.expectedSaleId : state.expectedSaleId,
        ...(retry ? { operationId: operation.id } : {}),
      },
    });
  }, [certsRef, refresh, submitting, updateCert]);

  const submit = useCallback(async () => {
    if (!target || submitting) return;
    const confirmed = target;
    const ownsRow = () => alive.current
      && certsRef.current.get(confirmed.certNumber)?.purchaseId === confirmed.purchaseId;
    if (!ownsRow()) {
      setTarget(null);
      return;
    }
    setSubmitting(true);
    updateCert(confirmed.certNumber, { returnBusy: true, returnError: undefined });
    try {
      const result = await api.confirmPurchaseReturn(confirmed.purchaseId, confirmed.request);
      if (!ownsRow()) return;
      const scan = await api.scanCert(confirmed.certNumber);
      if (!ownsRow()) return;
      if (scan.purchaseId && scan.purchaseId !== confirmed.purchaseId) {
        // A recreated purchase gets its own durable GET on the next render;
        // do not transfer the old owner's busy flag or recovery projection.
        updateCert(confirmed.certNumber, {
          returnBusy: false, returnState: undefined, returnStatusError: undefined,
          returnError: undefined, listingStatus: undefined, listingError: undefined,
        });
        applyScanResult(confirmed.certNumber, scan);
        return;
      }
      applyScanResult(confirmed.certNumber, scan);
      const latest = await refresh(confirmed.certNumber);
      if (ownsRow() && latest && !latest.sale && (result.outcome === 'local' || result.outcome === 'legacy_void')) {
        updateCert(confirmed.certNumber, { status: 'returned' });
      }
    } catch (err) {
      const latest = ownsRow() ? await refresh(confirmed.certNumber) : null;
      // The server may have committed before the response was lost. A fresh
      // completed receipt is historical evidence, not a reason for another POST.
      if (ownsRow() && !requestCompleted(latest, confirmed)) updateCert(confirmed.certNumber, {
        returnError: latest?.operation?.lastError?.message
          ?? (err instanceof Error ? err.message : 'Return failed'),
      });
    } finally {
      if (alive.current) {
        if (ownsRow()) updateCert(confirmed.certNumber, { returnBusy: false });
        setSubmitting(false);
        setTarget(null);
      }
    }
  }, [target, submitting, certsRef, updateCert, refresh, applyScanResult]);

  return { target, submitting, start, submit, refresh, cancel: () => setTarget(null) };
}
