import { afterEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import ShowEvidenceDisclosure, { EvidenceDetails, ShowSupport, ShowEvidenceButton } from './ShowEvidence';
import { evaluation, purchaseId } from './fixtures.test-support';

afterEach(() => vi.unstubAllGlobals());
it('includes visible support, cert and expanded relationship in the compact button name', () => {
  render(<ShowEvidenceButton purchaseId={purchaseId} certNumber="12345678" evaluation={evaluation()} expanded={false} onClick={() => {}} />);
  const button = screen.getByRole('button', { name: /Supported.*12345678|12345678.*Supported/ });
  expect(button).toHaveAttribute('aria-expanded', 'false');
  expect(button).toHaveAttribute('aria-controls', `show-evidence-${purchaseId}`);
});
it.each([
  { localPriceCents: 40000, listedPriceCents: 30000, priceAssociationUnclear: false, label: 'SlabLedger asking $400.00' },
  { localPriceCents: 0, listedPriceCents: 30000, priceAssociationUnclear: false, label: 'SlabLedger asking Missing' },
  { localPriceCents: 40000, listedPriceCents: 30000, priceAssociationUnclear: true, label: 'SlabLedger asking $400.00' },
])('identifies the exact evaluated price in compact show context: $label', ({ label, ...values }) => {
  render(<ShowEvidenceButton purchaseId={purchaseId} certNumber="12345678" evaluation={evaluation(values)} showListedPrice expanded={false} onClick={() => {}} />);
  expect(screen.getByText(label)).toBeVisible();
});
it('shows canonical support independently of a DH association warning', () => {
  render(<ShowSupport evaluation={evaluation({ listedPriceCents: 90000, localPriceCents: 30000, priceMismatch: true, priceAssociationUnclear: true })} />);
  expect(screen.getByText('Supported', { selector: 'strong' })).toBeVisible();
  expect(screen.getByText(/^SlabLedger asking/)).toHaveTextContent('$300.00');
  expect(screen.getByText(/Stored DH listed/)).toHaveTextContent('$900.00');
  expect(screen.getByText(/Recent median/)).toHaveTextContent('$280.00');
  expect(screen.getByText(/DH price association unclear; asking assessment is independent/)).toBeVisible();
  expect(screen.queryByText(/excluded from known value/)).not.toBeInTheDocument();
});
it('updates an open summary from a readiness-only observation without changing detail keys or selection versions', async () => {
  const running = evaluation({ readiness: { state: 'current', refreshEligibility: 'not_needed', identityKey: 'a'.repeat(64), expiresAt: '2026-09-15T00:00:00Z', retryAt: '' } });
  const fetcher = vi.fn(async () => new Response(JSON.stringify({ evaluation: running, sales: [] })));
  vi.stubGlobal('fetch', fetcher);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const tree = (e = running) => <QueryClientProvider client={qc}><ShowEvidenceDisclosure purchaseId={purchaseId} certNumber="12345678" expanded evaluation={e} /></QueryClientProvider>;
  const view = render(tree()); await screen.findByText(/Complete current lookup/);
  await waitFor(() => expect(screen.queryByText('Loading sale evidence…')).not.toBeInTheDocument());
  view.rerender(tree({ ...running, status: 'needs_review', evidenceNeedsReview: true, readiness: { ...(running.readiness as object), state: 'failed', refreshEligibility: 'retry_only', expiresAt: '' } }));
  expect(screen.getByText('Data unavailable', { selector: 'strong' })).toBeVisible();
  expect(screen.queryByText('Supported', { selector: 'strong' })).not.toBeInTheDocument();
  expect(fetcher).toHaveBeenCalledOnce();
});
it.each([
  ['not_checked', 'needed', 'No comp data'], ['running', 'wait', 'Data unavailable'], ['stale', 'needed', 'Out of date'],
  ['interrupted', 'retry_only', 'Data unavailable'], ['failed', 'retry_only', 'Data unavailable'], ['invalid', 'retry_only', 'Data unavailable'],
])('distinguishes %s evidence from a verified zero-sale window', (state, refreshEligibility, label) => {
  render(<ShowSupport evaluation={evaluation({ status: 'needs_review', compCount: 0, evidenceNeedsReview: true,
    readiness: { state, refreshEligibility, identityKey: 'a'.repeat(64),
      expiresAt: state === 'stale' ? '2026-09-14T00:00:00Z' : '', retryAt: ['running', 'interrupted'].includes(state) ? '2026-09-14T12:00:00Z' : '' },
  })} />);
  expect(screen.getByText(label)).toBeVisible();
  expect(screen.queryByText(/0 sales/)).not.toBeInTheDocument();
});
it.each([
  { name: 'resolved storage failure', identityKey: 'a'.repeat(64), label: 'Data unavailable' },
  { name: 'unresolved identity', identityKey: '', label: 'Needs matching' },
])('labels unavailable evidence without conflating $name', ({ identityKey, label }) => {
  const e = evaluation({ status: 'needs_review', evidenceNeedsReview: true,
    evidenceReason: identityKey ? 'Evidence storage unavailable' : 'Card identity unresolved',
    readiness: { state: 'unavailable', refreshEligibility: 'unavailable', identityKey, expiresAt: '', retryAt: '' },
  });
  render(<><ShowSupport evaluation={e} /><ShowEvidenceButton purchaseId={purchaseId} certNumber="12345678" evaluation={e} expanded={false} onClick={() => {}} /></>);
  expect(screen.getByRole('button', { name: `Show 30-day evidence 12345678: ${label}` })).toBeVisible();
  expect(screen.getAllByText(label, { selector: 'strong' })).toHaveLength(2);
  expect(screen.queryByText(identityKey ? 'Needs matching' : 'Data unavailable')).not.toBeInTheDocument();
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
it('keeps missing asking price separate from a failed check', () => {
  render(<ShowSupport evaluation={evaluation({ status: 'no_listed_price', localPriceCents: 0, compCount: 0, evidenceNeedsReview: true,
    readiness: { state: 'failed', refreshEligibility: 'retry_only', identityKey: 'a'.repeat(64), expiresAt: '', retryAt: '' },
  })} />);
  expect(screen.getByText('No asking price')).toBeVisible();
  expect(screen.getByText('Data unavailable')).toBeVisible();
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
  current = evaluation({ status: 'below_target', version: 'eval-2', localPriceCents: 45000 });
  view.rerender(tree());
  fireEvent.click(screen.getByRole('button', { name: 'Show 30-day evidence 12345678' }));
  expect(screen.queryByText('Supported', { selector: 'strong' })).not.toBeInTheDocument();
  await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(2));
  expect(screen.getByText('Asking above comps', { selector: 'strong' })).toBeVisible();
});
it.each([
  { name: 'no price and failed recheck', status: 'no_listed_price' as const, reason: 'No positive SlabLedger asking price', localPriceCents: 0, evidenceNeedsReview: true, evidenceReason: 'CardLadder refresh failed' },
  { name: 'no price and healthy evidence', status: 'no_listed_price' as const, reason: 'No positive SlabLedger asking price', localPriceCents: 0, evidenceNeedsReview: false, evidenceReason: '' },
  { name: 'ambiguous DH price and healthy evidence', status: 'thin_evidence' as const, reason: 'Only one recent matching sale; review its amount', priceAssociationUnclear: true, localPriceCents: 30000, evidenceNeedsReview: false, evidenceReason: '' },
])('warns about retained sales only when evidence is unhealthy: $name', async ({ name: _name, ...health }) => {
  const e = evaluation({ ...health, compCount: 1, medianCents: 27000, recent: {
    ...evaluation().recent, saleIds: ['a'], count: 1, medianCents: 27000,
    latestSaleCount: 1, latestSaleMinCents: 27000, latestSaleMaxCents: 27000, gapPct: health.localPriceCents <= 0 || health.evidenceNeedsReview ? null : 10,
  } });
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ evaluation: e, sales: [
    { id: 'a', date: '2026-09-13', priceCents: 27000, platform: 'eBay', url: 'https://example.test/sale', listingType: 'Auction' },
  ] }))));
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><ShowEvidenceDisclosure purchaseId={purchaseId} certNumber="12345678" evaluation={e} /></QueryClientProvider>);
  fireEvent.click(screen.getByRole('button', { name: 'Show 30-day evidence 12345678' }));
  expect(await screen.findByRole('list', { name: 'Individual matching sales' })).toHaveTextContent('$270.00');
  expect(screen.getByText(health.reason)).toBeVisible();
  if (health.evidenceNeedsReview) {
    expect(screen.getByText('No asking price', { selector: 'strong' })).toBeVisible();
    expect(screen.getByText('CardLadder refresh failed')).toBeVisible();
    expect(screen.getByText(/Stored sales may be partial or stale/)).toBeVisible();
  } else {
    expect(screen.queryByText(/Stored sales may be partial or stale/)).not.toBeInTheDocument();
  }
});

it.each([false, true])('describes an empty window from independent health, not missing price (needs review=%s)', evidenceNeedsReview => {
  render(<EvidenceDetails data={{ evaluation: evaluation({ status: 'no_listed_price', reason: 'No positive SlabLedger asking price', localPriceCents: 0,
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
