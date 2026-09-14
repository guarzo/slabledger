import { afterEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import ShowEvidenceDisclosure from './ShowEvidence';
import { evaluation, purchaseId } from './fixtures.test-support';

afterEach(() => vi.unstubAllGlobals());
it('does not revive an older supported detail after a newer evaluation arrives', async () => {
  let current = evaluation();
  const fetcher = vi.fn(async () => new Response(JSON.stringify({ evaluation: current, sales: [] })));
  vi.stubGlobal('fetch', fetcher);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const tree = () => <QueryClientProvider client={qc}><ShowEvidenceDisclosure purchaseId={purchaseId} certNumber="12345678" evaluation={current} /></QueryClientProvider>;
  const view = render(tree());
  fireEvent.click(screen.getByRole('button', { name: 'Show 30-day evidence 12345678' }));
  await screen.findByText(/No detailed sales available/);
  fireEvent.click(screen.getByRole('button', { name: 'Hide 30-day evidence 12345678' }));
  current = evaluation({ status: 'below_target', version: 'eval-2', listedPriceCents: 45000 });
  view.rerender(tree());
  fireEvent.click(screen.getByRole('button', { name: 'Show 30-day evidence 12345678' }));
  expect(screen.queryByText('Supported', { selector: 'strong' })).not.toBeInTheDocument();
  await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(2));
  expect(screen.getByText('Below target', { selector: 'strong' })).toBeVisible();
});
it('displays a failed evidence read with retry, never a successful empty lookup', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: 'Evidence unavailable' }), { status: 404 })));
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><ShowEvidenceDisclosure purchaseId={purchaseId} certNumber="12345678" error="Missing evaluation" /></QueryClientProvider>);
  fireEvent.click(screen.getByRole('button', { name: 'Show 30-day evidence 12345678' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Evidence unavailable');
  expect(screen.getByRole('button', { name: 'Retry evidence' })).toBeEnabled();
  expect(screen.queryByText('No recent comps')).not.toBeInTheDocument();
});
