import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { APIClient } from './client';

describe('APIClient error message construction', () => {
  let client: APIClient;

  beforeEach(() => {
    client = new APIClient('/api');
    client.maxRetries = 1; // no retries in tests
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('uses data.error when backend sends {"error": "..."} shape', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 409,
      statusText: 'Conflict',
      json: async () => ({ error: 'Purchase already listed on DH' }),
    }));

    await expect(client.get('/some-endpoint')).rejects.toMatchObject({
      message: 'Purchase already listed on DH',
      status: 409,
    });
  });

  it('falls back to data.message when data.error is absent', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      json: async () => ({ message: 'something went wrong' }),
    }));

    await expect(client.get('/some-endpoint')).rejects.toMatchObject({
      message: 'something went wrong',
      status: 500,
    });
  });

  it('prefers data.error over data.message when both are present', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 409,
      statusText: 'Conflict',
      json: async () => ({ error: 'primary', message: 'secondary' }),
    }));

    await expect(client.get('/some-endpoint')).rejects.toMatchObject({
      message: 'primary',
      status: 409,
    });
  });

  it('falls back to generic message when neither field is present', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 503,
      statusText: 'Service Unavailable',
      json: async () => ({}),
    }));

    await expect(client.get('/some-endpoint')).rejects.toMatchObject({
      message: 'API error: 503 Service Unavailable',
      status: 503,
    });
  });
});
