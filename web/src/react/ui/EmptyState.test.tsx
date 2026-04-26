import { render, screen } from '@testing-library/react';
import EmptyState from './EmptyState';

describe('EmptyState', () => {
  it('renders title, description, and icon', () => {
    render(<EmptyState icon="📊" title="No data" description="Nothing yet." />);
    expect(screen.getByText('No data')).toBeInTheDocument();
    expect(screen.getByText('Nothing yet.')).toBeInTheDocument();
    expect(screen.getByText('📊')).toBeInTheDocument();
  });

  it('renders lastAction caption when provided', () => {
    render(
      <EmptyState
        icon="📊"
        title="No sales"
        description="Record your first sale."
        lastAction="Last intake: Apr 24"
      />,
    );
    expect(screen.getByText('Last intake: Apr 24')).toBeInTheDocument();
  });

  it('does not render lastAction when omitted', () => {
    render(
      <EmptyState icon="📊" title="No sales" description="Record your first sale." />,
    );
    expect(screen.queryByText('Last intake: Apr 24')).not.toBeInTheDocument();
  });
});
