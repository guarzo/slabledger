import { useState, useRef } from 'react';
import { Dialog } from 'radix-ui';
import PriceCheckForm from '../PriceCheckForm';
import { SectionErrorBoundary, CardPriceCard, Button } from '../ui';
import type { CardPriceData, CardPrices } from '../ui/CardPriceCard';
import { useUserPreferences } from '../contexts/UserPreferencesContext';
import { useToast } from '../contexts/ToastContext';
import { getErrorMessage } from '../utils/formatters';
import { api } from '../../js/api';
import { reportError } from '../../js/errors';
import type { CardPricingResponse } from '../../types/pricing';

const CARD_SEARCH_LIMIT = 50;

interface Card {
  id?: string;
  name: string;
  setName: string;
  number?: string;
  imageUrl?: string;
  images?: {
    large?: string;
    small?: string;
  };
  marketPrice?: number;
}

export default function PriceLookupDrawer({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [pricesLoading, setPricesLoading] = useState(false);
  const [selectedCard, setSelectedCard] = useState<Card | null>(null);
  const [priceData, setPriceData] = useState<CardPricingResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const { addRecentPriceCheck } = useUserPreferences();
  const toast = useToast();
  const abortControllerRef = useRef<AbortController | null>(null);

  const handleCardSelect = async (card: Card) => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }
    const controller = new AbortController();
    abortControllerRef.current = controller;

    setSelectedCard(card);
    setPriceData(null);
    setPricesLoading(true);
    setError(null);

    try {
      const pricing = await api.getCardPricing(
        card.name || '',
        card.setName || '',
        card.number || '',
        { signal: controller.signal },
      );

      if (controller.signal.aborted) return;

      setPriceData(pricing);
      setPricesLoading(false);

      try {
        addRecentPriceCheck({
          id: card.id || `${card.name}-${card.setName}`,
          name: card.name,
          setName: card.setName,
          number: card.number || '',
          imageUrl: card.imageUrl || card.images?.large || card.images?.small || ''
        });
      } catch (e) {
        reportError('PriceLookupDrawer/saveRecent', e);
      }
    } catch (err) {
      if (controller.signal.aborted) return;

      setPriceData({
        card: card.name || '',
        set: card.setName || '',
        number: card.number || '',
        rawUSD: card.marketPrice || 0,
        psa8: 0,
        psa9: 0,
        psa10: 0,
        confidence: 0,
      });
      setError(getErrorMessage(err, 'Failed to load pricing data'));
      toast.error(getErrorMessage(err, 'Failed to load pricing data'));
      setPricesLoading(false);
    }
  };

  let cardData: CardPriceData | null = null;
  let prices: CardPrices | null = null;

  if (selectedCard) {
    cardData = {
      name: selectedCard.name,
      setName: selectedCard.setName,
      number: selectedCard.number || priceData?.number || '',
      imageUrl: selectedCard.imageUrl || selectedCard.images?.large || selectedCard.images?.small || '',
    };

    if (priceData) {
      prices = {
        raw: priceData.rawUSD || 0,
        psa8: priceData.psa8 || 0,
        psa9: priceData.psa9 || 0,
        psa10: priceData.psa10 || 0,
      };
    }
  }

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/50 backdrop-blur-sm z-50 data-[state=open]:animate-[fadeIn_200ms] data-[state=closed]:animate-[fadeOut_150ms]" />
        <Dialog.Content
          className="fixed top-0 right-0 h-full w-full max-w-md bg-[var(--surface-1)] border-l border-[var(--surface-2)] shadow-xl overflow-y-auto z-50 data-[state=open]:animate-[slideInFromRight_250ms_ease-out] data-[state=closed]:animate-[slideOutToRight_200ms_ease-in]"
        >
          <div className="sticky top-0 bg-[var(--surface-1)] border-b border-[var(--surface-2)] p-4 flex items-center justify-between z-10">
            <Dialog.Title className="text-lg font-semibold text-[var(--text)]">Price Lookup</Dialog.Title>
            <Dialog.Description className="sr-only">Search for card prices by name</Dialog.Description>
            <Dialog.Close asChild>
              <Button variant="ghost" size="icon" aria-label="Close">
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
                  <title>Close</title>
                  <line x1="18" y1="6" x2="6" y2="18" />
                  <line x1="6" y1="6" x2="18" y2="18" />
                </svg>
              </Button>
            </Dialog.Close>
          </div>

          <div className="p-4 space-y-4">
            <PriceCheckForm
              onSearch={(query: string) => api.searchCards(query, CARD_SEARCH_LIMIT)}
              onCardSelect={handleCardSelect}
            />

            <SectionErrorBoundary sectionName="Price Results">
              {error && !priceData && (
                <div className="text-center text-[var(--danger)] py-6 bg-[var(--danger-bg)] border border-[var(--danger-border)] rounded-[var(--radius-lg)]">
                  <p className="text-sm">{error}</p>
                </div>
              )}

              {cardData && (
                <div className="fade-in">
                  <CardPriceCard
                    card={cardData}
                    prices={prices}
                    pricesLoading={pricesLoading}
                    variant="featured"
                    gradeData={priceData?.gradeData}
                    market={priceData?.market}
                    velocity={priceData?.velocity}
                    lastSold={priceData?.lastSold}
                    conservativePrices={{
                      psa10: priceData?.conservativePsa10,
                      psa9: priceData?.conservativePsa9,
                      raw: priceData?.conservativeRaw,
                    }}
                  />
                </div>
              )}

              {!selectedCard && (
                <div className="text-center text-[var(--text-muted)] py-8">
                  <div className="text-4xl mb-2 opacity-40" role="presentation" aria-hidden="true">&#x1f50d;</div>
                  <p className="text-sm">Select a card to see pricing</p>
                </div>
              )}
            </SectionErrorBoundary>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
