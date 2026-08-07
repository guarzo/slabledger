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

export const phaseHexColors: Record<Phase, string> = {
  active: '#059669',
  pending: '#f59e0b',
  closed: '#4b5563',
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

/** Token stored in `Campaign.subjectFilterMode` / `CreateCampaignInput.subjectFilterMode`. */
export const SUBJECT_FILTER_TARGET = 'Target';
export const SUBJECT_FILTER_EXCLUDE = 'Exclude';

/**
 * Closed set of curated-spec-list language tokens. There is no "unset" entry:
 * unset is the empty array, expressed by checking nothing.
 *
 * DUPLICATED, deliberately. The same set lives in
 * internal/domain/inventory/validation.go, internal/domain/psacampaign/resolver.go
 * and cmd/psa-harvest/baseline.go. psacampaign imports inventory, so inventory
 * can never import psacampaign, and this copy is across a process boundary
 * anyway. A new token must be added to all four.
 */
export const targetLanguageOptions = [
  { value: 'english', label: 'English' },
  { value: 'japanese', label: 'Japanese' },
] as const;

/**
 * Mirrors Go's inventory.LegacyUnreconciledSubjectID. Migration 000023 backfills
 * pre-axis subjects with this id to mean "legacy, not yet reconciled with the
 * portal". Distinct from id 0, which means "operator typed this name, resolve it
 * by name at push time" — push translation resolves 0 and refuses -1.
 */
export const LEGACY_UNRECONCILED_SUBJECT_ID = -1;

export const subjectFilterModeOptions: { value: 'Target' | 'Exclude'; label: string }[] = [
  { value: SUBJECT_FILTER_TARGET, label: 'Target' },
  { value: SUBJECT_FILTER_EXCLUDE, label: 'Exclude' },
];

export const defaultCampaignInput: CreateCampaignInput = {
  name: '',
  sport: 'Pokemon',
  yearRange: '',
  gradeRange: '',
  priceRange: '',
  clConfidence: '',
  buyTermsCLPct: 0.78,
  dailySpendCapCents: 50000,
  targetLanguages: [],
  subjectFilterMode: SUBJECT_FILTER_TARGET,
  subjects: [],
  deniedSpecs: [],
  psaSourcingFeeCents: 300,
  ebayFeePct: 0.1235,
};
