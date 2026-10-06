import { useCallback, useEffect, useRef } from 'react';
import type { RefObject } from 'react';
import { api } from '../../../js/api';
import type { ConfirmReturnRequest, ConfirmedReturnState, DHSaleCheck, ReturnTargetIdentity, ScanCertResponse } from '../../../types/campaigns';
import type { CertRow } from './cardIntakeTypes';
import { returnNeedsDiagnosis } from './cardIntakeTypes';

type ReturnTarget = { certNumber: string; purchaseId: string; request: ConfirmReturnRequest };
type Snapshot = { purchaseId: string; saleId: string | null; target: ReturnTargetIdentity };
type UpdateCert = (cert: string, updates: Partial<CertRow>) => void;

function identity(state: ConfirmedReturnState): ReturnTargetIdentity | null {
  const p = state.purchase;
  if (!p) return null;
  return { dhInventoryId: p.dhInventoryId ?? 0, certNumber: p.certNumber, grader: p.grader ?? 'PSA' };
}

function sameTarget(a: ReturnTargetIdentity | null, b: ReturnTargetIdentity | null): boolean {
  return !!a && !!b && a.dhInventoryId === b.dhInventoryId
    && a.certNumber === b.certNumber && a.grader === b.grader;
}

function requestCompleted(state: ConfirmedReturnState | null, target: ReturnTarget): boolean {
  const operation = state?.operation;
  return !!operation && operation.state === 'completed'
    && state?.purchase?.id === target.purchaseId
    && operation.certNumber === target.certNumber
    && operation.dhInventoryId === target.request.expectedTarget?.dhInventoryId
    && operation.grader === target.request.expectedTarget.grader
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
  const requested = useRef(new Set<string>());
  const snapshots = useRef(new Map<string, Snapshot>());
  const blocked = useRef(new Map<string, string>());
  const versions = useRef(new Map<string, number>());
  const inflight = useRef(new Map<string, Promise<ConfirmedReturnState | null>>());
  const submitting = useRef(false);
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
        if (!snapshots.current.has(key)) {
          const target = identity(state);
          if (target) snapshots.current.set(key, { purchaseId, saleId: state.expectedSaleId, target });
        }
        const snapshot = snapshots.current.get(key);
        if (state.operation?.state === 'completed' && snapshot?.purchaseId === purchaseId
          && state.operation.expectedSaleId === snapshot.saleId
          && state.operation.dhInventoryId === snapshot.target.dhInventoryId
          && state.operation.certNumber === snapshot.target.certNumber
          && state.operation.grader === snapshot.target.grader) blocked.current.delete(key);
        updateCert(cert, {
          ...projection(certsRef.current.get(cert) ?? row, state),
          ...(blocked.current.has(key) && state.operation?.state !== 'pending'
            ? { returnError: blocked.current.get(key) } : {}),
        });
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
      snapshots.current.delete(key);
      blocked.current.delete(key);
      versions.current.set(key, (versions.current.get(key) ?? 0) + 1);
      inflight.current.delete(key);
    }
  }, [certs, refresh]);

  // Poll only the durable projection of an open listing attempt or failed read.
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

  const start = useCallback(async (cert: string, check?: DHSaleCheck) => {
    if (submitting.current) return;
    const row = certsRef.current.get(cert);
    const purchaseId = row?.purchaseId;
    const initial = row?.returnState;
    const key = `${cert}:${purchaseId}`;
    const captured = purchaseId ? snapshots.current.get(key) : undefined;
    if (!purchaseId || !initial || !captured || row?.returnLoading || row.returnStatusError
      || !sameTarget(identity(initial), captured.target)) return;
    const retry = !check && initial.operation?.state === 'pending' ? initial.operation : null;
    if (check && (!check.resolvable || check.status !== 'sold' || initial.sale || initial.operation
      || initial.precedingAttempt || !sameTarget(check.target, captured.target))) return;
    if (!check && !retry && (blocked.current.has(key) || !initial.sale || initial.expectedSaleId !== captured.saleId)) return;
    // Synchronous ref guard prevents a second click during the fresh read as well as POST.
    submitting.current = true;
    const ownsRow = () => alive.current && certsRef.current.get(cert)?.purchaseId === purchaseId;
    updateCert(cert, { returnBusy: true, returnError: undefined });
    try {
      const fresh = await refresh(cert);
      if (!ownsRow() || !fresh) return;
      const freshTarget = identity(fresh);
      const same = fresh.purchase?.id === purchaseId && sameTarget(freshTarget, captured.target)
        && !returnNeedsDiagnosis(fresh);
      const safe = check ? same && !fresh.precedingAttempt && !fresh.sale
          && fresh.expectedSaleId === null && !fresh.operation
        : retry ? same && fresh.operation?.state === 'pending' && fresh.operation.id === retry.id
          && fresh.operation.expectedSaleId === retry.expectedSaleId
        : same && !fresh.precedingAttempt && !!fresh.sale
          && fresh.sale.id === captured.saleId && fresh.expectedSaleId === captured.saleId;
      if (!safe) {
        updateCert(cert, { returnError: 'Return state changed; rescan before retrying.' });
        return;
      }
      const confirmed: ReturnTarget = { certNumber: cert, purchaseId, request: {
        returnConfirmed: true, expectedSaleId: retry ? retry.expectedSaleId : check ? null : captured.saleId,
        expectedTarget: retry ? { dhInventoryId: retry.dhInventoryId, certNumber: retry.certNumber, grader: retry.grader } : captured.target,
        ...(retry ? { operationId: retry.id } : {}),
      } };
      try {
        const result = await api.confirmPurchaseReturn(purchaseId, confirmed.request);
        if (!ownsRow()) return;
        const scan = await api.scanCert(cert);
        if (!ownsRow()) return;
        if (scan.purchaseId && scan.purchaseId !== purchaseId) {
          updateCert(cert, {
            returnBusy: false, returnState: undefined, returnStatusError: undefined,
            returnError: undefined, listingStatus: undefined, listingError: undefined,
          });
          applyScanResult(cert, scan);
          return;
        }
        applyScanResult(cert, scan);
        const latest = await refresh(cert);
        if (ownsRow() && latest && !latest.sale && (result.outcome === 'local' || result.outcome === 'legacy_void')) {
          updateCert(cert, { status: 'returned' });
        }
      } catch (err) {
        const latest = ownsRow() ? await refresh(cert) : null;
        if (ownsRow() && !requestCompleted(latest, confirmed)) {
          const message = latest?.operation?.lastError?.message
            ?? (err instanceof Error ? err.message : 'Return failed');
          if (latest?.operation?.state !== 'pending') blocked.current.set(key, message);
          updateCert(cert, { returnError: message });
        }
      }
    } finally {
      if (ownsRow()) updateCert(cert, { returnBusy: false });
      submitting.current = false;
    }
  }, [certsRef, updateCert, refresh, applyScanResult]);

  return { start, resolve: (cert: string, check: DHSaleCheck) => start(cert, check), refresh };
}
