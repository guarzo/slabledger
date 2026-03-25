import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import Button from '../../src/react/ui/Button';

describe('Button Component', () => {
  describe('Rendering', () => {
    it('renders with children text', () => {
      render(<Button>Click me</Button>);
      expect(screen.getByRole('button', { name: 'Click me' })).toBeInTheDocument();
    });

    it('renders with custom className', () => {
      render(<Button className="custom-class">Button</Button>);
      const button = screen.getByRole('button');
      expect(button).toHaveClass('custom-class');
    });

    it('renders as disabled when disabled prop is true', () => {
      render(<Button disabled>Disabled</Button>);
      expect(screen.getByRole('button')).toBeDisabled();
    });
  });

  describe('Variants', () => {
    it('renders primary variant with correct classes', () => {
      render(<Button variant="primary">Primary</Button>);
      const button = screen.getByRole('button');
      expect(button).toHaveClass('bg-[var(--brand-500)]');
      expect(button).toHaveClass('text-white');
    });

    it('renders secondary variant with correct classes', () => {
      render(<Button variant="secondary">Secondary</Button>);
      const button = screen.getByRole('button');
      expect(button).toHaveClass('bg-transparent');
      expect(button).toHaveClass('border-[var(--surface-2)]');
    });

    it('renders success variant with correct classes', () => {
      render(<Button variant="success">Success</Button>);
      const button = screen.getByRole('button');
      expect(button).toHaveClass('bg-[var(--success)]');
      expect(button).toHaveClass('text-white');
    });

    it('renders danger variant with correct classes', () => {
      render(<Button variant="danger">Danger</Button>);
      const button = screen.getByRole('button');
      expect(button).toHaveClass('bg-[var(--danger-bg)]');
      expect(button).toHaveClass('text-[var(--danger)]');
    });

    it('renders warning variant with correct classes', () => {
      render(<Button variant="warning">Warning</Button>);
      const button = screen.getByRole('button');
      expect(button).toHaveClass('bg-[var(--warning)]');
      expect(button).toHaveClass('text-white');
    });

    it('renders ghost variant with correct classes', () => {
      render(<Button variant="ghost">Ghost</Button>);
      const button = screen.getByRole('button');
      expect(button).toHaveClass('bg-transparent');
      expect(button).toHaveClass('hover:bg-[var(--surface-2)]');
    });

    it('renders link variant with correct classes', () => {
      render(<Button variant="link">Link</Button>);
      const button = screen.getByRole('button');
      expect(button).toHaveClass('text-[var(--brand-500)]');
      expect(button).toHaveClass('hover:underline');
    });
  });

  describe('Sizes', () => {
    it('renders small size with correct classes', () => {
      render(<Button size="sm">Small</Button>);
      const button = screen.getByRole('button');
      expect(button).toHaveClass('px-3');
      expect(button).toHaveClass('text-xs');
    });

    it('renders medium size (default) with correct classes', () => {
      render(<Button size="md">Medium</Button>);
      const button = screen.getByRole('button');
      expect(button).toHaveClass('px-4');
      expect(button).toHaveClass('text-sm');
    });

    it('renders large size with correct classes', () => {
      render(<Button size="lg">Large</Button>);
      const button = screen.getByRole('button');
      expect(button).toHaveClass('px-5');
      expect(button).toHaveClass('text-base');
    });

    it('renders icon size with correct classes', () => {
      render(<Button size="icon">🔍</Button>);
      const button = screen.getByRole('button');
      expect(button).toHaveClass('p-2');
      expect(button).toHaveClass('min-w-[40px]');
    });
  });

  describe('Full Width', () => {
    it('renders full width when fullWidth is true', () => {
      render(<Button fullWidth>Full Width</Button>);
      expect(screen.getByRole('button')).toHaveClass('w-full');
    });

    it('does not render full width by default', () => {
      render(<Button>Normal Width</Button>);
      expect(screen.getByRole('button')).not.toHaveClass('w-full');
    });
  });

  describe('Loading State', () => {
    it('shows loading spinner when loading is true', () => {
      render(<Button loading>Loading</Button>);
      const button = screen.getByRole('button');
      expect(button).toBeDisabled();
      expect(screen.getByText('Loading...')).toBeInTheDocument();
      expect(button.querySelector('svg')).toBeInTheDocument();
    });

    it('disables button when loading', () => {
      render(<Button loading>Loading</Button>);
      expect(screen.getByRole('button')).toBeDisabled();
    });

    it('hides children content when loading', () => {
      render(<Button loading>Click me</Button>);
      expect(screen.queryByText('Click me')).not.toBeInTheDocument();
    });

    it('shows loading spinner without text when no children', () => {
      render(<Button loading />);
      const button = screen.getByRole('button');
      expect(button.querySelector('svg')).toBeInTheDocument();
      expect(screen.queryByText('Loading...')).not.toBeInTheDocument();
    });
  });

  describe('Icons', () => {
    it('renders icon on left side with icon prop', () => {
      const icon = <span data-testid="icon">🔍</span>;
      render(<Button icon={icon} iconPosition="left">Search</Button>);
      const button = screen.getByRole('button');
      const iconElement = screen.getByTestId('icon');
      const textElement = screen.getByText('Search');

      expect(button).toContainElement(iconElement);
      expect(button).toContainElement(textElement);
      // Icon should appear before text in DOM order
      expect(button.innerHTML.indexOf('icon')).toBeLessThan(button.innerHTML.indexOf('Search'));
    });

    it('renders icon on right side with iconPosition="right"', () => {
      const icon = <span data-testid="icon">→</span>;
      render(<Button icon={icon} iconPosition="right">Next</Button>);
      const button = screen.getByRole('button');
      const iconElement = screen.getByTestId('icon');
      const textElement = screen.getByText('Next');

      expect(button).toContainElement(iconElement);
      expect(button).toContainElement(textElement);
      // Text should appear before icon in DOM order
      expect(button.innerHTML.indexOf('Next')).toBeLessThan(button.innerHTML.indexOf('icon'));
    });

    it('renders icon on left by default', () => {
      const icon = <span data-testid="left-icon">←</span>;
      render(<Button icon={icon}>Back</Button>);
      expect(screen.getByTestId('left-icon')).toBeInTheDocument();
      expect(screen.getByText('Back')).toBeInTheDocument();
    });

    it('icon has aria-hidden attribute', () => {
      const icon = <span data-testid="icon">🔍</span>;
      render(<Button icon={icon}>Search</Button>);
      const iconWrapper = screen.getByTestId('icon').parentElement;
      expect(iconWrapper).toHaveAttribute('aria-hidden', 'true');
    });
  });

  describe('Interactions', () => {
    it('calls onClick handler when clicked', async () => {
      const handleClick = vi.fn();
      const user = userEvent.setup();

      render(<Button onClick={handleClick}>Click me</Button>);
      await user.click(screen.getByRole('button'));

      expect(handleClick).toHaveBeenCalledTimes(1);
    });

    it('does not call onClick when disabled', async () => {
      const handleClick = vi.fn();
      const user = userEvent.setup();

      render(<Button onClick={handleClick} disabled>Disabled</Button>);
      // userEvent respects the disabled attribute, so click won't trigger
      const button = screen.getByRole('button');
      await user.click(button);

      expect(handleClick).not.toHaveBeenCalled();
    });

    it('does not call onClick when loading', async () => {
      const handleClick = vi.fn();
      const user = userEvent.setup();

      render(<Button onClick={handleClick} loading>Loading</Button>);
      const button = screen.getByRole('button');
      await user.click(button);

      expect(handleClick).not.toHaveBeenCalled();
    });

    it('forwards ref to button element', () => {
      const ref = vi.fn();
      render(<Button ref={ref}>Button</Button>);
      expect(ref).toHaveBeenCalledWith(expect.any(HTMLButtonElement));
    });
  });

  describe('Accessibility', () => {
    it('has proper button role', () => {
      render(<Button>Accessible</Button>);
      expect(screen.getByRole('button')).toBeInTheDocument();
    });

    it('respects custom type attribute', () => {
      render(<Button type="submit">Submit</Button>);
      expect(screen.getByRole('button')).toHaveAttribute('type', 'submit');
    });

    it('has focus ring classes', () => {
      render(<Button>Focus me</Button>);
      const button = screen.getByRole('button');
      expect(button).toHaveClass('focus:outline-none');
      expect(button).toHaveClass('focus:ring-2');
    });

    it('has disabled styles when disabled', () => {
      render(<Button disabled>Disabled</Button>);
      const button = screen.getByRole('button');
      expect(button).toHaveClass('disabled:opacity-40');
      expect(button).toHaveClass('disabled:cursor-not-allowed');
    });

    it('meets minimum touch target size (44px)', () => {
      render(<Button size="lg">Touch target</Button>);
      const button = screen.getByRole('button');
      expect(button).toHaveClass('min-h-[44px]');
    });
  });

  describe('Combined Props', () => {
    it('renders with multiple props correctly', () => {
      const handleClick = vi.fn();
      const icon = <span data-testid="icon">★</span>;

      render(
        <Button
          variant="success"
          size="lg"
          fullWidth
          icon={icon}
          onClick={handleClick}
          className="custom-class"
        >
          Complex Button
        </Button>
      );

      const button = screen.getByRole('button');
      expect(button).toHaveClass('bg-[var(--success)]');
      expect(button).toHaveClass('px-5');
      expect(button).toHaveClass('w-full');
      expect(button).toHaveClass('custom-class');
      expect(screen.getByTestId('icon')).toBeInTheDocument();
      expect(screen.getByText('Complex Button')).toBeInTheDocument();
    });

    it('prioritizes disabled over loading for click prevention', async () => {
      const handleClick = vi.fn();
      const user = userEvent.setup();

      render(
        <Button onClick={handleClick} disabled loading>
          Both disabled and loading
        </Button>
      );

      const button = screen.getByRole('button');
      await user.click(button);

      expect(handleClick).not.toHaveBeenCalled();
      expect(button).toBeDisabled();
    });
  });
});
