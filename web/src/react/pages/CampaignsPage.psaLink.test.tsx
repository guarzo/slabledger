import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import CampaignsPage from './CampaignsPage';
import { ToastProvider } from '../contexts/ToastContext';

vi.mock('../queries/useCampaignQueries', async (orig) => {
  const mod = await orig<typeof import('../queries/useCampaignQueries')>();
  return {
    ...mod,
    useCampaigns: () => ({ data: [], isLoading: false }),
    usePortfolioHealth: () => ({ data: undefined }),
    useCreateCampaign: () => ({ mutateAsync: vi.fn(), isPending: false }),
  };
});

vi.mock('../../js/api', async (orig) => {
  const mod = await orig<typeof import('../../js/api')>();
  return { ...mod, api: { ...mod.api, listPSAPushes: vi.fn().mockResolvedValue({ pushes: [] }) } };
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
    'https://www.psacard.com/buyercampaignmanager/',
    '_blank',
    'noopener,noreferrer',
  );
});
