export type ScannerMode = 'intake' | 'sale';

export interface SaleRowData {
  certNumber: string;
  status: 'scanning' | 'resolved' | 'error';
  error?: string;

  // Identity
  cardName?: string;
  purchaseId?: string;
  campaignId?: string;
  setName?: string;
  cardNumber?: string;
  cardYear?: string;
  gradeValue?: number;
  frontImageUrl?: string;

  // Reference prices (read-only, cents)
  buyCostCents?: number;
  clValueCents?: number;
  dhListingPriceCents?: number;
  lastSoldCents?: number;

  // Editable pricing (cents)
  compValueCents: number;
  compManuallySet: boolean;
  salePriceCents: number;
  salePriceManuallySet: boolean;
}

export interface SaleSummary {
  cardCount: number;
  compTotalCents: number;
  saleTotalCents: number;
  costTotalCents: number;
  profitCents: number;
  avgDiscountPct: number;
}

export const SALE_COST_VISIBLE_KEY = 'slabledger:sale-cost-visible';
export const SALE_DEFAULT_DISCOUNT_KEY = 'slabledger:sale-default-discount';

export const SALE_CHANNELS = [
  { value: 'cardshow', label: 'Card Show' },
  { value: 'local', label: 'Local' },
  { value: 'ebay', label: 'eBay' },
  { value: 'tcgplayer', label: 'TCGPlayer' },
  { value: 'doubleholo', label: 'DoubleHolo' },
  { value: 'other', label: 'Other' },
] as const;

export function formatCents(cents: number): string {
  return `$${(cents / 100).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}
