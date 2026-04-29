/**
 * Types for the PSA-Exchange acquisition feed.
 * Mirrors the JSON returned by GET /api/psa-exchange/opportunities.
 * All money fields are dollars (the Go handler converts from cents).
 */

export interface PsaExchangeOpportunity {
  cert: string;
  name: string;
  description: string;
  grade: string;
  listPrice: number;
  targetOffer: number;
  maxOfferPct: number;
  comp: number;
  lastSalePrice: number;
  lastSaleDate: string;        // ISO timestamp
  velocityMonth: number;
  velocityQuarter: number;
  confidence: number;
  population: number;
  edgeAtOffer: number;
  score: number;
  listRunwayPct: number;
  mayTakeAtList: boolean;
  frontImage: string;
  backImage: string;
  indexId: string;
  tier: string;
}

export interface PsaExchangeOpportunitiesResponse {
  opportunities: PsaExchangeOpportunity[];
  categoryUrl: string;
  fetchedAt: string;
  totalCatalogPokemon: number;
  afterFilter: number;
  enrichmentErrors: number;
}
