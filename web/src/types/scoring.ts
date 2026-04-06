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

export const FACTOR_DISPLAY_NAMES: Record<string, string> = {
  market_trend: 'Market Trend',
  liquidity: 'Liquidity',
  roi_potential: 'ROI Potential',
  scarcity: 'Scarcity',
  market_alignment: 'Market Alignment',
  portfolio_fit: 'Portfolio Fit',
  grade_fit: 'Grade Fit',
  capital_pressure: 'Capital Pressure',
  carrying_cost: 'Carrying Cost',
  crack_advantage: 'Crack Advantage',
  sell_through: 'Sell-Through',
  spend_efficiency: 'Spend Efficiency',
  coverage_impact: 'Coverage Impact',
};
