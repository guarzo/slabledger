import { replaceEqualDeep, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { evaluateInventory, showPrepAPI, type InventoryEvaluations } from '../../js/api/showprep';
import type { ShowItemAdd, ShowItemUpdate, ShowListDetail } from '../../types/showprep';

export const showPrepKeys = {
  all: ['show-prep'] as const,
  evaluations: ['show-prep', 'evaluations'] as const,
  lists: ['show-prep', 'lists'] as const,
  detail: (id: string) => ['show-prep', 'list', id] as const,
  evidence: (id: string) => ['show-prep', 'evidence', id] as const,
};

type InventoryRead = InventoryEvaluations & { observedAtStart?: InventoryEvaluations['evaluations'] };

function preserveDetailObservations(previous: unknown, incoming: unknown): InventoryEvaluations {
  const old = previous as InventoryEvaluations | undefined;
  const read = incoming as InventoryRead;
  if (!('observedAtStart' in read)) return replaceEqualDeep(old, read);
  const evaluations = { ...read.evaluations };
  const errors = { ...read.errors };
  // Query.setData runs structuralSharing at the actual cache-write boundary.
  // Immutable records changed since this read started came from a detail write;
  // preserve them even if a slow, older batch finishes afterward. Later reads
  // start from those same records and are free to advance them normally.
  for (const [id, value] of Object.entries(old?.evaluations ?? {})) {
    if (value !== read.observedAtStart?.[id]) {
      evaluations[id] = value;
      delete errors[id];
    }
  }
  // Strip the request-only baseline instead of retaining chains of old results.
  return replaceEqualDeep(old, { evaluations, errors });
}

function unresolvedEvaluations(ids: string[], data?: InventoryEvaluations): number {
  return ids.filter(id => !data?.evaluations[id] && !data?.errors[id]).length;
}

export function useShowEvaluations(ids: string[]) {
  const qc = useQueryClient();
  const sorted = [...new Set(ids)].sort();
  const query = useQuery<InventoryEvaluations>({
    queryKey: [...showPrepKeys.evaluations, sorted],
    queryFn: async ({ signal, queryKey }) => {
      const observedAtStart = qc.getQueryData<InventoryEvaluations>(queryKey)?.evaluations;
      const result = await evaluateInventory(sorted, signal);
      return { ...result, observedAtStart };
    },
    structuralSharing: preserveDetailObservations,
    // Detail publication is not completion of the aggregate. Cancellation keeps
    // that partial data, so it must remain stale and resume on the next mount.
    staleTime: query => unresolvedEvaluations(sorted, query.state.data) > 0 ? 0 : 30000,
    enabled: sorted.length > 0, retry: false,
  });
  return { ...query, unresolvedCount: unresolvedEvaluations(sorted, query.data) };
}
export function useShowEvidence(id: string, enabled: boolean, evaluationVersion = '') {
  const qc = useQueryClient();
  return useQuery({
    queryKey: [...showPrepKeys.evidence(id), evaluationVersion], enabled, staleTime: 30000, retry: false,
    queryFn: async ({ signal }) => {
      const data = await showPrepAPI.evidence(id, { signal });
      if (data.evaluation.version !== evaluationVersion) {
        qc.setQueryData([...showPrepKeys.evidence(id), data.evaluation.version], data);
      }
      // A detail read can see newer inputs. Publish its evaluation so the compact
      // badge and subsequent add intent refer to the same evidence the operator saw.
      qc.setQueriesData<InventoryEvaluations>({
        queryKey: showPrepKeys.evaluations,
        predicate: query => Array.isArray(query.queryKey[2]) && query.queryKey[2].includes(id),
      }, old => {
        const errors = { ...old?.errors }; delete errors[id];
        return { evaluations: { ...old?.evaluations, [id]: data.evaluation }, errors };
      });
      void qc.invalidateQueries({ predicate: query => {
        if (query.queryKey[0] !== 'show-prep' || query.queryKey[1] !== 'list') return false;
        const old = query.state.data as ShowListDetail | undefined;
        return !!old?.items.some(item => item.purchaseId === id && item.evaluation?.version !== data.evaluation.version);
      } });
      return data;
    },
  });
}
export function useShowLists() {
  return useQuery({ queryKey: showPrepKeys.lists, queryFn: showPrepAPI.lists, staleTime: 30000, retry: false });
}
export function useShowList(id: string) {
  return useQuery({ queryKey: showPrepKeys.detail(id), queryFn: () => showPrepAPI.detail(id), enabled: !!id, staleTime: 0, retry: false });
}
export function useShowListWrites() {
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: showPrepKeys.all });
  return {
    create: useMutation({ mutationFn: ({ id, name }: { id: string; name: string }) => showPrepAPI.createList(id, name), onSettled: invalidate }),
    rename: useMutation({ mutationFn: ({ id, name }: { id: string; name: string }) => showPrepAPI.renameList(id, name), onSettled: invalidate }),
    add: useMutation({ mutationFn: ({ id, items }: { id: string; items: ShowItemAdd[] }) => showPrepAPI.addItems(id, items), onSettled: invalidate }),
    update: useMutation({ mutationFn: ({ id, itemId, input }: { id: string; itemId: string; input: ShowItemUpdate }) => showPrepAPI.updateItem(id, itemId, input), onSettled: invalidate }),
    remove: useMutation({ mutationFn: ({ id, itemId }: { id: string; itemId: string }) => showPrepAPI.removeItem(id, itemId), onSettled: invalidate }),
  };
}
