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

const updateMutateAsync = vi.fn().mockResolvedValue(campaign);

vi.mock('../queries/useCampaignQueries', async (orig) => {
  const mod = await orig<typeof import('../queries/useCampaignQueries')>();
  return {
    ...mod,
    useCampaigns: () => ({ data: [campaign], isLoading: false }),
    usePortfolioHealth: () => ({ data: undefined }),
    useCreateCampaign: () => ({ mutateAsync: vi.fn(), isPending: false }),
    useUpdateCampaign: () => ({ mutateAsync: updateMutateAsync, isPending: false }),
  };
});

const getCampaign = vi.fn();

vi.mock('../../js/api', async (orig) => {
  const mod = await orig<typeof import('../../js/api')>();
  return {
    ...mod,
    api: {
      ...mod.api,
      listPSAPushes: vi.fn().mockResolvedValue({ pushes: [] }),
      listPSASubjects: vi.fn().mockResolvedValue({ subjects: [], fetchedAt: '2026-08-01T00:00:00Z' }),
      getCampaign: (...args: unknown[]) => getCampaign(...args),
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
  updateMutateAsync.mockReset();
  updateMutateAsync.mockResolvedValue(campaign);
  getCampaign.mockReset();
});

async function openEditAndSave() {
  const user = userEvent.setup();
  renderPage();
  await user.click(screen.getByRole('button', { name: /edit vintage core/i }));
  await user.click(await screen.findByRole('button', { name: /save changes/i }));
  return user;
}

it('sends the full campaign so a full-row PUT cannot blank server-owned fields', async () => {
  getCampaign.mockResolvedValue(campaign);
  await openEditAndSave();

  await waitFor(() => expect(updateMutateAsync).toHaveBeenCalled());
  const { id, data } = updateMutateAsync.mock.calls[0][0];
  expect(id).toBe('c1');
  // Omitting either of these from a full-row UPDATE writes its zero value:
  // psaCampaignRequestId silently unlinks the campaign from the portal, and
  // expectedFillRate zeroes an analytics input.
  expect(data.psaCampaignRequestId).toBe('req-1');
  // Seeded from the campaign by toFormValues (not absent, not zeroed by the
  // onChange-commits-immediately field) and preserved through the save spread.
  expect(data.expectedFillRate).toBe(42);
});

it('round-trips existing portal subject ids byte-for-byte', async () => {
  getCampaign.mockResolvedValue(campaign);
  await openEditAndSave();

  await waitFor(() => expect(updateMutateAsync).toHaveBeenCalled());
  const { data } = updateMutateAsync.mock.calls[0][0];
  expect(data.subjects).toEqual([{ id: 22210, name: 'Machamp' }, { id: 4807, name: 'Charizard' }]);
  expect(data.targetLanguages).toEqual(['english']);
  expect(data.subjectFilterMode).toBe('Target');
});

it('checks staleness over the network rather than from cached campaign data', async () => {
  // useCampaigns holds data fresh for 30s and cannot observe a write made by
  // the psa-harvest process, so the guard must actually hit the server.
  getCampaign.mockResolvedValue(campaign);
  await openEditAndSave();

  await waitFor(() => expect(getCampaign).toHaveBeenCalledWith('c1'));
});

it('aborts the save when the campaign changed since the form opened', async () => {
  getCampaign.mockResolvedValue({ ...campaign, updatedAt: '2026-03-03T00:00:00Z' });
  await openEditAndSave();

  await waitFor(() => expect(getCampaign).toHaveBeenCalled());
  expect(updateMutateAsync).not.toHaveBeenCalled();
  expect(await screen.findByText(/harvester baseline pull/i)).toBeInTheDocument();
  // The form stays open so the operator does not lose their edits.
  expect(screen.getByRole('button', { name: /save changes/i })).toBeInTheDocument();
});

it('aborts the save when the staleness check itself fails', async () => {
  // Fail closed: saving anyway would reintroduce the race the check closes.
  getCampaign.mockRejectedValue(new Error('network down'));
  await openEditAndSave();

  await waitFor(() => expect(getCampaign).toHaveBeenCalled());
  expect(updateMutateAsync).not.toHaveBeenCalled();
  expect(await screen.findByText(/could not confirm/i)).toBeInTheDocument();
});

it('sends the fresh updatedAt as the write precondition', async () => {
  // The comparison in the previous test is advisory: it can be overtaken between
  // the GET and the PUT. This parameter is what makes the server compare-and-write
  // in a single statement, so dropping it silently reopens the race.
  getCampaign.mockResolvedValue(campaign);
  await openEditAndSave();

  await waitFor(() => expect(updateMutateAsync).toHaveBeenCalled());
  expect(updateMutateAsync.mock.calls[0][0].ifUnmodifiedSince).toBe('2026-02-02T00:00:00Z');
});

it('reports a 409 as a conflict rather than a generic failure', async () => {
  // The row moved inside the GET→PUT window, so the advisory check passed and
  // only the server could catch it. The operator needs the same "nothing was
  // saved, re-open Edit" guidance as the pre-flight rejection.
  getCampaign.mockResolvedValue(campaign);
  updateMutateAsync.mockRejectedValue(new APIError('Campaign changed since it was loaded', 409));
  await openEditAndSave();

  await waitFor(() => expect(updateMutateAsync).toHaveBeenCalled());
  expect(await screen.findByText(/changed while the save was in flight/i)).toBeInTheDocument();
  // The form stays open so the operator does not lose their edits.
  expect(screen.getByRole('button', { name: /save changes/i })).toBeInTheDocument();
});

it('sends the operator-edited name, not just the pre-edit snapshot', async () => {
  // All the other tests here save without touching a field, so
  // `{ ...fresh, ...values }` and `{ ...values, ...fresh }` at
  // CampaignsPage.tsx's save handler produce identical payloads either way.
  // This test changes a field first, so reversing that spread order — which
  // would silently discard every operator edit while still reporting success
  // — turns this assertion red.
  getCampaign.mockResolvedValue(campaign);
  const user = userEvent.setup();
  renderPage();
  await user.click(screen.getByRole('button', { name: /edit vintage core/i }));
  // The Name Input's label isn't wired to the input via htmlFor/id (a
  // pre-existing gap in CampaignFormFields, out of scope here), so
  // getByLabelText can't reach it — select by the seeded display value instead.
  const nameInput = await screen.findByDisplayValue('Vintage Core');
  await user.clear(nameInput);
  await user.type(nameInput, 'Vintage Core Renamed');
  await user.click(screen.getByRole('button', { name: /save changes/i }));

  await waitFor(() => expect(updateMutateAsync).toHaveBeenCalled());
  const { data } = updateMutateAsync.mock.calls[0][0];
  expect(data.name).toBe('Vintage Core Renamed');
});
