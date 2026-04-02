export type Verdict =
  | 'strong_buy' | 'buy' | 'lean_buy'
  | 'hold'
  | 'lean_sell' | 'sell' | 'strong_sell';

export interface Factor {
  name: string;
  value: number;
  confidence: number;
  source: string;
}

export interface DataGap {
  factor: string;
  reason: string;
}

export interface ScoreCard {
  entity_id: string;
  entity_type: string;
  factors: Factor[];
  composite: number;
  confidence: number;
  engine_verdict: Verdict;
  data_gaps: DataGap[];
  mixed_signals?: boolean;
  scored_at: string;
}
