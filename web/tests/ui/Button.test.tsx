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
      expect(screen.getByRole('button')).toHaveClass('custom-class');
    });

    it('renders as disabled when disabled prop is true', () => {
      render(<Button disabled>Disabled</Button>);
      expect(screen.getByRole('button')).toBeDisabled();
    });

    it('defaults to type="button"', () => {
      render(<Button>Default</Button>);
      expect(screen.getByRole('button')).toHaveAttribute('type', 'button');
    });

    it('respects custom type attribute', () => {
      render(<Button type="submit">Submit</Button>);
      expect(screen.getByRole('button')).toHaveAttribute('type', 'submit');
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
    it('renders icon before children', () => {
      const icon = <span data-testid="icon">🔍</span>;
      render(<Button icon={icon}>Search</Button>);
      const button = screen.getByRole('button');
      expect(button).toContainElement(screen.getByTestId('icon'));
      expect(button).toContainElement(screen.getByText('Search'));
      expect(button.innerHTML.indexOf('icon')).toBeLessThan(button.innerHTML.indexOf('Search'));
    });

    it('icon wrapper has aria-hidden attribute', () => {
      const icon = <span data-testid="icon">🔍</span>;
      render(<Button icon={icon}>Search</Button>);
      expect(screen.getByTestId('icon').parentElement).toHaveAttribute('aria-hidden', 'true');
    });
  });

  describe('Keyboard Hint', () => {
    it('renders kbd chip when kbd prop is provided', () => {
      render(<Button kbd="Esc">Cancel</Button>);
      expect(screen.getByText('Esc')).toBeInTheDocument();
    });

    it('does not render kbd chip when not provided', () => {
      render(<Button>Save</Button>);
      expect(screen.getByRole('button').querySelectorAll('span')).toHaveLength(0);
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
      await user.click(screen.getByRole('button'));
      expect(handleClick).not.toHaveBeenCalled();
    });

    it('does not call onClick when loading', async () => {
      const handleClick = vi.fn();
      const user = userEvent.setup();
      render(<Button onClick={handleClick} loading>Loading</Button>);
      await user.click(screen.getByRole('button'));
      expect(handleClick).not.toHaveBeenCalled();
    });

    it('forwards ref to button element', () => {
      const ref = vi.fn();
      render(<Button ref={ref}>Button</Button>);
      expect(ref).toHaveBeenCalledWith(expect.any(HTMLButtonElement));
    });
  });

  describe('Combined Props', () => {
    it('renders with multiple props correctly', () => {
      const icon = <span data-testid="icon">★</span>;
      render(
        <Button variant="success" size="lg" fullWidth icon={icon} className="custom-class">
          Complex Button
        </Button>
      );
      const button = screen.getByRole('button');
      expect(button).toHaveClass('custom-class');
      expect(screen.getByTestId('icon')).toBeInTheDocument();
      expect(screen.getByText('Complex Button')).toBeInTheDocument();
    });

    it('prioritizes disabled over loading for click prevention', async () => {
      const handleClick = vi.fn();
      const user = userEvent.setup();
      render(<Button onClick={handleClick} disabled loading>Both</Button>);
      const button = screen.getByRole('button');
      await user.click(button);
      expect(handleClick).not.toHaveBeenCalled();
      expect(button).toBeDisabled();
    });
  });
});
