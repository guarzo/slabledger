import { render, screen } from '@testing-library/react';
import TabularPriceTriplet from './TabularPriceTriplet';

describe('TabularPriceTriplet', () => {
  it('renders all three labeled rows', () => {
    render(
      <TabularPriceTriplet
        rows={[
          { label: 'Cost', value: '$2,328.00' },
          { label: 'CL', value: '$4,715.00' },
          { label: 'Sug', value: '$4,243.50' },
        ]}
      />,
    );
    expect(screen.getByText('Cost')).toBeInTheDocument();
    expect(screen.getByText('$2,328.00')).toBeInTheDocument();
    expect(screen.getByText('CL')).toBeInTheDocument();
    expect(screen.getByText('$4,715.00')).toBeInTheDocument();
    expect(screen.getByText('Sug')).toBeInTheDocument();
    expect(screen.getByText('$4,243.50')).toBeInTheDocument();
  });

  it('applies highlight class to the row marked highlighted', () => {
    const { container } = render(
      <TabularPriceTriplet
        rows={[
          { label: 'Cost', value: '$1.00' },
          { label: 'CL', value: '$2.00' },
          { label: 'Sug', value: '$3.00', highlighted: true },
        ]}
      />,
    );
    const highlightedRow = container.querySelector('[data-highlighted="true"]');
    expect(highlightedRow).not.toBeNull();
    expect(highlightedRow?.textContent).toContain('Sug');
    expect(highlightedRow?.textContent).toContain('$3.00');
  });

  it('renders nothing when rows is empty', () => {
    const { container } = render(<TabularPriceTriplet rows={[]} />);
    expect(container.firstChild).toBeNull();
  });
});
