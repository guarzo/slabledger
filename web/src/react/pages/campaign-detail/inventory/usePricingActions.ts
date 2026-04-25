import { useState, useCallback } from 'react';
import type { AgingItem, Purchase } from '../../../../types/campaigns';
import type { PriceFlagReason } from '../../../../types/campaigns/priceReview';
import { api } from '../../../../js/api';
import { getErrorMessage } from '../../../utils/formatters';
import { costBasis, bestPrice } from './utils';

interface PricingActionsParams {
  toast: { success: (msg: string) => void; error: (msg: string) => void };
  invalidateInventory: (opts?: { sellSheet?: boolean }) => void;
  onReviewed: () => void;
}

export interface PricingActionsState {
  priceTarget: {
    purchaseId: string;
    cardName: string;
    costBasisCents: number;
    currentPriceCents: number;
    currentOverrideCents?: number;
    currentOverrideSource?: string;
    aiSuggestedCents?: number;
  } | null;
  setPriceTarget: React.Dispatch<React.SetStateAction<PricingActionsState['priceTarget']>>;
  flagTarget: { purchaseId: string; cardName: string; grade: number } | null;
  setFlagTarget: React.Dispatch<React.SetStateAction<PricingActionsState['flagTarget']>>;
  flagSubmitting: boolean;
  hintTarget: { cardName: string; setName: string; cardNumber: string } | null;
  setHintTarget: React.Dispatch<React.SetStateAction<PricingActionsState['hintTarget']>>;
  handleResolveFlag: (flagId: number) => Promise<void>;
  handleFlagSubmit: (reason: PriceFlagReason) => Promise<void>;
  handleSetPrice: (item: AgingItem) => void;
  handlePriceSaved: () => void;
  handleInlinePriceSave: (purchaseId: string, priceCents: number) => Promise<void>;
  handleFixPricing: (purchase: Purchase) => void;
  handleHintSaved: () => void;
}

export function usePricingActions({ toast, invalidateInventory, onReviewed }: PricingActionsParams): PricingActionsState {
  const [priceTarget, setPriceTarget] = useState<PricingActionsState['priceTarget']>(null);
  const [flagTarget, setFlagTarget] = useState<PricingActionsState['flagTarget']>(null);
  const [flagSubmitting, setFlagSubmitting] = useState(false);
  const [hintTarget, setHintTarget] = useState<PricingActionsState['hintTarget']>(null);

  const handleResolveFlag = useCallback(async (flagId: number) => {
    try {
      await api.resolvePriceFlag(flagId);
      toast.success('Flag resolved');
      invalidateInventory();
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to resolve flag'));
    }
  }, [toast, invalidateInventory]);

  const handleFlagSubmit = useCallback(async (reason: PriceFlagReason) => {
    if (!flagTarget) return;
    setFlagSubmitting(true);
    try {
      await api.createPriceFlag(flagTarget.purchaseId, reason);
      toast.success('Price flag submitted');
      setFlagTarget(null);
      onReviewed();
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to submit price flag'));
    } finally {
      setFlagSubmitting(false);
    }
  }, [flagTarget, toast, onReviewed]);

  function handleSetPrice(item: AgingItem) {
    const currentPrice = bestPrice(item);
    setPriceTarget({
      purchaseId: item.purchase.id,
      cardName: item.purchase.cardName,
      costBasisCents: costBasis(item.purchase),
      currentPriceCents: currentPrice,
      currentOverrideCents: item.purchase.overridePriceCents,
      currentOverrideSource: item.purchase.overrideSource,
      aiSuggestedCents: item.purchase.aiSuggestedPriceCents,
    });
  }

  function handlePriceSaved() {
    invalidateInventory({ sellSheet: true });
  }

  const handleInlinePriceSave = useCallback(async (purchaseId: string, priceCents: number) => {
    try {
      await api.setReviewedPrice(purchaseId, priceCents, 'manual');
      toast.success('Price saved');
      invalidateInventory({ sellSheet: true });
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to save price'));
      throw err;
    }
  }, [toast, invalidateInventory]);

  function handleFixPricing(purchase: Purchase) {
    if (!purchase.setName || !purchase.cardNumber) {
      toast.error('Cannot create hint: set name and card number are required');
      return;
    }
    setHintTarget({ cardName: purchase.cardName, setName: purchase.setName, cardNumber: purchase.cardNumber });
  }

  function handleHintSaved() {
    invalidateInventory();
  }

  return {
    priceTarget, setPriceTarget,
    flagTarget, setFlagTarget,
    flagSubmitting,
    hintTarget, setHintTarget,
    handleResolveFlag,
    handleFlagSubmit,
    handleSetPrice,
    handlePriceSaved,
    handleInlinePriceSave,
    handleFixPricing,
    handleHintSaved,
  };
}
