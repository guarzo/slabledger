import type { ConfirmedReturnState, MarketSnapshot } from '../../../types/campaigns';

export type CertStatus = 'scanning' | 'existing' | 'sold' | 'returned' | 'resolving' | 'resolved' | 'failed' | 'retry' | 'importing' | 'imported';
export type ListingStatus = 'setting-price' | 'listing' | 'listed' | 'list-error';

export interface CertRow {
  certNumber: string;
  status: CertStatus;
  cardName?: string;
  purchaseId?: string;
  campaignId?: string;
  error?: string;
  buyCostCents?: number;
  market?: MarketSnapshot;
  listingStatus?: ListingStatus;
  listingError?: string;
  dhCardId?: number;
  dhInventoryId?: number;
  dhPushStatus?: string;
  dhStatus?: string;
  firstScanAt?: number;
  frontImageUrl?: string;
  setName?: string;
  cardNumber?: string;
  cardYear?: string;
  gradeValue?: number;
  population?: number;
  dhSearchQuery?: string;
  returnState?: ConfirmedReturnState;
  returnLoading?: boolean;
  returnBusy?: boolean;
  returnStatusError?: string;
  returnError?: string;
}

export function hasDHMatch(row: CertRow): boolean {
  return (row.dhCardId ?? 0) > 0 || (row.market?.gradePriceCents ?? 0) > 0;
}

export function hasDHInventory(row: CertRow): boolean {
  return (row.dhInventoryId ?? 0) > 0;
}

export function hasCLPrice(row: CertRow): boolean {
  return (row.market?.clValueCents ?? 0) > 0;
}

export function dhPushStuck(row: CertRow): boolean {
  const s = row.dhPushStatus;
  return s === 'unmatched' || s === 'held' || s === 'dismissed';
}

export function returnNeedsDiagnosis(state: ConfirmedReturnState): boolean {
  const operation = state.operation;
  const p = state.purchase;
  if (operation && (state.awaitingListing || operation.state !== 'completed')
    && (!p || operation.dhInventoryId !== p.dhInventoryId
      || operation.certNumber !== p.certNumber || operation.grader !== (p.grader ?? 'PSA'))) return true;
  if (operation?.state === 'conflicted') return true;
  const attempt = state.precedingAttempt;
  return !!attempt && !(attempt.kind === 'return' && state.operation?.state === 'pending'
    && attempt.operationId === state.operation.id);
}

export function returnActionLabel(row: CertRow): string | null {
  if (!row.purchaseId) return null;
  if (row.returnStatusError) return 'Refresh return state';
  if (row.returnLoading || row.returnBusy) return row.returnBusy ? 'Returning…' : 'Checking return…';
  const state = row.returnState;
  if (state && returnNeedsDiagnosis(state)) return 'Refresh return state';
  if (state?.operation?.state === 'pending') {
    return state.operation.observedReceipt
      && ['completion', 'settlement'].includes(state.operation.lastError?.phase ?? '')
      ? 'Retry completion' : 'Retry return';
  }
  if (state?.operation?.state === 'completed' && !state.sale) return null;
  if (row.status === 'sold') return 'Return';
  if (hasDHInventory(row) && row.dhStatus !== 'listed'
    && ['existing', 'returned', 'imported'].includes(row.status)) return 'Confirm DH return';
  return null;
}

export function rowIsListable(row: CertRow): boolean {
  const state = row.returnState;
  const blocked = !state || row.returnStatusError || row.status === 'sold'
    || row.dhStatus === 'sold' || row.returnBusy || row.returnLoading
    || (state && returnNeedsDiagnosis(state)) || state?.sale || state?.precedingAttempt
    || (state?.operation && state.operation.state !== 'completed');
  return !blocked && !!row.purchaseId && hasDHInventory(row) && hasCLPrice(row);
}

// importErrorStatus maps a per-cert import error to its resulting terminal
// row status: transient (retryable) failures are staged as 'retry' so the
// operator can re-import them, permanent failures become terminal 'failed'.
export function importErrorStatus(err: { retryable?: boolean }): Extract<CertStatus, 'retry' | 'failed'> {
  return err.retryable ? 'retry' : 'failed';
}

export function rowAwaitingSync(row: CertRow): boolean {
  if (row.purchaseId && (!row.returnState || row.returnStatusError)) return false;
  if (row.returnLoading || row.returnBusy || row.returnState?.precedingAttempt
    || (row.returnState?.operation && row.returnState.operation.state !== 'completed')) return false;
  if (row.listingStatus === 'listed') return false;
  if (row.status === 'failed' || row.status === 'retry' || row.status === 'sold') return false;
  if (row.status === 'resolving') return true;
  if ((row.status === 'existing' || row.status === 'returned' || row.status === 'imported') && !rowIsListable(row)) {
    if (dhPushStuck(row)) return false;
    return true;
  }
  return false;
}

export function scanFieldsFromResult(result: {
  cardName?: string; purchaseId?: string; campaignId?: string;
  buyCostCents?: number; market?: MarketSnapshot;
  frontImageUrl?: string; setName?: string; cardNumber?: string;
  cardYear?: string; gradeValue?: number; population?: number;
  dhSearchQuery?: string; dhCardId?: number; dhInventoryId?: number;
  dhPushStatus?: string; dhStatus?: string;
}): Partial<CertRow> {
  return {
    cardName: result.cardName,
    purchaseId: result.purchaseId,
    campaignId: result.campaignId,
    buyCostCents: result.buyCostCents,
    market: result.market,
    frontImageUrl: result.frontImageUrl,
    setName: result.setName,
    cardNumber: result.cardNumber,
    cardYear: result.cardYear,
    gradeValue: result.gradeValue,
    population: result.population,
    dhSearchQuery: result.dhSearchQuery,
    dhCardId: result.dhCardId,
    dhInventoryId: result.dhInventoryId,
    dhPushStatus: result.dhPushStatus,
    dhStatus: result.dhStatus,
  };
}
