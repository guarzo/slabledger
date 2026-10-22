import { useCallback, useRef } from 'react';
import type { RefObject } from 'react';
import { api, isAPIError } from '../../../js/api';
import type { ConfirmedReturnState } from '../../../types/campaigns';
import type { CertRow } from './cardIntakeTypes';
import { rowCanRetryListing } from './cardIntakeTypes';

export function useCardIntakeListing(
  certsRef: RefObject<Map<string, CertRow>>,
  updateCert: (cert: string, patch: Partial<CertRow>) => void,
  refreshReturnState: (cert: string) => Promise<ConfirmedReturnState | null>,
) {
  const submitting = useRef(new Set<string>());
  const list = useCallback(async (cert: string, purchaseId: string) => {
    updateCert(cert, { listingStatus: 'listing', listingError: undefined });
    let failed = false;
    try {
      await api.listPurchaseOnDH(purchaseId);
      updateCert(cert, { listingStatus: 'listed' });
    } catch (err) {
      if (isAPIError(err) && err.status === 409 && err.data?.error === 'Purchase already listed on DH') {
        updateCert(cert, {
          dhStatus: 'listed',
          listingStatus: 'list-error',
          listingError: 'SlabLedger records this card as listed; verify live price and channels on DH before retrying.',
        });
        return;
      }
      failed = true;
      updateCert(cert, {
        listingStatus: 'list-error',
        listingError: err instanceof Error ? err.message : 'Listing failed',
      });
    }
    // Reads, never the browser's previous failure, decide retry availability.
    const state = await refreshReturnState(cert);
    // The refresh may have captured the previous render's in-flight status.
    if (failed && state?.purchase?.dhStatus !== 'listed') updateCert(cert, { listingStatus: 'list-error' });
  }, [refreshReturnState, updateCert]);

  const handleSetPriceAndList = useCallback(async (cert: string, priceCents: number, source: string) => {
    const row = certsRef.current.get(cert);
    if (!row?.purchaseId || submitting.current.has(cert)) return;
    if (priceCents <= 0) {
      updateCert(cert, { listingError: 'Enter a valid price before listing.' });
      return;
    }
    submitting.current.add(cert);
    try {
      updateCert(cert, { listingStatus: 'setting-price', listingError: undefined });
      try {
        await api.setReviewedPrice(row.purchaseId, priceCents, source, { manualList: true });
      } catch (err) {
        updateCert(cert, {
          listingStatus: 'list-error',
          listingError: isAPIError(err) && (err.status === 0 || err.status >= 500)
            ? 'Price save unconfirmed; refresh the card before deciding whether to try again.'
            : err instanceof Error ? err.message : 'Failed to set price',
        });
        return;
      }
      await list(cert, row.purchaseId);
    } finally {
      submitting.current.delete(cert);
    }
  }, [certsRef, list, updateCert]);

  const handleRetryListing = useCallback(async (cert: string) => {
    const row = certsRef.current.get(cert);
    if (!row?.purchaseId || !rowCanRetryListing(row) || submitting.current.has(cert)) return;
    submitting.current.add(cert);
    try {
      await list(cert, row.purchaseId);
    } finally {
      submitting.current.delete(cert);
    }
  }, [certsRef, list]);

  return { handleSetPriceAndList, handleRetryListing };
}
