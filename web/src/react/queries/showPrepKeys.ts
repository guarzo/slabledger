import type { ShowEvaluation } from '../../types/showprep';

export const showPrepKeys = {
  all: ['show-prep'] as const,
  evaluations: ['show-prep', 'evaluations'] as const,
  lists: ['show-prep', 'lists'] as const,
  detail: (id: string) => ['show-prep', 'list', id] as const,
  evidence: (id: string) => ['show-prep', 'evidence', id] as const,
  pricePreview: (id: string, cents: number | null, saved?: ShowEvaluation) =>
    ['show-prep', 'price-preview', id, cents, saved?.version, saved?.evidenceVersion, saved?.policyVersion] as const,
};
