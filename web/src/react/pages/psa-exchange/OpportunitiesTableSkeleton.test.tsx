import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import OpportunitiesTableSkeleton from './OpportunitiesTableSkeleton';

describe('OpportunitiesTableSkeleton', () => {
  it('renders 5 skeleton rows by default', () => {
    render(<OpportunitiesTableSkeleton />);
    const rows = screen.getAllByTestId('opportunities-skeleton-row');
    expect(rows).toHaveLength(5);
  });

  it('respects the rows prop', () => {
    render(<OpportunitiesTableSkeleton rows={3} />);
    expect(screen.getAllByTestId('opportunities-skeleton-row')).toHaveLength(3);
  });

  it('exposes a polite live region announcing the loading state', () => {
    render(<OpportunitiesTableSkeleton />);
    const status = screen.getByRole('status');
    expect(status).toHaveTextContent(/fetching psa-exchange listings/i);
    expect(status).toHaveAttribute('aria-live', 'polite');
  });

  it('marks the skeleton container as aria-busy', () => {
    const { container } = render(<OpportunitiesTableSkeleton />);
    expect(container.querySelector('[aria-busy="true"]')).toBeInTheDocument();
  });
});
