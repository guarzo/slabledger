import type { Phase, SaleChannel, CreateCampaignInput } from '../../types/campaigns';

export const DEFAULT_SALE_CHANNEL: SaleChannel = 'ebay';

export const saleChannelLabels: Record<SaleChannel, string> = {
  ebay: 'eBay',
  tcgplayer: 'TCGPlayer',
  local: 'Local',
  other: 'Other',
  gamestop: 'GameStop',
  website: 'Website',
  cardshow: 'Card Show',
};

export const saleChannelColors: Record<SaleChannel, string> = {
  ebay: 'bg-blue-500',
  tcgplayer: 'bg-purple-500',
  local: 'bg-green-500',
  other: 'bg-gray-500',
  gamestop: 'bg-red-500',
  website: 'bg-indigo-500',
  cardshow: 'bg-amber-500',
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
