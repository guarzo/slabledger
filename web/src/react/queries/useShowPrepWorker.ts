import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { showPrepWorkerAPI } from '../../js/api/showprepWorker';

export const showPrepWorkerKeys = {
  all: ['show-prep-worker'] as const,
  status: ['show-prep-worker', 'status'] as const,
  coverage: ['show-prep-worker', 'coverage'] as const,
};

export function useShowPrepWorkerStatus(enabled = true) {
  return useQuery({ queryKey: showPrepWorkerKeys.status, queryFn: ({ signal }) => showPrepWorkerAPI.status({ signal }),
    enabled, staleTime: 5_000, refetchInterval: 5_000 });
}

/** A single fleet-level read. No checkbox, filter, or selected cohort is an input. */
export function useShowPrepCoverage(enabled = true) {
  return useQuery({ queryKey: showPrepWorkerKeys.coverage, queryFn: ({ signal }) => showPrepWorkerAPI.coverage({ signal }),
    enabled, staleTime: 60_000, refetchInterval: 60_000 });
}

export function useRequestShowPrepRun() {
  const qc = useQueryClient();
  return useMutation({ mutationFn: showPrepWorkerAPI.request, retry: false,
    onSuccess: () => { void qc.invalidateQueries({ queryKey: showPrepWorkerKeys.all }); } });
}
