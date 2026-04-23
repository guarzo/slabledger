/**
 * API Client for SlabLedger
 *
 * This barrel module re-exports the full APIClient with all domain methods
 * attached via prototype augmentation. Existing imports like
 *   import { api } from '../js/api'
 * continue to work unchanged.
 */

// Side-effect imports: each file augments APIClient.prototype
import './campaigns';
import './admin';
import './liquidation';

// Re-export everything consumers depend on from client.ts
export {
  APIClient,
  APIError,
  isAPIError,
} from './client';

export type {
  APIRequestOptions,
} from './client';

// Re-export types that the old api.ts re-exported
export type { AllowedEmail, AdminUser } from '../../types/admin';

// Create and export the singleton
import { APIClient } from './client';
export const api = new APIClient();
