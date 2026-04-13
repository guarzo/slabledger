import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  APIError,
  isAPIError,
} from '../src/js/api';
import type { Campaign } from '../src/types/campaigns';

describe('api module', () => {
  describe('APIError', () => {
    it('creates error with status and code', () => {
      const err = new APIError('test error', 404, 'NOT_FOUND');
      expect(err.message).toBe('test error');
      expect(err.status).toBe(404);
      expect(err.code).toBe('NOT_FOUND');
      expect(err.name).toBe('APIError');
    });

    it('creates error with data payload', () => {
      const data = { error: 'not found', details: { id: '123' } };
      const err = new APIError('test', 404, undefined, data);
      expect(err.data).toEqual(data);
    });

    it('is an instance of Error', () => {
      const err = new APIError('test', 500);
      expect(err instanceof Error).toBe(true);
      expect(err instanceof APIError).toBe(true);
    });
  });

  describe('isAPIError', () => {
    it('returns true for APIError instances', () => {
      expect(isAPIError(new APIError('test', 500))).toBe(true);
    });

    it('returns false for regular errors', () => {
      expect(isAPIError(new Error('test'))).toBe(false);
    });

    it('returns false for non-error values', () => {
      expect(isAPIError('string')).toBe(false);
      expect(isAPIError(null)).toBe(false);
      expect(isAPIError(undefined)).toBe(false);
    });
  });

  describe('APIClient fetch behavior', () => {
    const originalFetch = globalThis.fetch;

    beforeEach(() => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
    });

    afterEach(() => {
      globalThis.fetch = originalFetch;
      vi.useRealTimers();
    });

    it('GET request parses JSON response', async () => {
      const mockCampaigns: Campaign[] = [{ id: '1', name: 'Test', sport: '', yearRange: '', gradeRange: '', priceRange: '', clConfidence: '', buyTermsCLPct: 0, dailySpendCapCents: 0, inclusionList: '', exclusionMode: false, phase: 'pending', psaSourcingFeeCents: 0, ebayFeePct: 0, expectedFillRate: 0, createdAt: '', updatedAt: '' }];
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(mockCampaigns),
        status: 200,
      });

      // Import fresh api instance
      const { api } = await import('../src/js/api');
      const result = await api.listCampaigns();
      expect(globalThis.fetch).toHaveBeenCalled();
      expect(result).toEqual(mockCampaigns);
    });

    it('throws APIError on non-ok response', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: false,
        status: 404,
        statusText: 'Not Found',
        json: () => Promise.resolve({ error: 'Not found' }),
      });

      const { api } = await import('../src/js/api');
      await expect(api.listCampaigns()).rejects.toThrow();
    });
  });
});
