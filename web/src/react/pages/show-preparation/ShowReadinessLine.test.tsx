import { expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import type { useShowReadiness } from '../../queries/useShowReadiness';
import ShowReadinessLine from './ShowReadinessLine';

it('distinguishes coverage gaps without acquisition controls or manual-price prompts', () => {
  const readiness = { observing: false, observationError: '', currentCount: 2, cohortCount: 8,
    counts: { current: 2, not_checked: 1, stale: 1, failed: 1, running: 0, interrupted: 0, invalid: 0, unknown: 1, unavailable: 2 },
    missingPriceCount: 1 } as ReturnType<typeof useShowReadiness>;
  render(<ShowReadinessLine readiness={readiness} />);
  expect(screen.getByLabelText('Comp data coverage')).toHaveTextContent('2/8 cards with current evidence');
  for (const label of ['1 no comp data', '1 out of date', '1 data unavailable', '1 no DH price']) expect(screen.getByText(label)).toBeVisible();
  expect(screen.queryByRole('button')).not.toBeInTheDocument();
  expect(screen.queryByText(/select.*manually/i)).not.toBeInTheDocument();
});
