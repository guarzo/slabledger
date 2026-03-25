import { render, screen, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import CardShell from './CardShell';

describe('CardShell', () => {
  describe('Rendering', () => {
    it('renders children correctly', () => {
      render(
        <CardShell>
          <div>Test Content</div>
        </CardShell>
      );
      expect(screen.getByText('Test Content')).toBeInTheDocument();
    });

    it('applies default variant classes', () => {
      const { container } = render(<CardShell>Content</CardShell>);
      const card = container.firstChild as HTMLElement;

      // Check for token-based classes
      expect(card).toHaveClass('rounded-[var(--radius-lg)]');
      expect(card).toHaveClass('bg-[var(--surface-1)]');
      expect(card).toHaveClass('border');
      expect(card).toHaveClass('border-[var(--surface-0)]');
    });

    it('applies elevated variant correctly', () => {
      const { container } = render(
        <CardShell variant="elevated">Content</CardShell>
      );
      const card = container.firstChild as HTMLElement;

      expect(card).toHaveClass('bg-[var(--surface-2)]');
      expect(card).toHaveClass('shadow-[var(--shadow-2)]');
    });

    it('applies interactive variant correctly', () => {
      const { container } = render(
        <CardShell variant="interactive" onClick={() => {}}>
          Content
        </CardShell>
      );
      const card = container.firstChild as HTMLElement;

      expect(card).toHaveClass('cursor-pointer');
      expect(card).toHaveClass('hover:bg-[var(--surface-hover)]');
    });

    it('applies premium variant correctly', () => {
      const { container } = render(
        <CardShell variant="premium">Content</CardShell>
      );
      const card = container.firstChild as HTMLElement;

      expect(card).toHaveClass('bg-gradient-to-br');
      expect(card).toHaveClass('from-[var(--surface-1)]');
      expect(card).toHaveClass('to-[var(--surface-2)]');
    });

    it('applies custom className', () => {
      const { container } = render(
        <CardShell className="custom-class">Content</CardShell>
      );
      const card = container.firstChild as HTMLElement;

      expect(card).toHaveClass('custom-class');
    });

    it('renders as article by default', () => {
      const { container } = render(<CardShell>Content</CardShell>);
      expect(container.firstChild?.nodeName).toBe('ARTICLE');
    });

    it('renders as div when as="div" is specified', () => {
      const { container } = render(<CardShell as="div">Content</CardShell>);
      expect(container.firstChild?.nodeName).toBe('DIV');
    });

    it('renders as section when as="section" is specified', () => {
      const { container } = render(<CardShell as="section">Content</CardShell>);
      expect(container.firstChild?.nodeName).toBe('SECTION');
    });
  });

  describe('Padding', () => {
    it('applies no padding when padding="none"', () => {
      const { container } = render(
        <CardShell padding="none">Content</CardShell>
      );
      const card = container.firstChild as HTMLElement;

      expect(card).not.toHaveClass('p-3');
      expect(card).not.toHaveClass('p-4');
      expect(card).not.toHaveClass('p-6');
    });

    it('applies small padding when padding="sm"', () => {
      const { container } = render(
        <CardShell padding="sm">Content</CardShell>
      );
      const card = container.firstChild as HTMLElement;

      expect(card).toHaveClass('p-3');
    });

    it('applies medium padding when padding="md" (default)', () => {
      const { container } = render(<CardShell>Content</CardShell>);
      const card = container.firstChild as HTMLElement;

      expect(card).toHaveClass('p-4');
    });

    it('applies large padding when padding="lg"', () => {
      const { container } = render(
        <CardShell padding="lg">Content</CardShell>
      );
      const card = container.firstChild as HTMLElement;

      expect(card).toHaveClass('p-6');
    });
  });

  describe('Click Interaction', () => {
    it('calls onClick when card is clicked', () => {
      const handleClick = vi.fn();
      render(
        <CardShell onClick={handleClick}>
          <div>Clickable Content</div>
        </CardShell>
      );

      const card = screen.getByText('Clickable Content').parentElement!;
      fireEvent.click(card);

      expect(handleClick).toHaveBeenCalledTimes(1);
    });

    it('does not have click handler when onClick is not provided', () => {
      const { container } = render(<CardShell>Content</CardShell>);
      const card = container.firstChild as HTMLElement;

      // Card should not have onClick attribute
      expect(card.onclick).toBeNull();
    });
  });

  describe('Selection State', () => {
    it('shows selection ring when isSelected is true', () => {
      const { container } = render(
        <CardShell selectable isSelected>
          Content
        </CardShell>
      );
      const card = container.firstChild as HTMLElement;

      expect(card).toHaveClass('ring-2');
      expect(card).toHaveClass('ring-[var(--brand-500)]');
      expect(card).toHaveClass('bg-[var(--surface-2)]');
    });

    it('does not show selection ring when isSelected is false', () => {
      const { container } = render(
        <CardShell selectable isSelected={false}>
          Content
        </CardShell>
      );
      const card = container.firstChild as HTMLElement;

      expect(card).not.toHaveClass('ring-2');
    });

    it('calls onToggleSelect when selectable card is clicked', () => {
      const handleToggleSelect = vi.fn();
      render(
        <CardShell selectable onToggleSelect={handleToggleSelect}>
          <div>Selectable Content</div>
        </CardShell>
      );

      const card = screen.getByText('Selectable Content').parentElement!;
      fireEvent.click(card);

      expect(handleToggleSelect).toHaveBeenCalledTimes(1);
    });

    it('prioritizes onToggleSelect over onClick when selectable', () => {
      const handleClick = vi.fn();
      const handleToggleSelect = vi.fn();

      render(
        <CardShell
          selectable
          onClick={handleClick}
          onToggleSelect={handleToggleSelect}
        >
          <div>Content</div>
        </CardShell>
      );

      const card = screen.getByText('Content').parentElement!;
      fireEvent.click(card);

      expect(handleToggleSelect).toHaveBeenCalledTimes(1);
      expect(handleClick).not.toHaveBeenCalled();
    });

    it('sets aria-selected when selectable', () => {
      const { container, rerender } = render(
        <CardShell selectable isSelected={false}>
          Content
        </CardShell>
      );
      let card = container.firstChild as HTMLElement;

      expect(card).toHaveAttribute('aria-selected', 'false');

      rerender(
        <CardShell selectable isSelected>
          Content
        </CardShell>
      );
      card = container.firstChild as HTMLElement;

      expect(card).toHaveAttribute('aria-selected', 'true');
    });

    it('does not set aria-selected when not selectable', () => {
      const { container } = render(<CardShell>Content</CardShell>);
      const card = container.firstChild as HTMLElement;

      expect(card).not.toHaveAttribute('aria-selected');
    });
  });

  describe('Keyboard Navigation', () => {
    it('triggers onClick when Enter key is pressed', async () => {
      const user = userEvent.setup();
      const handleClick = vi.fn();

      render(
        <CardShell onClick={handleClick}>
          <div>Keyboard Content</div>
        </CardShell>
      );

      const card = screen.getByText('Keyboard Content').parentElement!;
      card.focus();
      await user.keyboard('{Enter}');

      expect(handleClick).toHaveBeenCalledTimes(1);
    });

    it('triggers onClick when Space key is pressed', async () => {
      const user = userEvent.setup();
      const handleClick = vi.fn();

      render(
        <CardShell onClick={handleClick}>
          <div>Keyboard Content</div>
        </CardShell>
      );

      const card = screen.getByText('Keyboard Content').parentElement!;
      card.focus();
      await user.keyboard(' ');

      expect(handleClick).toHaveBeenCalledTimes(1);
    });

    it('triggers onToggleSelect when Enter is pressed on selectable card', async () => {
      const user = userEvent.setup();
      const handleToggleSelect = vi.fn();

      render(
        <CardShell selectable onToggleSelect={handleToggleSelect}>
          <div>Selectable Content</div>
        </CardShell>
      );

      const card = screen.getByText('Selectable Content').parentElement!;
      card.focus();
      await user.keyboard('{Enter}');

      expect(handleToggleSelect).toHaveBeenCalledTimes(1);
    });

    it('triggers onToggleSelect when Space is pressed on selectable card', async () => {
      const user = userEvent.setup();
      const handleToggleSelect = vi.fn();

      render(
        <CardShell selectable onToggleSelect={handleToggleSelect}>
          <div>Selectable Content</div>
        </CardShell>
      );

      const card = screen.getByText('Selectable Content').parentElement!;
      card.focus();
      await user.keyboard(' ');

      expect(handleToggleSelect).toHaveBeenCalledTimes(1);
    });

    it('calls custom onKeyDown handler', async () => {
      const user = userEvent.setup();
      const handleKeyDown = vi.fn();

      render(
        <CardShell onKeyDown={handleKeyDown} variant="interactive">
          <div>Content</div>
        </CardShell>
      );

      const card = screen.getByText('Content').parentElement!;
      card.focus();
      await user.keyboard('{ArrowRight}');

      expect(handleKeyDown).toHaveBeenCalled();
    });
  });

  describe('Accessibility', () => {
    it('sets aria-label when provided', () => {
      const { container } = render(
        <CardShell ariaLabel="Product card">Content</CardShell>
      );
      const card = container.firstChild as HTMLElement;

      expect(card).toHaveAttribute('aria-label', 'Product card');
    });

    it('sets custom role when provided', () => {
      const { container } = render(
        <CardShell role="region">Content</CardShell>
      );
      const card = container.firstChild as HTMLElement;

      expect(card).toHaveAttribute('role', 'region');
    });

    it('defaults to article role', () => {
      const { container } = render(<CardShell>Content</CardShell>);
      const card = container.firstChild as HTMLElement;

      expect(card).toHaveAttribute('role', 'article');
    });

    it('is focusable when interactive', () => {
      const { container } = render(
        <CardShell variant="interactive">Content</CardShell>
      );
      const card = container.firstChild as HTMLElement;

      expect(card).toHaveAttribute('tabIndex', '0');
    });

    it('is focusable when onClick is provided', () => {
      const { container } = render(
        <CardShell onClick={() => {}}>Content</CardShell>
      );
      const card = container.firstChild as HTMLElement;

      expect(card).toHaveAttribute('tabIndex', '0');
    });

    it('is focusable when selectable', () => {
      const { container } = render(
        <CardShell selectable>Content</CardShell>
      );
      const card = container.firstChild as HTMLElement;

      expect(card).toHaveAttribute('tabIndex', '0');
    });

    it('is not focusable by default', () => {
      const { container } = render(<CardShell>Content</CardShell>);
      const card = container.firstChild as HTMLElement;

      expect(card).not.toHaveAttribute('tabIndex');
    });

    it('respects explicit tabIndex prop', () => {
      const { container } = render(
        <CardShell tabIndex={-1}>Content</CardShell>
      );
      const card = container.firstChild as HTMLElement;

      expect(card).toHaveAttribute('tabIndex', '-1');
    });

    it('allows explicit tabIndex to override default focusable behavior', () => {
      const { container } = render(
        <CardShell variant="interactive" tabIndex={-1}>
          Content
        </CardShell>
      );
      const card = container.firstChild as HTMLElement;

      expect(card).toHaveAttribute('tabIndex', '-1');
    });
  });

  describe('Focus Management', () => {
    it('can receive focus programmatically', () => {
      const { container } = render(
        <CardShell variant="interactive">Content</CardShell>
      );
      const card = container.firstChild as HTMLElement;

      card.focus();
      expect(document.activeElement).toBe(card);
    });

    it('shows focus ring on interactive variant', () => {
      const { container } = render(
        <CardShell variant="interactive" onClick={() => {}}>
          Content
        </CardShell>
      );
      const card = container.firstChild as HTMLElement;

      expect(card).toHaveClass('focus-visible:outline-none');
      expect(card).toHaveClass('focus-visible:ring-2');
      expect(card).toHaveClass('focus-visible:ring-[var(--brand-500)]');
    });
  });

  describe('Design Token Enforcement', () => {
    it('uses only CSS variable-based classes (no hard-coded colors)', () => {
      const { container } = render(
        <CardShell variant="elevated">Content</CardShell>
      );
      const card = container.firstChild as HTMLElement;
      const classString = card.className;

      // Should NOT contain hard-coded Tailwind colors
      expect(classString).not.toMatch(/bg-white/);
      expect(classString).not.toMatch(/bg-gray-/);
      expect(classString).not.toMatch(/text-gray-/);
      expect(classString).not.toMatch(/border-gray-/);

      // Should contain token-based classes
      expect(classString).toMatch(/bg-\[var\(--surface-/);
      expect(classString).toMatch(/border-\[var\(--surface-/);
    });

    it('uses design tokens for all variants', () => {
      const variants = ['default', 'elevated', 'interactive', 'premium'] as const;

      variants.forEach(variant => {
        const { container } = render(
          <CardShell variant={variant}>Content</CardShell>
        );
        const card = container.firstChild as HTMLElement;
        const classString = card.className;

        // All variants must use tokens
        expect(classString).toMatch(/\[var\(--/);
      });
    });
  });

  describe('Props Forwarding', () => {
    it('forwards HTML attributes', () => {
      const { container } = render(
        <CardShell data-testid="custom-card" id="card-123">
          Content
        </CardShell>
      );
      const card = container.firstChild as HTMLElement;

      expect(card).toHaveAttribute('data-testid', 'custom-card');
      expect(card).toHaveAttribute('id', 'card-123');
    });

    it('accepts ref', () => {
      let refValue: HTMLElement | null = null;
      const ref = (node: HTMLElement | null) => {
        refValue = node;
      };

      render(
        <CardShell ref={ref}>Content</CardShell>
      );

      expect(refValue).toBeInstanceOf(HTMLElement);
      expect(refValue).not.toBeNull();
      expect(refValue!.textContent).toBe('Content');
    });
  });
});
