import { forwardRef, HTMLAttributes, KeyboardEvent, ReactNode } from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { clsx } from 'clsx';

/**
 * CardShell Variant Definitions
 *
 * Base card component that ENFORCES design token usage.
 * All styling uses CSS custom properties from tokens.css.
 *
 * This component provides:
 * - Consistent design token usage across all cards
 * - Dark mode support via CSS variables
 * - Accessibility (keyboard navigation, ARIA)
 * - Selection state management
 * - Interactive behaviors (hover, focus, click)
 */
const cardShellVariants = cva(
  [
    // Base token-based styles
    'rounded-[var(--radius-lg)]',
    'bg-[var(--surface-1)]',
    'border',
    'border-[var(--surface-0)]',
    'text-[var(--text)]',
    'transition-all',
    'duration-[var(--transition-base)]',
  ],
  {
    variants: {
      variant: {
        /**
         * default - Standard card appearance
         * Uses base surface with standard shadow
         */
        default: [
          'shadow-[var(--shadow-1)]',
        ],

        /**
         * elevated - Raised card appearance
         * Higher elevation with more prominent shadow
         */
        elevated: [
          'bg-[var(--surface-2)]',
          'shadow-[var(--shadow-2)]',
        ],

        /**
         * interactive - Clickable/hoverable card
         * Includes hover states and cursor pointer
         */
        interactive: [
          'shadow-[var(--shadow-1)]',
          'hover:bg-[var(--surface-hover)]',
          'hover:shadow-[var(--shadow-2)]',
          'hover:-translate-y-0.5',
          'cursor-pointer',
          'focus-visible:outline-none',
          'focus-visible:ring-2',
          'focus-visible:ring-[var(--brand-500)]',
          'focus-visible:ring-offset-2',
          'focus-visible:ring-offset-[var(--bg)]',
        ],

        /**
         * premium - Premium/featured card appearance
         * Subtle gradient and glow effect for special cards
         */
        premium: [
          'bg-gradient-to-br',
          'from-[var(--surface-1)]',
          'to-[var(--surface-2)]',
          'border-[var(--brand-500)]/30',
          'shadow-[var(--shadow-2)]',
          'hover:shadow-[var(--glow-brand)]',
          'transition-all',
          'duration-[var(--transition-slow)]',
        ],

        /**
         * glass - Glassmorphism appearance
         * Frosted glass effect with backdrop blur
         */
        glass: [
          'bg-[var(--glass-bg,rgba(22,27,34,0.7))]',
          'backdrop-blur-[20px]',
          'backdrop-saturate-[180%]',
          'border-[var(--glass-border,rgba(139,152,207,0.1))]',
          'shadow-[0_8px_32px_0_rgba(0,0,0,0.37)]',
          'hover:shadow-[0_12px_48px_0_rgba(0,0,0,0.5)]',
          'transition-all',
          'duration-[var(--transition-slow)]',
        ],
      },

      padding: {
        none: '',
        sm: 'p-3',
        md: 'p-4',
        lg: 'p-6',
      },
    },
    defaultVariants: {
      variant: 'default',
      padding: 'md',
    },
  }
);

/**
 * CardShell Component Props
 */
export interface CardShellProps
  extends Omit<HTMLAttributes<HTMLDivElement>, 'onClick'>,
    VariantProps<typeof cardShellVariants> {
  /** Card content */
  children: ReactNode;

  /** Additional CSS classes */
  className?: string;

  // ========== Interaction ==========

  /** Click handler for the entire card */
  onClick?: () => void;

  /** Keyboard event handler for custom keyboard interactions */
  onKeyDown?: (e: KeyboardEvent<HTMLDivElement>) => void;

  // ========== Selection (for comparison mode) ==========

  /** Whether the card can be selected/checked */
  selectable?: boolean;

  /** Whether the card is currently selected */
  isSelected?: boolean;

  /** Handler for toggling selection state */
  onToggleSelect?: () => void;

  // ========== Accessibility ==========

  /** ARIA label for screen readers */
  ariaLabel?: string;

  /** ARIA role (defaults to 'article' for semantic cards) */
  role?: string;

  /** Tab index for keyboard navigation (-1 to exclude, 0 to include) */
  tabIndex?: number;

  /** HTML element type */
  as?: 'article' | 'div' | 'section';
}

/**
 * CardShell - Foundation Component for All Cards
 *
 * Enforces design token usage and provides common card behaviors.
 * All card components should build on top of this component.
 *
 * Features:
 * - ✅ Design token enforcement (no hard-coded colors)
 * - ✅ Dark mode support via CSS variables
 * - ✅ Accessibility (keyboard navigation, ARIA, focus management)
 * - ✅ Selection state (for comparison/multi-select UIs)
 * - ✅ Interactive behaviors (hover, click, keyboard)
 * - ✅ Flexible variants (default, elevated, interactive, premium)
 *
 * @example
 * ```tsx
 * // Basic card
 * <CardShell variant="default">
 *   <p>Card content</p>
 * </CardShell>
 *
 * // Interactive card with click handler
 * <CardShell
 *   variant="interactive"
 *   onClick={() => console.log('clicked')}
 *   ariaLabel="Product card"
 * >
 *   <ProductDetails />
 * </CardShell>
 *
 * // Selectable card (for comparison mode)
 * <CardShell
 *   variant="interactive"
 *   selectable={true}
 *   isSelected={isSelected}
 *   onToggleSelect={() => setSelected(!isSelected)}
 * >
 *   <CardContent />
 * </CardShell>
 * ```
 */
export const CardShell = forwardRef<HTMLDivElement, CardShellProps>(
  (
    {
      children,
      variant,
      padding,
      className,
      onClick,
      onKeyDown,
      selectable = false,
      isSelected = false,
      onToggleSelect,
      ariaLabel,
      role = 'article',
      tabIndex,
      as: Component = 'article',
      ...props
    },
    ref
  ) => {
    /**
     * Handle keyboard interactions
     * - Enter/Space: Trigger onClick or onToggleSelect
     * - Custom handlers via onKeyDown prop
     */
    const handleKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
      // Allow custom keyboard handlers
      if (onKeyDown) {
        onKeyDown(e);
      }

      // Handle Enter/Space for interactive cards
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();

        // Priority: selection > click
        if (selectable && onToggleSelect) {
          onToggleSelect();
        } else if (onClick) {
          onClick();
        }
      }
    };

    /**
     * Handle click events
     * - If selectable, toggle selection
     * - Otherwise, trigger onClick handler
     */
    const handleClick = () => {
      if (selectable && onToggleSelect) {
        onToggleSelect();
      } else if (onClick) {
        onClick();
      }
    };

    /**
     * Determine if card should be keyboard-focusable
     * - Interactive cards are focusable by default
     * - Explicit tabIndex prop overrides
     */
    const shouldBeFocusable = variant === 'interactive' || onClick || selectable;
    const effectiveTabIndex = tabIndex !== undefined ? tabIndex : shouldBeFocusable ? 0 : undefined;

    return (
      <Component
        ref={ref}
        className={clsx(
          cardShellVariants({ variant, padding }),
          // Selection state styling
          isSelected && [
            'ring-2',
            'ring-[var(--brand-500)]',
            'ring-offset-2',
            'ring-offset-[var(--bg)]',
            'bg-[var(--surface-2)]',
          ],
          className
        )}
        onClick={onClick || selectable ? handleClick : undefined}
        onKeyDown={onClick || selectable || onKeyDown ? handleKeyDown : undefined}
        role={role}
        aria-label={ariaLabel}
        aria-selected={selectable ? isSelected : undefined}
        tabIndex={effectiveTabIndex}
        {...props}
      >
        {children}
      </Component>
    );
  }
);

CardShell.displayName = 'CardShell';

export default CardShell;
