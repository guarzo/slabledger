import type { ShopifyPriceSyncMatch } from '../../../types/campaigns';

export type SyncFilter = 'all' | 'price_drop' | 'price_increase' | 'no_market_data';
export type SyncSort = 'delta' | 'value' | 'margin' | 'name';

export type CSVFormat = 'shopify' | 'ebay';

export interface CSVRow {
  raw: string[];
  certNumber: string;
  grader: string;
  price: string;
  title: string;
}

export interface ParsedCSV {
  format: CSVFormat;
  headers: string[];
  prefixLines: string[];  // lines before headers (e.g. eBay info line)
  items: CSVRow[];
  certIdx: number;
  priceIdx: number;
}

export type ItemDecision = { action: 'update'; priceCents: number } | { action: 'skip' };

export type Phase = 'upload' | 'review' | 'export';

export type FilterCounts = {
  all: number;
  price_drop: number;
  price_increase: number;
  no_market_data: number;
};

export type SortFn = (a: ShopifyPriceSyncMatch, b: ShopifyPriceSyncMatch) => number;
