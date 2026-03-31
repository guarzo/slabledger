import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import PriceDecisionBar from './PriceDecisionBar';
import type { PriceSource } from './PriceDecisionBar';

const sources: PriceSource[] = [
  { label: 'CL', priceCents: 28500, source: 'cl' },
  { label: 'Market', priceCents: 26000, source: 'market' },
  { label: 'Cost', priceCents: 14250, source: 'cost_basis' },
  { label: 'Last Sold', priceCents: 27000, source: 'last_sold' },
];

describe('PriceDecisionBar', () => {
  it('renders all source buttons with formatted prices', () => {
    render(<PriceDecisionBar sources={sources} onConfirm={() => {}} />);
    expect(screen.getByText(/CL/)).toBeInTheDocument();
    expect(screen.getByText(/\$285\.00/)).toBeInTheDocument();
    expect(screen.getByText(/Market/)).toBeInTheDocument();
    expect(screen.getByText(/\$260\.00/)).toBeInTheDocument();
    expect(screen.getByText(/Cost/)).toBeInTheDocument();
    expect(screen.getByText(/\$142\.50/)).toBeInTheDocument();
    expect(screen.getByText(/Last Sold/)).toBeInTheDocument();
    expect(screen.getByText(/\$270\.00/)).toBeInTheDocument();
  });

  it('disables buttons with 0 price and shows dash', () => {
    const withZero: PriceSource[] = [
      { label: 'CL', priceCents: 0, source: 'cl' },
      { label: 'Cost', priceCents: 14250, source: 'cost_basis' },
    ];
    render(<PriceDecisionBar sources={withZero} onConfirm={() => {}} />);
    const clButton = screen.getByRole('button', { name: /CL/ });
    expect(clButton).toBeDisabled();
    expect(clButton).toHaveTextContent('—');
  });

  it('pre-selects the specified source on mount', () => {
    render(<PriceDecisionBar sources={sources} preSelected="cl" onConfirm={() => {}} />);
    const input = screen.getByPlaceholderText('0.00') as HTMLInputElement;
    expect(input.value).toBe('285.00');
  });

  it('calls onConfirm with selected source price', async () => {
    const onConfirm = vi.fn();
    render(<PriceDecisionBar sources={sources} preSelected="cl" onConfirm={onConfirm} />);
    await userEvent.click(screen.getByRole('button', { name: /Confirm/ }));
    expect(onConfirm).toHaveBeenCalledWith(28500, 'cl');
  });

  it('calls onConfirm with custom value as manual source', async () => {
    const onConfirm = vi.fn();
    render(<PriceDecisionBar sources={sources} onConfirm={onConfirm} />);
    const input = screen.getByPlaceholderText('0.00');
    await userEvent.clear(input);
    await userEvent.type(input, '300.00');
    await userEvent.click(screen.getByRole('button', { name: /Confirm/ }));
    expect(onConfirm).toHaveBeenCalledWith(30000, 'manual');
  });

  it('clicking a source button syncs the dollar input', async () => {
    render(<PriceDecisionBar sources={sources} onConfirm={() => {}} />);
    await userEvent.click(screen.getByRole('button', { name: /Market/ }));
    const input = screen.getByPlaceholderText('0.00') as HTMLInputElement;
    expect(input.value).toBe('260.00');
  });

  it('typing in input clears source selection', async () => {
    const onConfirm = vi.fn();
    render(<PriceDecisionBar sources={sources} preSelected="cl" onConfirm={onConfirm} />);
    const input = screen.getByPlaceholderText('0.00');
    await userEvent.clear(input);
    await userEvent.type(input, '999.00');
    await userEvent.click(screen.getByRole('button', { name: /Confirm/ }));
    expect(onConfirm).toHaveBeenCalledWith(99900, 'manual');
  });

  it('shows Skip button when onSkip is provided', () => {
    render(<PriceDecisionBar sources={sources} onConfirm={() => {}} onSkip={() => {}} />);
    expect(screen.getByRole('button', { name: /Skip/ })).toBeInTheDocument();
  });

  it('does not show Skip button when onSkip is not provided', () => {
    render(<PriceDecisionBar sources={sources} onConfirm={() => {}} />);
    expect(screen.queryByRole('button', { name: /Skip/ })).not.toBeInTheDocument();
  });

  it('shows Flag Price Issue button when onFlag is provided', () => {
    render(<PriceDecisionBar sources={sources} onConfirm={() => {}} onFlag={() => {}} />);
    expect(screen.getByRole('button', { name: /Flag Price Issue/ })).toBeInTheDocument();
  });

  it('renders accepted state with locked price and Change button', () => {
    const onReset = vi.fn();
    render(
      <PriceDecisionBar sources={sources} preSelected="cl" status="accepted" onConfirm={() => {}} onReset={onReset} />
    );
    const priceElements = screen.getAllByText(/\$285\.00/);
    expect(priceElements.length).toBeGreaterThanOrEqual(1);
    expect(screen.getByRole('button', { name: /Change/ })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Confirm/ })).not.toBeInTheDocument();
  });

  it('Change button calls onReset', async () => {
    const onReset = vi.fn();
    render(
      <PriceDecisionBar sources={sources} preSelected="cl" status="accepted" onConfirm={() => {}} onReset={onReset} />
    );
    await userEvent.click(screen.getByRole('button', { name: /Change/ }));
    expect(onReset).toHaveBeenCalled();
  });

  it('renders skipped state with Undo button', () => {
    const onReset = vi.fn();
    render(
      <PriceDecisionBar sources={sources} status="skipped" onConfirm={() => {}} onSkip={() => {}} onReset={onReset} />
    );
    expect(screen.getByText(/Skipped/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Undo/ })).toBeInTheDocument();
  });

  it('Undo button calls onReset', async () => {
    const onReset = vi.fn();
    render(
      <PriceDecisionBar sources={sources} status="skipped" onConfirm={() => {}} onSkip={() => {}} onReset={onReset} />
    );
    await userEvent.click(screen.getByRole('button', { name: /Undo/ }));
    expect(onReset).toHaveBeenCalled();
  });

  it('disables all controls when disabled prop is true', () => {
    render(<PriceDecisionBar sources={sources} preSelected="cl" disabled onConfirm={() => {}} />);
    const buttons = screen.getAllByRole('button');
    buttons.forEach(btn => expect(btn).toBeDisabled());
    expect(screen.getByPlaceholderText('0.00')).toBeDisabled();
  });

  it('Enter in input triggers confirm', async () => {
    const onConfirm = vi.fn();
    render(<PriceDecisionBar sources={sources} onConfirm={onConfirm} />);
    const input = screen.getByPlaceholderText('0.00');
    await userEvent.type(input, '500.00{Enter}');
    expect(onConfirm).toHaveBeenCalledWith(50000, 'manual');
  });
});
