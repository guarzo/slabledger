/** Pricing types for the card pricing API response */

export type GradeKey = 'raw' | 'psa1' | 'psa2' | 'psa3' | 'psa4' | 'psa5' | 'psa6' | 'psa7' | 'psa8' | 'psa9' | 'psa10';

/** Price hint mapping for manual price corrections */
export interface PriceHint {
  cardName: string;
  setName: string;
  cardNumber: string;
  provider: 'doubleholo';
  externalId: string;
}
