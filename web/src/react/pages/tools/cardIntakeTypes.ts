import type { MarketSnapshot } from '../../../types/campaigns';

export type CertStatus = 'scanning' | 'existing' | 'sold' | 'returned' | 'resolving' | 'resolved' | 'failed' | 'importing' | 'imported';
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

export function rowIsListable(row: CertRow): boolean {
  return !!row.purchaseId && hasDHInventory(row) && hasCLPrice(row);
}

export function rowAwaitingSync(row: CertRow): boolean {
  if (row.listingStatus === 'listed') return false;
  if (row.status === 'failed' || row.status === 'sold') return false;
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
