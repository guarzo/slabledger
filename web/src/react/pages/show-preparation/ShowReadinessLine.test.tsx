import { expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useShowReadiness } from '../../queries/useShowReadiness';
import { evaluation } from './fixtures.test-support';
import ShowReadinessLine from './ShowReadinessLine';

it('distinguishes coverage gaps without acquisition controls or manual-price prompts', () => {
  const readiness = { observing: false, observationError: '', currentCount: 2, cohortCount: 8,
    counts: { current: 2, not_checked: 1, stale: 1, failed: 1, running: 0, interrupted: 0, invalid: 0, unknown: 1, unavailable: 2 },
    needsMatchingCount: 1, missingPriceCount: 1 } as ReturnType<typeof useShowReadiness>;
  render(<ShowReadinessLine readiness={readiness} coverage={{ eligibleIdentities: 5, currentIdentities: 1, missingIdentities: 1, staleIdentities: 1, failedIdentities: 2, eligibleCards: 8, currentCards: 2, unresolvedCards: 1 }} />);
  expect(screen.getByLabelText('Comp data coverage')).toHaveTextContent('2/8 cards with current evidence');
  expect(screen.getByText('All inventory: 1/5 identities current')).toBeVisible();
  for (const label of ['1 missing · 1 stale · 2 failed identities', '1 unresolved card', '1 no asking price in this view']) expect(screen.getByText(label)).toBeVisible();
  expect(screen.queryByRole('button')).not.toBeInTheDocument();
  expect(screen.queryByText(/select.*manually/i)).not.toBeInTheDocument();
});

it('stays quiet when coverage is current and prices are present', () => {
  const readiness = { missingPriceCount: 0, observationError: '' } as ReturnType<typeof useShowReadiness>;
  render(<ShowReadinessLine readiness={readiness} coverage={{ eligibleIdentities: 1, currentIdentities: 1, missingIdentities: 0, staleIdentities: 0, failedIdentities: 0, eligibleCards: 5, currentCards: 5, unresolvedCards: 0 }} />);
  expect(screen.queryByLabelText('Comp data coverage')).not.toBeInTheDocument();
});

it('counts resolved storage failures separately from identities needing matching', () => {
  const values = [
    evaluation({ purchaseId: 'resolved', status: 'needs_review', evidenceNeedsReview: true,
      readiness: { state: 'unavailable', refreshEligibility: 'unavailable', identityKey: 'a'.repeat(64), expiresAt: '', retryAt: '' } }),
    evaluation({ purchaseId: 'unresolved', status: 'needs_review', evidenceNeedsReview: true,
      readiness: { state: 'unavailable', refreshEligibility: 'unavailable', identityKey: '', expiresAt: '', retryAt: '' } }),
    evaluation({ purchaseId: 'unreadable', status: 'needs_review', evidenceNeedsReview: true, availability: 'unknown', canAdd: false, canPack: false,
      readiness: { state: 'unavailable', refreshEligibility: 'unavailable', identityKey: '', expiresAt: '', retryAt: '' } }),
  ];
  const ids = values.map(e => e.purchaseId);
  const evaluations = Object.fromEntries(values.map(e => [e.purchaseId, e]));
  function Coverage() {
    const readiness = useShowReadiness(ids, evaluations);
    return <ShowReadinessLine readiness={readiness} coverage={{ eligibleIdentities: 1, currentIdentities: 0, missingIdentities: 0, staleIdentities: 0, failedIdentities: 1, eligibleCards: 2, currentCards: 0, unresolvedCards: 1 }} />;
  }
  const qc = new QueryClient();
  const view = render(<QueryClientProvider client={qc}><Coverage /></QueryClientProvider>);
  expect(screen.getByText('0 missing · 0 stale · 1 failed identities')).toBeVisible();
  expect(screen.getByText('1 unresolved card')).toBeVisible();
  expect(screen.queryByText(/needs matching or unavailable/)).not.toBeInTheDocument();
  expect(screen.queryByRole('button')).not.toBeInTheDocument();
  view.unmount(); qc.clear();
});
