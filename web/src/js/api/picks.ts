/**
 * Picks and acquisition watchlist API methods
 */

import type { PicksResponse, WatchlistResponse } from '../../types/picks';
import type { APIClient } from './client';

/* ------------------------------------------------------------------ */
/*  Declaration merging — tells TypeScript about the methods we add   */
/* ------------------------------------------------------------------ */

declare module './client' {
  interface APIClient {
    getPicks(): Promise<PicksResponse>;
    getPickHistory(days?: number): Promise<PicksResponse>;
    getAcquisitionWatchlist(): Promise<WatchlistResponse>;
    addToAcquisitionWatchlist(cardName: string, setName: string, grade: string): Promise<void>;
    removeFromAcquisitionWatchlist(id: number): Promise<void>;
  }
}

/* ------------------------------------------------------------------ */
/*  Prototype implementations                                         */
/* ------------------------------------------------------------------ */

import { APIClient as _APIClient } from './client';
const proto = _APIClient.prototype;

proto.getPicks = async function (this: APIClient): Promise<PicksResponse> {
  return this.get<PicksResponse>('/picks');
};

proto.getPickHistory = async function (this: APIClient, days = 7): Promise<PicksResponse> {
  return this.get<PicksResponse>(`/picks/history?days=${days}`);
};

proto.getAcquisitionWatchlist = async function (this: APIClient): Promise<WatchlistResponse> {
  return this.get<WatchlistResponse>('/picks/watchlist');
};

proto.addToAcquisitionWatchlist = async function (this: APIClient, cardName: string, setName: string, grade: string): Promise<void> {
  const response = await this.fetchWithRetry(
    `${this.baseURL}/picks/watchlist`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ card_name: cardName, set_name: setName, grade: grade }),
    }
  );
  if (response.status === 201 || response.status === 204) return;
  await this.expectNoContent(response);
};

proto.removeFromAcquisitionWatchlist = async function (this: APIClient, id: number): Promise<void> {
  return this.deleteResource(`/picks/watchlist/${id}`);
};
