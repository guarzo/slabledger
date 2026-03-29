export interface Signal {
  factor: string;
  direction: 'bullish' | 'bearish' | 'neutral';
  title: string;
  detail: string;
}

export interface Pick {
  id: number;
  date: string;
  card_name: string;
  set_name: string;
  grade: string;
  direction: 'buy' | 'watch' | 'avoid';
  confidence: 'high' | 'medium' | 'low';
  buy_thesis: string;
  target_buy_price: number;
  expected_sell_price: number;
  signals: Signal[];
  rank: number;
  source: 'ai' | 'watchlist_reassessment';
  created_at: string;
}

export interface PicksResponse {
  picks: Pick[];
}

export interface WatchlistItem {
  id: number;
  card_name: string;
  set_name: string;
  grade: string;
  source: 'manual' | 'auto_from_pick';
  active: boolean;
  latest_assessment?: Pick;
  added_at: string;
  updated_at: string;
}

export interface WatchlistResponse {
  items: WatchlistItem[];
}
