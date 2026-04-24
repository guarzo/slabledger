import { render, screen, fireEvent } from '@testing-library/react';
import CardShell from './CardShell';
import styles from './CardShell.module.css';

describe('CardShell', () => {
  describe('Rendering', () => {
    it('renders children', () => {
      render(<CardShell>Hello</CardShell>);
      expect(screen.getByText('Hello')).toBeInTheDocument();
    });

    it('applies default variant/padding/radius classes', () => {
      const { container } = render(<CardShell>Content</CardShell>);
      const card = container.firstChild as HTMLElement;
      expect(card).toHaveClass(styles.card);
      expect(card).toHaveClass(styles['v-default']);
      expect(card).toHaveClass(styles['p-md']);
      expect(card).toHaveClass(styles['r-md']);
      expect(card).not.toHaveClass(styles.interactive);
    });

    it('renders as <div> by default', () => {
      const { container } = render(<CardShell>Content</CardShell>);
      expect((container.firstChild as HTMLElement).tagName).toBe('DIV');
    });
  });

  describe('Variants', () => {
    const variants = ['default', 'elevated', 'glass', 'premium', 'ai', 'data'] as const;

    it.each(variants)('applies v-%s class for variant "%s"', (variant) => {
      const { container } = render(<CardShell variant={variant}>x</CardShell>);
      expect(container.firstChild).toHaveClass(styles[`v-${variant}`]);
    });
  });

  describe('Padding', () => {
    const paddings = ['sm', 'md', 'lg', 'none'] as const;

    it.each(paddings)('applies p-%s class for padding "%s"', (padding) => {
      const { container } = render(<CardShell padding={padding}>x</CardShell>);
      expect(container.firstChild).toHaveClass(styles[`p-${padding}`]);
    });
  });

  describe('Radius', () => {
    const radii = ['sm', 'md', 'lg'] as const;

    it.each(radii)('applies r-%s class for radius "%s"', (radius) => {
      const { container } = render(<CardShell radius={radius}>x</CardShell>);
      expect(container.firstChild).toHaveClass(styles[`r-${radius}`]);
    });
  });

  describe('Interactive', () => {
    it('omits interactive class by default', () => {
      const { container } = render(<CardShell>x</CardShell>);
      expect(container.firstChild).not.toHaveClass(styles.interactive);
    });

    it('adds interactive class when interactive=true', () => {
      const { container } = render(<CardShell interactive>x</CardShell>);
      expect(container.firstChild).toHaveClass(styles.interactive);
    });

    it.each(['default', 'elevated', 'glass', 'premium', 'ai', 'data'] as const)(
      'supports interactive on variant "%s"',
      (variant) => {
        const { container } = render(
          <CardShell variant={variant} interactive>x</CardShell>,
        );
        const card = container.firstChild as HTMLElement;
        expect(card).toHaveClass(styles[`v-${variant}`]);
        expect(card).toHaveClass(styles.interactive);
      },
    );
  });

  describe('Polymorphism via `as`', () => {
    it('renders as <button> when as="button"', () => {
      const { container } = render(
        <CardShell as="button" interactive>Click</CardShell>,
      );
      expect((container.firstChild as HTMLElement).tagName).toBe('BUTTON');
    });

    it('forwards the `type="button"` attribute when rendered as a button', () => {
      const { container } = render(
        <CardShell as="button" type="button" interactive>Click</CardShell>,
      );
      expect(container.firstChild).toHaveAttribute('type', 'button');
    });

    it('renders as <section> when as="section"', () => {
      const { container } = render(<CardShell as="section">x</CardShell>);
      expect((container.firstChild as HTMLElement).tagName).toBe('SECTION');
    });
  });

  describe('Keyboard a11y shim (interactive div fallback)', () => {
    it('adds tabIndex=0 and role="button" on interactive div', () => {
      const { container } = render(
        <CardShell interactive onClick={() => {}}>x</CardShell>,
      );
      const card = container.firstChild as HTMLElement;
      expect(card).toHaveAttribute('tabindex', '0');
      expect(card).toHaveAttribute('role', 'button');
    });

    it('does NOT add the shim when `as="button"` is explicit', () => {
      const { container } = render(
        <CardShell as="button" type="button" interactive onClick={() => {}}>x</CardShell>,
      );
      const card = container.firstChild as HTMLElement;
      expect(card).not.toHaveAttribute('role', 'button');
      expect(card).not.toHaveAttribute('tabindex');
    });

    it('does NOT add the shim when interactive is false', () => {
      const { container } = render(
        <CardShell onClick={() => {}}>x</CardShell>,
      );
      const card = container.firstChild as HTMLElement;
      expect(card).not.toHaveAttribute('role');
      expect(card).not.toHaveAttribute('tabindex');
    });

    it('fires onClick on Enter key for an interactive div', () => {
      const onClick = vi.fn();
      const { container } = render(
        <CardShell interactive onClick={onClick}>x</CardShell>,
      );
      fireEvent.keyDown(container.firstChild as HTMLElement, { key: 'Enter' });
      expect(onClick).toHaveBeenCalledTimes(1);
    });

    it('fires onClick on Space key for an interactive div and preventDefaults', () => {
      const onClick = vi.fn();
      const { container } = render(
        <CardShell interactive onClick={onClick}>x</CardShell>,
      );
      const card = container.firstChild as HTMLElement;
      const event = new KeyboardEvent('keydown', { key: ' ', bubbles: true, cancelable: true });
      card.dispatchEvent(event);
      expect(onClick).toHaveBeenCalledTimes(1);
      expect(event.defaultPrevented).toBe(true);
    });

    it('respects caller-provided tabIndex/role', () => {
      const { container } = render(
        <CardShell interactive tabIndex={-1} role="link">x</CardShell>,
      );
      const card = container.firstChild as HTMLElement;
      expect(card).toHaveAttribute('tabindex', '-1');
      expect(card).toHaveAttribute('role', 'link');
    });

    it('invokes caller-provided onKeyDown before the shim', () => {
      const onClick = vi.fn();
      const onKeyDown = vi.fn();
      const { container } = render(
        <CardShell interactive onClick={onClick} onKeyDown={onKeyDown}>x</CardShell>,
      );
      fireEvent.keyDown(container.firstChild as HTMLElement, { key: 'Enter' });
      expect(onKeyDown).toHaveBeenCalledTimes(1);
      expect(onClick).toHaveBeenCalledTimes(1);
    });
  });

  describe('HTML passthrough', () => {
    it('forwards onClick', () => {
      const onClick = vi.fn();
      render(
        <CardShell onClick={onClick} data-testid="card">
          x
        </CardShell>,
      );
      fireEvent.click(screen.getByTestId('card'));
      expect(onClick).toHaveBeenCalledTimes(1);
    });

    it('forwards arbitrary attributes (aria-label, data-*, id)', () => {
      render(
        <CardShell aria-label="Stat" data-testid="card" id="my-card">
          x
        </CardShell>,
      );
      const card = screen.getByTestId('card');
      expect(card).toHaveAttribute('aria-label', 'Stat');
      expect(card).toHaveAttribute('id', 'my-card');
    });

    it('merges caller className after variant/padding/radius classes', () => {
      const { container } = render(
        <CardShell className="extra">x</CardShell>,
      );
      const card = container.firstChild as HTMLElement;
      expect(card).toHaveClass(styles.card);
      expect(card).toHaveClass('extra');
    });
  });
});
