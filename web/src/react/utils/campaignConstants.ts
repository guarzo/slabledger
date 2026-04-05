import type { Phase, SaleChannel, CreateCampaignInput } from '../../types/campaigns';

export const DEFAULT_SALE_CHANNEL: SaleChannel = 'ebay';

/** Channels available for recording new sales. */
export const activeSaleChannels: SaleChannel[] = ['ebay', 'website', 'inperson'];

/** Maps any channel (including legacy) to its display label. */
export const saleChannelLabels: Record<SaleChannel, string> = {
  ebay: 'eBay',
  website: 'Website',
  inperson: 'In Person',
  // Legacy channels — displayed for historical data
  tcgplayer: 'eBay',
  local: 'In Person',
  other: 'In Person',
  gamestop: 'In Person',
  cardshow: 'In Person',
  doubleholo: 'In Person',
};

/** Normalizes a legacy channel to one of the 3 active channels. */
export function normalizeChannel(ch: SaleChannel): SaleChannel {
  switch (ch) {
    case 'ebay':
    case 'tcgplayer':
      return 'ebay';
    case 'website':
      return 'website';
    default:
      return 'inperson';
  }
}

export const saleChannelColors: Record<SaleChannel, string> = {
  ebay: 'bg-blue-500',
  website: 'bg-indigo-500',
  inperson: 'bg-green-500',
  // Legacy channels map to their normalized color
  tcgplayer: 'bg-blue-500',
  local: 'bg-green-500',
  other: 'bg-green-500',
  gamestop: 'bg-green-500',
  cardshow: 'bg-green-500',
  doubleholo: 'bg-green-500',
};

export const phaseColors: Record<Phase, string> = {
  pending: 'bg-amber-500',
  active: 'bg-green-500',
  closed: 'bg-gray-400',
};

export const campaignTabs = [
  { id: 'overview', label: 'Overview' },
  { id: 'transactions', label: 'Transactions' },
  { id: 'tuning', label: 'Tuning' },
  { id: 'settings', label: 'Settings' },
] as const;

export type CampaignTabId = typeof campaignTabs[number]['id'];

export const phaseOptions = [
  { value: 'pending', label: 'Pending' },
  { value: 'active', label: 'Active' },
  { value: 'closed', label: 'Closed' },
] as const;

export const defaultCampaignInput: CreateCampaignInput = {
  name: '',
  sport: 'Pokemon',
  yearRange: '',
  gradeRange: '',
  priceRange: '',
  clConfidence: '',
  buyTermsCLPct: 0.78,
  dailySpendCapCents: 50000,
  inclusionList: '',
  exclusionMode: false,
  psaSourcingFeeCents: 300,
  ebayFeePct: 0.1235,
};
