/**
 * Liquidation-related API methods
 */

import type { LiquidationPreviewResponse, LiquidationApplyItem, LiquidationApplyResult } from '../../types/liquidation';
import type { APIClient } from './client';

/* ------------------------------------------------------------------ */
/*  Declaration merging — tells TypeScript about the methods we add   */
/* ------------------------------------------------------------------ */

declare module './client' {
  interface APIClient {
    getLiquidationPreview(discountWithCompsPct: number, discountNoCompsPct: number): Promise<LiquidationPreviewResponse>;
    applyLiquidation(items: LiquidationApplyItem[]): Promise<LiquidationApplyResult>;
  }
}

/* ------------------------------------------------------------------ */
/*  Prototype implementations                                         */
/* ------------------------------------------------------------------ */

import { APIClient as _APIClient } from './client';
const proto = _APIClient.prototype;

proto.getLiquidationPreview = async function (this: APIClient, discountWithCompsPct: number, discountNoCompsPct: number): Promise<LiquidationPreviewResponse> {
  return this.post<LiquidationPreviewResponse>('/liquidation/preview', { discountWithCompsPct, discountNoCompsPct });
};

proto.applyLiquidation = async function (this: APIClient, items: LiquidationApplyItem[]): Promise<LiquidationApplyResult> {
  return this.post<LiquidationApplyResult>('/liquidation/apply', { items });
};
