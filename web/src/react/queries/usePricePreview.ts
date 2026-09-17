import { useQuery } from '@tanstack/react-query';
import { priceReviewAPI } from '../../js/api/priceReview';
import { showPrepKeys } from './showPrepKeys';
import type { ShowEvaluation } from '../../types/showprep';

export function usePricePreview(purchaseId: string, draftCents: number | null, savedEvaluation?: ShowEvaluation) {
  return useQuery({
    queryKey: showPrepKeys.pricePreview(purchaseId, draftCents, savedEvaluation),
    enabled: !!purchaseId && !!savedEvaluation && draftCents !== null
      && Number.isSafeInteger(draftCents) && draftCents > 0
      && draftCents !== savedEvaluation.localPriceCents,
    queryFn: ({ signal }) => priceReviewAPI.preview(purchaseId, draftCents!, { signal }),
    retry: false,
    staleTime: 0,
  });
}
