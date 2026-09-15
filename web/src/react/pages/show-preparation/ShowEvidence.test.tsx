import { afterEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import ShowEvidenceDisclosure, { EvidenceDetails, ShowSupport } from './ShowEvidence';
import { evaluation, purchaseId } from './fixtures.test-support';

afterEach(() => vi.unstubAllGlobals());
it.each([
  ['not_checked', 'needed', 'Not checked'], ['running', 'wait', 'Checking'], ['stale', 'needed', 'Stale comps'],
  ['interrupted', 'retry_only', 'Check interrupted'], ['failed', 'retry_only', 'Check failed'], ['invalid', 'retry_only', 'Evidence needs review'],
])('distinguishes %s evidence from a verified zero-sale window', (state, refreshEligibility, label) => {
  render(<ShowSupport evaluation={evaluation({ status: 'needs_review', compCount: 0, evidenceNeedsReview: true,
    readiness: { state, refreshEligibility, identityKey: 'a'.repeat(64),
      expiresAt: state === 'stale' ? '2026-09-14T00:00:00Z' : '', retryAt: ['running', 'interrupted'].includes(state) ? '2026-09-14T12:00:00Z' : '' },
  })} />);
  expect(screen.getByText(label)).toBeVisible();
  expect(screen.queryByText(/0 sales/)).not.toBeInTheDocument();
});
it('does not color a supported explanation as a warning and formats source listing enums', () => {
  render(<EvidenceDetails data={{ evaluation: evaluation({ reason: 'Median supports listed price' }), sales: [
    { id: 'a', date: '2026-09-13', priceCents: 27000, platform: 'eBay', url: '', listingType: 'BestOffer' },
    { id: 'b', date: '2026-09-13', priceCents: 28000, platform: 'eBay', url: '', listingType: 'FixedPrice' },
  ] }} />);
  expect(screen.getByText('Median supports listed price')).not.toHaveClass('text-[var(--warning)]');
  expect(screen.getByText('Best offer')).toBeVisible();
  expect(screen.getByText('Fixed price')).toBeVisible();
});
it('keeps missing listed price separate from a failed check', () => {
  render(<ShowSupport evaluation={evaluation({ status: 'no_listed_price', listedPriceCents: 0, compCount: 0, evidenceNeedsReview: true,
    readiness: { state: 'failed', refreshEligibility: 'retry_only', identityKey: 'a'.repeat(64), expiresAt: '', retryAt: '' },
  })} />);
  expect(screen.getByText('No listed price')).toBeVisible();
  expect(screen.getByText('Check failed')).toBeVisible();
  expect(screen.queryByText(/0 sales/)).not.toBeInTheDocument();
});
it('does not revive an older supported detail after a newer evaluation arrives', async () => {
  let current = evaluation();
  const fetcher = vi.fn(async () => new Response(JSON.stringify({ evaluation: current, sales: [
    { id: 'a', date: '2026-09-13', priceCents: 27000, platform: 'eBay', url: '', listingType: 'Auction' },
    { id: 'b', date: '2026-09-13', priceCents: 29000, platform: 'eBay', url: '', listingType: 'Auction' },
  ] })));
  vi.stubGlobal('fetch', fetcher);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const tree = () => <QueryClientProvider client={qc}><ShowEvidenceDisclosure purchaseId={purchaseId} certNumber="12345678" evaluation={current} /></QueryClientProvider>;
  const view = render(tree());
  fireEvent.click(screen.getByRole('button', { name: 'Show 30-day evidence 12345678' }));
  await screen.findByRole('list', { name: 'Individual matching sales' });
  fireEvent.click(screen.getByRole('button', { name: 'Hide 30-day evidence 12345678' }));
  current = evaluation({ status: 'below_target', version: 'eval-2', listedPriceCents: 45000 });
  view.rerender(tree());
  fireEvent.click(screen.getByRole('button', { name: 'Show 30-day evidence 12345678' }));
  expect(screen.queryByText('Supported', { selector: 'strong' })).not.toBeInTheDocument();
  await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(2));
  expect(screen.getByText('Below target', { selector: 'strong' })).toBeVisible();
});
it.each([
  { name: 'no price and failed recheck', status: 'no_listed_price' as const, reason: 'No positive DH listed price', listedPriceCents: 0, evidenceNeedsReview: true, evidenceReason: 'CardLadder refresh failed' },
  { name: 'no price and healthy evidence', status: 'no_listed_price' as const, reason: 'No positive DH listed price', listedPriceCents: 0, evidenceNeedsReview: false, evidenceReason: '' },
  { name: 'ambiguous price and healthy evidence', status: 'needs_review' as const, reason: 'DH price association unclear', listedPriceCents: 30000, evidenceNeedsReview: false, evidenceReason: '' },
])('warns about retained sales only when evidence is unhealthy: $name', async ({ name: _name, ...health }) => {
  const e = evaluation({ ...health, compCount: 1, medianCents: 27000 });
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ evaluation: e, sales: [
    { id: 'a', date: '2026-09-13', priceCents: 27000, platform: 'eBay', url: 'https://example.test/sale', listingType: 'Auction' },
  ] }))));
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><ShowEvidenceDisclosure purchaseId={purchaseId} certNumber="12345678" evaluation={e} /></QueryClientProvider>);
  fireEvent.click(screen.getByRole('button', { name: 'Show 30-day evidence 12345678' }));
  expect(await screen.findByRole('list', { name: 'Individual matching sales' })).toHaveTextContent('$270.00');
  expect(screen.getByText(health.reason)).toBeVisible();
  if (health.evidenceNeedsReview) {
    expect(screen.getByText('No listed price', { selector: 'strong' })).toBeVisible();
    expect(screen.getByText('CardLadder refresh failed')).toBeVisible();
    expect(screen.getByText(/Stored sales may be partial or stale/)).toBeVisible();
  } else {
    expect(screen.queryByText(/Stored sales may be partial or stale/)).not.toBeInTheDocument();
  }
});

it.each([false, true])('describes an empty window from independent health, not missing price (needs review=%s)', evidenceNeedsReview => {
  render(<EvidenceDetails data={{ evaluation: evaluation({ status: 'no_listed_price', reason: 'No positive DH listed price', listedPriceCents: 0,
    compCount: 0, medianCents: 0, evidenceNeedsReview, evidenceReason: evidenceNeedsReview ? 'CardLadder refresh failed' : '',
  }), sales: [] }} />);
  if (evidenceNeedsReview) {
    expect(screen.getByText(/This does not establish no recent comps/)).toBeVisible();
    expect(screen.queryByText(/Complete current lookup/)).not.toBeInTheDocument();
  } else {
    expect(screen.getByText('Complete current lookup: no matching sales in this window.')).toBeVisible();
    expect(screen.queryByText(/This does not establish no recent comps/)).not.toBeInTheDocument();
  }
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
