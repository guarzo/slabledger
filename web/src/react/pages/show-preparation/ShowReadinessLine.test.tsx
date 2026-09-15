import { expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import type { useShowReadiness } from '../../queries/useShowReadiness';
import ShowReadinessLine from './ShowReadinessLine';

it.each([0, 12])('does not offer a no-op source check with %s cards and no incomplete work', count => {
  const readiness = { busy: false, incomplete: false, currentCount: count, cohortCount: count,
    counts: { running: 0, unknown: 0, interrupted: 0 }, missingPriceCount: 0, error: '', check: vi.fn() } as unknown as ReturnType<typeof useShowReadiness>;
  render(<ShowReadinessLine readiness={readiness} selectedCount={0} />);
  expect(screen.queryByRole('button', { name: 'Check comps' })).not.toBeInTheDocument();
  expect(screen.getByRole('status')).toHaveTextContent(count ? 'Check complete' : 'No cards in check scope');
});
