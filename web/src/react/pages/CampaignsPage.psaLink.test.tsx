import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import CampaignsPage from './CampaignsPage';
import { ToastProvider } from '../contexts/ToastContext';
import { api } from '../../js/api';

vi.mock('../queries/useCampaignQueries', async (orig) => {
  const mod = await orig<typeof import('../queries/useCampaignQueries')>();
  return {
    ...mod,
    useCampaigns: () => ({ data: [], isLoading: false }),
    usePortfolioHealth: () => ({ data: undefined }),
    useCreateCampaign: () => ({ mutateAsync: vi.fn(), isPending: false }),
  };
});

// Spying on the real singleton rather than spreading it: `api` is an APIClient
// instance whose endpoint methods live on the prototype, so `{ ...mod.api }`
// copies none of them and every endpoint this file does not name becomes
// undefined. Stubbing fetchWithRetry — the single choke point behind
// get/post/put/deleteResource — keeps an unstubbed endpoint off the network.
beforeEach(() => {
  vi.spyOn(api, 'fetchWithRetry').mockRejectedValue(
    new Error('unstubbed API call — add a vi.spyOn for this endpoint'),
  );
  vi.spyOn(api, 'listPSAPushes').mockResolvedValue({ pushes: [] });
});

afterEach(() => {
  vi.restoreAllMocks();
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

it('PSA portal button opens the buyer campaign manager in a new tab', async () => {
  const user = userEvent.setup();
  const openSpy = vi.spyOn(window, 'open').mockImplementation(() => null);
  renderPage();

  await user.click(screen.getByRole('button', { name: /PSA Buyer Campaign Manager/i }));
  expect(openSpy).toHaveBeenCalledWith(
    'https://exchange.psacard.com/campaigns',
    '_blank',
    'noopener,noreferrer',
  );
});
