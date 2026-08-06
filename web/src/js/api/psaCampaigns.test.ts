import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { APIClient } from './client';
import './psaCampaigns';

describe('APIClient.listPSASubjects', () => {
  let client: APIClient;

  beforeEach(() => {
    client = new APIClient('/api');
    client.maxRetries = 1;
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('GETs /api/psa/subjects and returns the parsed catalog', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        subjects: [{ id: 22210, name: 'Machamp' }],
        fetchedAt: '2026-08-01T00:00:00Z',
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await client.listPSASubjects();

    // GET requests omit `method` entirely (fetch defaults to GET) — unlike
    // this file's POST helpers, which set `method: 'POST'` explicitly.
    // expect.objectContaining({ method: undefined }) does not match an
    // options object that lacks the key at all (vitest 4), so assert
    // directly on the second call argument instead.
    expect(fetchMock.mock.calls[0][0]).toBe('/api/psa/subjects');
    expect(fetchMock.mock.calls[0][1]).not.toHaveProperty('method');
    expect(result).toEqual({
      subjects: [{ id: 22210, name: 'Machamp' }],
      fetchedAt: '2026-08-01T00:00:00Z',
    });
  });
});
