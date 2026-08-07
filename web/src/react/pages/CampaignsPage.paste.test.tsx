import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import CampaignsPage from './CampaignsPage';
import { ToastProvider } from '../contexts/ToastContext';
import { APIError } from '../../js/api';
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
// the paste format deliberately does not round-trip targeting. The second block
// matches nothing, so it exercises the create branch and proves the loop keeps
// going after a failure on the first.
const pasteText = [
  'Campaign 1 — Vintage Core',
  'SPORT: Pokemon',
  'BUY TERMS: 80.0%',
  'Daily Spend: $750.00',
  'Campaign 2 — Second Wave',
  'SPORT: Pokemon',
  'BUY TERMS: 70.0%',
  'Daily Spend: $250.00',
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
const createCampaign = vi.fn();

vi.mock('../../js/api', async (orig) => {
  const mod = await orig<typeof import('../../js/api')>();
  return {
    ...mod,
    api: {
      ...mod.api,
      listPSAPushes: vi.fn().mockResolvedValue({ pushes: [] }),
      listPSASubjects: vi.fn().mockResolvedValue({ subjects: [], fetchedAt: '2026-08-01T00:00:00Z' }),
      getCampaign: (...args: unknown[]) => getCampaign(...args),
      updateCampaign: (...args: unknown[]) => updateCampaign(...args),
      createCampaign: (...args: unknown[]) => createCampaign(...args),
    },
  };
});

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
  createCampaign.mockReset();
  updateCampaign.mockResolvedValue(campaign);
  createCampaign.mockResolvedValue({ ...campaign, id: 'c2', name: 'Second Wave' });
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

it('sends the fresh updatedAt as the write precondition', async () => {
  // The advisory comparison above can be overtaken between the GET and the PUT.
  // Only this parameter makes the server compare-and-write in one statement, so
  // its absence would silently reopen the race the guard exists to close.
  getCampaign.mockResolvedValue(campaign);
  await clickPaste();

  await waitFor(() => expect(updateCampaign).toHaveBeenCalled());
  expect(updateCampaign.mock.calls[0][2]).toBe('2026-02-02T00:00:00Z');
});

it('records a 409 as a per-campaign error and keeps importing the rest', async () => {
  // Losing the race on one campaign must not abandon the others in the block —
  // a paste is a batch, and a partial import is the useful outcome.
  getCampaign.mockResolvedValue(campaign);
  updateCampaign.mockRejectedValue(new APIError('Campaign changed since it was loaded', 409));
  await clickPaste();

  await waitFor(() => expect(updateCampaign).toHaveBeenCalled());
  // 'Second Wave' has no matching campaign, so it takes the create branch and
  // must still run after the conflict on 'Vintage Core'.
  await waitFor(() => expect(createCampaign).toHaveBeenCalled());
  expect(createCampaign.mock.calls[0][0].name).toBe('Second Wave');
  expect(await screen.findByText(/changed while the update was in flight/i)).toBeInTheDocument();
});
