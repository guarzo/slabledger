/**
 * Edit-card lifecycle: scroll/focus on open, remount on campaign switch,
 * mutual exclusion with Create, and recovery when the campaign is deleted
 * mid-edit.
 *
 * Kept apart from CampaignsPage.edit.test.tsx (which covers the save path's
 * staleness guard and PUT payload) because these tests need a mutable campaign
 * list and a populated subject catalog, where those need a fixed single row.
 */
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import CampaignsPage from './CampaignsPage';
import { ToastProvider } from '../contexts/ToastContext';
import { api } from '../../js/api';
import type { Campaign } from '../../types/campaigns';
import { LEGACY_UNRECONCILED_SUBJECT_ID } from '../utils/campaignConstants';

function makeCampaign(over: Partial<Campaign>): Campaign {
  return {
    id: 'c1',
    name: 'Alpha',
    sport: 'Pokemon',
    yearRange: '1999-2003',
    gradeRange: '8-10',
    priceRange: '50-500',
    clConfidence: 'high',
    buyTermsCLPct: 0.78,
    dailySpendCapCents: 50000,
    targetLanguages: ['english'],
    subjectFilterMode: 'Target',
    subjects: [],
    deniedSpecs: [],
    phase: 'active',
    psaSourcingFeeCents: 300,
    ebayFeePct: 0.1235,
    expectedFillRate: 42,
    psaCampaignRequestId: '',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-02-02T00:00:00Z',
    ...over,
  } as Campaign;
}

// Campaign A carries a legacy (-1) subject; the catalog offers the same name.
// Removing it on A registers the name in SubjectListEditor's removedLegacyNames
// set — the state that must not survive a switch to B.
const alpha = makeCampaign({
  id: 'c1',
  name: 'Alpha',
  subjects: [{ id: LEGACY_UNRECONCILED_SUBJECT_ID, name: 'Pikachu' }],
});
const beta = makeCampaign({ id: 'c2', name: 'Beta', subjects: [] });

// Mutable so a test can simulate the list refetching without a deleted row.
let campaignList: Campaign[] = [alpha, beta];
let listIsSuccess = true;

vi.mock('../queries/useCampaignQueries', async (orig) => {
  const mod = await orig<typeof import('../queries/useCampaignQueries')>();
  return {
    ...mod,
    useCampaigns: () => ({ data: campaignList, isLoading: false, isSuccess: listIsSuccess }),
    usePortfolioHealth: () => ({ data: undefined }),
    useCreateCampaign: () => ({ mutateAsync: vi.fn(), isPending: false }),
    useUpdateCampaign: () => ({ mutateAsync: vi.fn(), isPending: false }),
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

/** The edit card, addressed by the accessible name its heading supplies. */
function editCard(name: string) {
  return screen.getByRole('region', { name: new RegExp(`edit ${name}`, 'i') });
}

beforeEach(() => {
  campaignList = [alpha, beta];
  listIsSuccess = true;
  // Spying on the real singleton rather than spreading it: `api` is an
  // APIClient instance whose endpoint methods live on the prototype, so
  // `{ ...mod.api }` copies none of them and every endpoint this file does not
  // name becomes undefined. Stubbing fetchWithRetry — the single choke point
  // behind get/post/put/deleteResource — keeps an unstubbed endpoint off the
  // network.
  vi.spyOn(api, 'fetchWithRetry').mockRejectedValue(
    new Error('unstubbed API call — add a vi.spyOn for this endpoint'),
  );
  vi.spyOn(api, 'listPSAPushes').mockResolvedValue({ pushes: [] });
  vi.spyOn(api, 'listPSASubjects').mockResolvedValue({
    subjects: [{ id: 777, name: 'Pikachu' }, { id: 888, name: 'Blastoise' }],
    fetchedAt: new Date().toISOString(),
  });
  vi.spyOn(api, 'getCampaign');
  // getCampaignPNL is deliberately left to the fetchWithRetry stub. The old
  // mock resolved it to `undefined`, which does not typecheck against
  // Promise<CampaignPNL> — the spread mock was untyped, so nothing caught it.
  // Letting it reject leaves the query's `data` undefined either way, which is
  // all this file's assertions depend on.
  // jsdom implements neither of these; the component calls both optionally.
  Element.prototype.scrollIntoView = vi.fn();
  vi.stubGlobal('matchMedia', undefined);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('opening the edit card', () => {
  it('scrolls the card into view and focuses it', async () => {
    // The card renders above the list, so an Edit click on a lower row is
    // otherwise invisible — no scroll, no focus move, no SR announcement.
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByRole('button', { name: /edit beta/i }));

    const card = await screen.findByRole('region', { name: /edit beta/i });
    expect(Element.prototype.scrollIntoView).toHaveBeenCalled();
    // Focus on the section is what carries the announcement: it is labelled by
    // its own "Edit Beta" heading via aria-labelledby.
    expect(card).toHaveFocus();
  });
});

describe('switching directly from one campaign to another', () => {
  it('remounts the subject editor so a removed legacy name is addable again', async () => {
    // Without a key on the edit card, React reuses the SubjectListEditor
    // instance across the switch and its removedLegacyNames set leaks: a name
    // blocked on Alpha stays unaddable on Beta, where it is legitimate.
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByRole('button', { name: /edit alpha/i }));
    await screen.findByRole('region', { name: /edit alpha/i });

    // Remove the legacy chip on Alpha — this is what registers the block.
    // Removing a legacy subject goes through ConfirmDialog rather than
    // window.confirm, so the block registers on the dialog's confirm.
    await user.click(screen.getByRole('button', { name: /remove pikachu/i }));
    await user.click(await screen.findByRole('button', { name: 'Remove' }));

    // Still on Alpha, the catalog no longer offers the name back.
    const alphaInput = within(editCard('Alpha')).getByPlaceholderText(/add a subject by name/i);
    await user.type(alphaInput, 'Pika');
    expect(screen.queryByRole('button', { name: 'Pikachu' })).not.toBeInTheDocument();

    // Switch straight to Beta without cancelling first.
    await user.click(screen.getByRole('button', { name: /edit beta/i }));
    await screen.findByRole('region', { name: /edit beta/i });

    const betaInput = within(editCard('Beta')).getByPlaceholderText(/add a subject by name/i);
    await user.type(betaInput, 'Pika');
    expect(await screen.findByRole('button', { name: 'Pikachu' })).toBeInTheDocument();
  });

  it('re-runs scroll and focus for the newly selected campaign', async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByRole('button', { name: /edit alpha/i }));
    await screen.findByRole('region', { name: /edit alpha/i });

    await user.click(screen.getByRole('button', { name: /edit beta/i }));
    const betaCard = await screen.findByRole('region', { name: /edit beta/i });
    expect(betaCard).toHaveFocus();
  });
});

describe('create/edit exclusion', () => {
  it('closes the edit card when Create is opened', async () => {
    // handleEdit already clears showCreate; without the reverse clear both
    // full-height form cards stack and it stops being clear which one a Save
    // applies to.
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByRole('button', { name: /edit alpha/i }));
    expect(await screen.findByRole('region', { name: /edit alpha/i })).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /new campaign/i }));

    expect(await screen.findByRole('heading', { name: /create campaign/i })).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.queryByRole('region', { name: /edit alpha/i })).not.toBeInTheDocument(),
    );
  });
});

describe('a campaign deleted mid-edit', () => {
  it('clears the edit state and tells the operator', async () => {
    // The row drops out on the next refetch. Previously the card unmounted
    // silently while `editing` stayed set, leaving a save target that could
    // only ever 404.
    const user = userEvent.setup();
    const { rerender } = renderPage();

    await user.click(screen.getByRole('button', { name: /edit alpha/i }));
    await screen.findByRole('region', { name: /edit alpha/i });

    campaignList = [beta];
    rerender(
      <MemoryRouter>
        <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
          <ToastProvider>
            <CampaignsPage />
          </ToastProvider>
        </QueryClientProvider>
      </MemoryRouter>,
    );

    expect(await screen.findByText(/no longer exists/i)).toBeInTheDocument();
    expect(screen.queryByRole('region', { name: /edit alpha/i })).not.toBeInTheDocument();
  });

  it('keeps the edit open when the list query merely failed', async () => {
    // A failed refetch falls back to [], which must not read as
    // "everything was deleted" and discard an in-progress edit.
    const user = userEvent.setup();
    const { rerender } = renderPage();

    await user.click(screen.getByRole('button', { name: /edit alpha/i }));
    await screen.findByRole('region', { name: /edit alpha/i });

    campaignList = [];
    listIsSuccess = false;
    rerender(
      <MemoryRouter>
        <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
          <ToastProvider>
            <CampaignsPage />
          </ToastProvider>
        </QueryClientProvider>
      </MemoryRouter>,
    );

    expect(screen.queryByText(/no longer exists/i)).not.toBeInTheDocument();
  });
});
