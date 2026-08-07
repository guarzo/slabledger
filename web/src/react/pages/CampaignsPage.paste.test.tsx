import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import CampaignsPage from './CampaignsPage';
import { ToastProvider } from '../contexts/ToastContext';
import { api } from '../../js/api';
import type { Campaign } from '../../types/campaigns';

const campaign: Campaign = {
  id: 'c1',
  name: 'Vintage Core',
  sport: 'Pokemon',
  yearRange: '1999-2003',
  gradeRange: '8-10',
  priceRange: '50-500',
  clConfidence: 'high',
  buyTermsCLPct: 0.78,
  dailySpendCapCents: 50000,
  targetLanguages: ['english'],
  subjectFilterMode: 'Target',
  subjects: [{ id: 22210, name: 'Machamp' }, { id: 4807, name: 'Charizard' }],
  deniedSpecs: [],
  phase: 'active',
  psaSourcingFeeCents: 300,
  ebayFeePct: 0.1235,
  expectedFillRate: 42,
  psaCampaignRequestId: 'req-1',
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-02-02T00:00:00Z',
} as Campaign;

// Matches the campaign above by name, and carries only scalar economics —
// the paste format deliberately does not round-trip targeting.
const pasteText = [
  'Campaign 1 — Vintage Core',
  'SPORT: Pokemon',
  'BUY TERMS: 80.0%',
  'Daily Spend: $750.00',
].join('\n');

vi.mock('../queries/useCampaignQueries', async (orig) => {
  const mod = await orig<typeof import('../queries/useCampaignQueries')>();
  return {
    ...mod,
    useCampaigns: () => ({ data: [campaign], isLoading: false }),
    usePortfolioHealth: () => ({ data: undefined }),
    useCreateCampaign: () => ({ mutateAsync: vi.fn(), isPending: false }),
    useUpdateCampaign: () => ({ mutateAsync: vi.fn(), isPending: false }),
  };
});

const getCampaign = vi.fn();
const updateCampaign = vi.fn();

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <MemoryRouter>
      <QueryClientProvider client={qc}>
        <ToastProvider>
          <CampaignsPage />
        </ToastProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  getCampaign.mockReset();
  updateCampaign.mockReset();
  updateCampaign.mockResolvedValue(campaign);
  // Spying on the real singleton rather than spreading it: `api` is an
  // APIClient instance whose endpoint methods live on the prototype, so
  // `{ ...mod.api }` copies none of them and every endpoint this file does not
  // name becomes undefined. Stubbing fetchWithRetry — the single choke point
  // behind get/post/put/deleteResource — keeps an unstubbed endpoint off the
  // network. The two local vi.fn()s stay the assertion handles; the spies just
  // delegate to them, so the mockResolvedValue calls in each test still read
  // the same way.
  vi.spyOn(api, 'fetchWithRetry').mockRejectedValue(
    new Error('unstubbed API call — add a vi.spyOn for this endpoint'),
  );
  vi.spyOn(api, 'listPSAPushes').mockResolvedValue({ pushes: [] });
  vi.spyOn(api, 'listPSASubjects').mockResolvedValue({ subjects: [], fetchedAt: '2026-08-01T00:00:00Z' });
  vi.spyOn(api, 'getCampaign').mockImplementation(getCampaign);
  vi.spyOn(api, 'updateCampaign').mockImplementation(updateCampaign);
});

afterEach(() => {
  vi.restoreAllMocks();
});

async function clickPaste() {
  // userEvent.setup() installs its own navigator.clipboard stub, so the paste
  // text has to be planted after setup, not in beforeEach.
  const user = userEvent.setup();
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { readText: vi.fn().mockResolvedValue(pasteText) },
  });
  renderPage();
  await user.click(screen.getByRole('button', { name: /import campaigns from clipboard/i }));
  return user;
}

it('preserves expectedFillRate through a paste update', async () => {
  // expectedFillRate is operator-owned, not server-owned: campaign_store.go
  // binds c.ExpectedFillRate verbatim into the full-row UPDATE and nothing in
  // service_crud.go derives it, so stripping it here wrote 0 on every paste.
  getCampaign.mockResolvedValue(campaign);
  await clickPaste();

  await waitFor(() => expect(updateCampaign).toHaveBeenCalled());
  const [id, data] = updateCampaign.mock.calls[0];
  expect(id).toBe('c1');
  expect(data.expectedFillRate).toBe(42);
  // Unlinking the campaign from the portal is the other full-row hazard.
  expect(data.psaCampaignRequestId).toBe('req-1');
});

it('carries targeting through untouched rather than from the paste text', async () => {
  // The paste format has no targeting lines by design; targeting survives only
  // because the full row is echoed back, with portal ids copied verbatim.
  getCampaign.mockResolvedValue(campaign);
  await clickPaste();

  await waitFor(() => expect(updateCampaign).toHaveBeenCalled());
  const [, data] = updateCampaign.mock.calls[0];
  expect(data.subjects).toEqual([{ id: 22210, name: 'Machamp' }, { id: 4807, name: 'Charizard' }]);
  expect(data.targetLanguages).toEqual(['english']);
  expect(data.subjectFilterMode).toBe('Target');
});

it('still applies the fields the paste text does carry', async () => {
  // Guards the spread order: `{ ...fresh, ...input }` reversed would silently
  // drop every pasted value while still reporting the campaign as updated.
  getCampaign.mockResolvedValue(campaign);
  await clickPaste();

  await waitFor(() => expect(updateCampaign).toHaveBeenCalled());
  const [, data] = updateCampaign.mock.calls[0];
  expect(data.buyTermsCLPct).toBeCloseTo(0.8);
  expect(data.dailySpendCapCents).toBe(75000);
});

it('checks staleness over the network rather than from cached campaign data', async () => {
  getCampaign.mockResolvedValue(campaign);
  await clickPaste();

  await waitFor(() => expect(getCampaign).toHaveBeenCalledWith('c1'));
});

it('skips the update when the campaign changed since the list loaded', async () => {
  // The racing writer is the psa-harvest baseline pull; writing here would put
  // the cached row's targeting ids back over the newer baseline.
  getCampaign.mockResolvedValue({ ...campaign, updatedAt: '2026-03-03T00:00:00Z' });
  await clickPaste();

  await waitFor(() => expect(getCampaign).toHaveBeenCalled());
  expect(updateCampaign).not.toHaveBeenCalled();
  expect(await screen.findByText(/harvester baseline pull/i)).toBeInTheDocument();
});

it('skips the update when the staleness check itself fails', async () => {
  // Fail closed, same as the edit form.
  getCampaign.mockRejectedValue(new Error('network down'));
  await clickPaste();

  await waitFor(() => expect(getCampaign).toHaveBeenCalled());
  expect(updateCampaign).not.toHaveBeenCalled();
  expect(await screen.findByText(/could not confirm the campaign is unchanged/i)).toBeInTheDocument();
});
