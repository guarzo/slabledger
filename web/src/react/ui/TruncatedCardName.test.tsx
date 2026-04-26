import { render, screen } from '@testing-library/react';
import TruncatedCardName from './TruncatedCardName';

describe('TruncatedCardName', () => {
  it('renders the full text in the visible element', () => {
    render(<TruncatedCardName name="MEGA CHARIZARD X & SPECIAL ILLUSTRATION RARE" />);
    expect(
      screen.getByText('MEGA CHARIZARD X & SPECIAL ILLUSTRATION RARE'),
    ).toBeInTheDocument();
  });

  it('exposes full text via title attribute for native tooltip', () => {
    render(<TruncatedCardName name="Pikachu Promo SWSH039" />);
    const span = screen.getByText('Pikachu Promo SWSH039');
    expect(span).toHaveAttribute('title', 'Pikachu Promo SWSH039');
  });

  it('applies clamp class so text truncates at 2 lines', () => {
    render(<TruncatedCardName name="Long card name that should clamp" />);
    const span = screen.getByText('Long card name that should clamp');
    expect(span.className).toMatch(/line-clamp-2/);
  });

  it('passes through extra className', () => {
    render(<TruncatedCardName name="X" className="text-red-500" />);
    const span = screen.getByText('X');
    expect(span.className).toContain('text-red-500');
  });
});
