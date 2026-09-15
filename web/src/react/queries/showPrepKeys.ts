export const showPrepKeys = {
  all: ['show-prep'] as const,
  evaluations: ['show-prep', 'evaluations'] as const,
  lists: ['show-prep', 'lists'] as const,
  detail: (id: string) => ['show-prep', 'list', id] as const,
  evidence: (id: string) => ['show-prep', 'evidence', id] as const,
};
