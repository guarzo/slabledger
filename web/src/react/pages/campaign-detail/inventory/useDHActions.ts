import { useState, useEffect, useCallback, useRef } from 'react';
import type { AgingItem, Purchase } from '../../../../types/campaigns';
import { api, isAPIError } from '../../../../js/api';
import { getErrorMessage } from '../../../utils/formatters';

const isAlreadyListedError = (err: unknown): boolean =>
  isAPIError(err) && err.status === 409 && err.data?.error === 'Purchase already listed on DH';

const isEffectiveSuccess = (r: PromiseSettledResult<unknown>): boolean =>
  r.status === 'fulfilled' || (r.status === 'rejected' && isAlreadyListedError(r.reason));

interface DHActionsParams {
  toast: { success: (msg: string) => void; error: (msg: string) => void };
  invalidateInventory: (opts?: { sellSheet?: boolean }) => void;
  items: AgingItem[];
  setSelected: React.Dispatch<React.SetStateAction<Set<string>>>;
}

export interface DHActionsState {
  dhListingInFlight: Set<string>;
  dhListedOptimistic: Set<string>;
  dhRetryInFlight: Set<string>;
  fixMatchTarget: { purchaseId: string; cardName: string; certNumber?: string; currentDHCardId?: number } | null;
  setFixMatchTarget: React.Dispatch<React.SetStateAction<DHActionsState['fixMatchTarget']>>;
  handleApproveDHPush: (purchaseId: string) => Promise<void>;
  handleDismiss: (purchaseId: string) => Promise<void>;
  handleUndismiss: (purchaseId: string) => Promise<void>;
  handleListOnDH: (purchaseId: string) => Promise<void>;
  handleBulkListOnDH: (purchaseIds: string[]) => Promise<void>;
  handleUnmatchDH: (purchase: Purchase) => Promise<void>;
  handleRetryDHMatch: (purchase: Purchase) => Promise<void>;
  handleFixDHMatch: (purchase: Purchase) => void;
  handleFixDHMatchSaved: () => void;
}

export function useDHActions({ toast, invalidateInventory, items, setSelected }: DHActionsParams): DHActionsState {
  const [dhListingInFlight, setDHListingInFlight] = useState<Set<string>>(new Set());
  const [dhListedOptimistic, setDHListedOptimistic] = useState<Set<string>>(new Set());
  const [dhRetryInFlight, setDHRetryInFlight] = useState<Set<string>>(new Set());
  const [fixMatchTarget, setFixMatchTarget] = useState<DHActionsState['fixMatchTarget']>(null);

  // Clear optimistic overrides when fresh data arrives
  useEffect(() => { if (dhListedOptimistic.size > 0) setDHListedOptimistic(new Set()); }, [items]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleApproveDHPush = useCallback(async (purchaseId: string) => {
    try {
      await api.approveDHPush(purchaseId);
      toast.success('DH push approved — will push on next cycle');
      invalidateInventory();
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to approve DH push'));
    }
  }, [toast, invalidateInventory]);

  const handleDismiss = useCallback(async (purchaseId: string) => {
    try {
      await api.dismissDHMatch(purchaseId);
      toast.success('Dismissed from DH listing');
      invalidateInventory();
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to dismiss'));
    }
  }, [toast, invalidateInventory]);

  const handleUndismiss = useCallback(async (purchaseId: string) => {
    try {
      await api.undismissDHMatch(purchaseId);
      toast.success('Restored to DH pipeline');
      invalidateInventory();
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to restore'));
    }
  }, [toast, invalidateInventory]);

  const handleListOnDH = useCallback(async (purchaseId: string) => {
    setDHListingInFlight(prev => new Set(prev).add(purchaseId));
    try {
      await api.listPurchaseOnDH(purchaseId);
      toast.success('Listed on DH');
      setDHListedOptimistic(prev => new Set(prev).add(purchaseId));
      invalidateInventory();
    } catch (err) {
      if (isAlreadyListedError(err)) {
        toast.success('Listed on DH');
        setDHListedOptimistic(prev => new Set(prev).add(purchaseId));
      } else {
        toast.error(getErrorMessage(err, 'Failed to list on DH'));
      }
      invalidateInventory();
    } finally {
      setDHListingInFlight(prev => { const next = new Set(prev); next.delete(purchaseId); return next; });
    }
  }, [toast, invalidateInventory]);

  const handleBulkListOnDH = useCallback(async (purchaseIds: string[]) => {
    if (purchaseIds.length === 0) return;
    setDHListingInFlight(prev => {
      const next = new Set(prev);
      for (const id of purchaseIds) next.add(id);
      return next;
    });
    const CHUNK_SIZE = 5;
    const results: PromiseSettledResult<unknown>[] = [];
    for (let i = 0; i < purchaseIds.length; i += CHUNK_SIZE) {
      const chunk = purchaseIds.slice(i, i + CHUNK_SIZE);
      const chunkResults = await Promise.allSettled(chunk.map(id => api.listPurchaseOnDH(id)));
      results.push(...chunkResults);
      const chunkSucceeded: string[] = [];
      for (let j = 0; j < chunk.length; j++) {
        if (isEffectiveSuccess(chunkResults[j])) chunkSucceeded.push(chunk[j]);
      }
      if (chunkSucceeded.length > 0) {
        setDHListedOptimistic(prev => {
          const next = new Set(prev);
          for (const id of chunkSucceeded) next.add(id);
          return next;
        });
      }
      setDHListingInFlight(prev => {
        const next = new Set(prev);
        for (const id of chunk) next.delete(id);
        return next;
      });
    }
    const succeededIds = purchaseIds.filter((_, i) => isEffectiveSuccess(results[i]));
    const failed = purchaseIds.length - succeededIds.length;
    if (failed === 0) {
      toast.success(`Listed ${succeededIds.length} on DH`);
    } else if (succeededIds.length === 0) {
      toast.error(`Failed to list ${failed} on DH`);
    } else {
      toast.error(`Listed ${succeededIds.length}, ${failed} failed`);
    }
    if (succeededIds.length > 0) {
      setSelected(prev => {
        const next = new Set(prev);
        for (const id of succeededIds) next.delete(id);
        return next;
      });
    }
    invalidateInventory();
  }, [toast, invalidateInventory, setSelected]);

  const handleUnmatchDH = useCallback(async (purchase: Purchase) => {
    try {
      await api.unmatchDH(purchase.id);
      toast.success('DH match removed');
      invalidateInventory();
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to remove DH match'));
    }
  }, [toast, invalidateInventory]);

  const dhRetryInFlightRef = useRef(dhRetryInFlight);
  dhRetryInFlightRef.current = dhRetryInFlight;

  const handleRetryDHMatch = useCallback(async (purchase: Purchase) => {
    if (dhRetryInFlightRef.current.has(purchase.id)) return;
    setDHRetryInFlight(prev => new Set(prev).add(purchase.id));
    try {
      await api.retryDHMatch(purchase.id);
      toast.success('DH match retry succeeded');
      invalidateInventory();
    } catch (err) {
      toast.error(getErrorMessage(err, 'DH match retry failed'));
    } finally {
      setDHRetryInFlight(prev => { const next = new Set(prev); next.delete(purchase.id); return next; });
    }
  }, [toast, invalidateInventory]);

  const handleFixDHMatch = useCallback((purchase: Purchase) => {
    setFixMatchTarget({
      purchaseId: purchase.id,
      cardName: purchase.cardName,
      certNumber: purchase.certNumber,
      currentDHCardId: purchase.dhCardId,
    });
  }, []);

  const handleFixDHMatchSaved = useCallback(() => {
    invalidateInventory();
  }, [invalidateInventory]);

  return {
    dhListingInFlight,
    dhListedOptimistic,
    dhRetryInFlight,
    fixMatchTarget, setFixMatchTarget,
    handleApproveDHPush,
    handleDismiss,
    handleUndismiss,
    handleListOnDH,
    handleBulkListOnDH,
    handleUnmatchDH,
    handleRetryDHMatch,
    handleFixDHMatch,
    handleFixDHMatchSaved,
  };
}
