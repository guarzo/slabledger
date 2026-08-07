import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { APIClient } from './client';
import './campaigns';

function okJson(body: unknown) {
  return vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => body });
}

describe('APIClient campaign CRUD URLs', () => {
  let client: APIClient;

  beforeEach(() => {
    client = new APIClient('/api');
    client.maxRetries = 1;
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('getCampaign GETs /api/campaigns/{id} and returns the parsed campaign', async () => {
    const fetchMock = okJson({ id: 'abc', name: 'Test' });
    vi.stubGlobal('fetch', fetchMock);

    const result = await client.getCampaign('abc');

    // GET omits `method` entirely — see psaCampaigns.test.ts for why this is
    // asserted positionally rather than via objectContaining.
    expect(fetchMock.mock.calls[0][0]).toBe('/api/campaigns/abc');
    expect(fetchMock.mock.calls[0][1]).not.toHaveProperty('method');
    expect(result).toEqual({ id: 'abc', name: 'Test' });
  });

  // Ids are server-generated, so encoding is hygiene rather than a live bug —
  // but it is applied uniformly across the campaign surface, and these pin
  // that down so a future hand-rolled id shape cannot silently break routing.
  it.each([
    ['getCampaign', (c: APIClient, id: string) => c.getCampaign(id)],
    ['deleteCampaign', (c: APIClient, id: string) => c.deleteCampaign(id)],
    ['updateCampaign', (c: APIClient, id: string) => c.updateCampaign(id, {})],
  ])('%s percent-encodes the id path segment', async (_name, call) => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({}),
      text: async () => '',
    });
    vi.stubGlobal('fetch', fetchMock);

    await call(client, 'a/b c');

    expect(fetchMock.mock.calls[0][0]).toBe('/api/campaigns/a%2Fb%20c');
  });
});
