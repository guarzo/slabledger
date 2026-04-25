import { render, screen } from '@testing-library/react';
import StatCard from './StatCard';

describe('StatCard', () => {
  describe('default (size="md")', () => {
    it('renders label and value', () => {
      render(<StatCard label="Revenue" value="$1,234.56" />);
      expect(screen.getByText('Revenue')).toBeInTheDocument();
      expect(screen.getByText('$1,234.56')).toBeInTheDocument();
    });

    it('uses text-xl font-bold for the value', () => {
      render(<StatCard label="Revenue" value="$1,234.56" />);
      const valueEl = screen.getByText('$1,234.56');
      expect(valueEl).toHaveClass('text-xl');
      expect(valueEl).toHaveClass('font-bold');
      expect(valueEl).toHaveClass('tabular-nums');
    });

    it('renders inside a card with padding p-3', () => {
      const { container } = render(<StatCard label="Revenue" value="$1,234.56" />);
      const card = container.firstChild as HTMLElement;
      expect(card).toHaveClass('p-3');
      expect(card).toHaveClass('rounded-xl');
    });

    it('produces no double or trailing whitespace in className', () => {
      const { container } = render(<StatCard label="Revenue" value="$1" />);
      const cls = (container.firstChild as HTMLElement).className;
      expect(cls).not.toMatch(/ {2,}/);
      expect(cls).not.toMatch(/\s$/);
    });
  });

  describe('size="lg"', () => {
    it('uses text-3xl font-extrabold for the value', () => {
      render(<StatCard label="Total Spent" value="$10,000.00" size="lg" />);
      const valueEl = screen.getByText('$10,000.00');
      expect(valueEl).toHaveClass('text-3xl');
      expect(valueEl).toHaveClass('font-extrabold');
      expect(valueEl).toHaveClass('tabular-nums');
    });

    it('uses elevated padding and stronger border', () => {
      const { container } = render(
        <StatCard label="Total Spent" value="$10,000.00" size="lg" />,
      );
      const card = container.firstChild as HTMLElement;
      expect(card).toHaveClass('p-5');
      expect(card).toHaveClass('border-[var(--surface-3)]');
    });

    it('does not impose a grid span on its parent', () => {
      const { container } = render(
        <StatCard label="Total Spent" value="$10,000.00" size="lg" />,
      );
      expect(container.firstChild).not.toHaveClass('col-span-2');
    });
  });

  describe('size="sm"', () => {
    it('renders without card chrome (no border, no surface bg)', () => {
      const { container } = render(
        <StatCard label="Sold" value="3" size="sm" />,
      );
      const root = container.firstChild as HTMLElement;
      expect(root).not.toHaveClass('rounded-xl');
      expect(root).not.toHaveClass('p-3');
      expect(root).not.toHaveClass('p-5');
      expect(root).not.toHaveClass('bg-[var(--surface-1)]');
      expect(root).toHaveClass('flex');
      expect(root).toHaveClass('gap-2');
      expect(root).toHaveClass('items-baseline');
    });

    it('renders the label as uppercase tracked micro-label', () => {
      render(<StatCard label="Sold" value="3" size="sm" />);
      const labelEl = screen.getByText('Sold');
      expect(labelEl).toHaveClass('uppercase');
      expect(labelEl).toHaveClass('tracking-wider');
      expect(labelEl).toHaveClass('text-xs');
    });

    it('renders the value with tabular-nums', () => {
      render(<StatCard label="Sold" value="3" size="sm" />);
      const valueEl = screen.getByText('3');
      expect(valueEl).toHaveClass('tabular-nums');
      expect(valueEl).toHaveClass('font-semibold');
    });
  });

  describe('color accent', () => {
    it('applies success color when color="green"', () => {
      render(<StatCard label="Net Profit" value="$1.00" color="green" />);
      expect(screen.getByText('$1.00')).toHaveClass('text-[var(--success)]');
    });

    it('applies danger color when color="red"', () => {
      render(<StatCard label="Net Profit" value="-$1.00" color="red" />);
      expect(screen.getByText('-$1.00')).toHaveClass('text-[var(--danger)]');
    });

    it('falls back to default text color when color is undefined', () => {
      render(<StatCard label="Net Profit" value="$0.00" />);
      expect(screen.getByText('$0.00')).toHaveClass('text-[var(--text)]');
    });

    it('color tokens still apply at size="sm"', () => {
      render(<StatCard label="Sold" value="3" color="green" size="sm" />);
      expect(screen.getByText('3')).toHaveClass('text-[var(--success)]');
    });

    it('color tokens still apply at size="lg"', () => {
      render(<StatCard label="Net Profit" value="$1" color="red" size="lg" />);
      expect(screen.getByText('$1')).toHaveClass('text-[var(--danger)]');
    });
  });
});
