import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import userEvent from '@testing-library/user-event';
import SignalCell from './SignalCell';

const baseRow = {
  edgeAtOffer: 0.333,
  daysToSellValue: 1,
  velocityMonth: 35,
  confidence: 8,
  comp: 13100,
  population: 12,
};

describe('SignalCell', () => {
  it('renders the Edge percentage as the loud line', () => {
    render(<SignalCell {...baseRow} />);
    expect(screen.getByText('33.3%')).toBeInTheDocument();
  });

  it('renders three indicator glyphs (days, velocity, confidence)', () => {
    render(<SignalCell {...baseRow} />);
    expect(screen.getByLabelText(/days to sell/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/velocity/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/confidence/i)).toBeInTheDocument();
  });

  it('exposes the full numerics in the popover', async () => {
    const user = userEvent.setup();
    render(<SignalCell {...baseRow} />);
    await user.click(screen.getByRole('button', { name: /signal details/i }));
    expect(await screen.findByText(/days\/sale/i)).toBeInTheDocument();
    expect(await screen.findByText(/^velocity$/i)).toBeInTheDocument();
    expect(await screen.findByText(/^confidence$/i)).toBeInTheDocument();
    expect(await screen.findByText(/^comp$/i)).toBeInTheDocument();
    expect(await screen.findByText(/^pop$/i)).toBeInTheDocument();
    expect(await screen.findByText('$13,100')).toBeInTheDocument();
    expect(await screen.findByText('12')).toBeInTheDocument();
  });

  it('formats <1d for sub-1 days/sale', () => {
    render(<SignalCell {...baseRow} daysToSellValue={0.5} />);
    // The trigger button does not surface this string; check via aria-label.
    expect(screen.getByLabelText(/days to sell: <1d/i)).toBeInTheDocument();
  });

  it('still renders Edge value when daysToSell is non-finite', () => {
    render(<SignalCell {...baseRow} daysToSellValue={Number.POSITIVE_INFINITY} />);
    expect(screen.getByText('33.3%')).toBeInTheDocument();
  });
});
