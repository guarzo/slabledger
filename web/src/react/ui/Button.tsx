import { forwardRef, ButtonHTMLAttributes } from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { clsx } from 'clsx';
import { useRipple } from '../hooks/useRipple';

/**
 * Button variant definitions using CVA
 *
 * CVA provides type-safe variant composition with proper TypeScript inference
 * and better maintainability than manual className concatenation.
 */
const buttonVariants = cva(
  // Base styles applied to all buttons
  'inline-flex items-center justify-center gap-2 font-medium transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:opacity-40 disabled:cursor-not-allowed disabled:pointer-events-none',
  {
    variants: {
      variant: {
        primary: 'bg-[var(--brand-500)] text-white border border-[var(--brand-500)] hover:bg-[var(--brand-600)] hover:-translate-y-0.5 hover:shadow-sm focus:ring-[var(--brand-500)]',
        secondary: 'bg-transparent text-[var(--text)] border border-[var(--surface-2)] hover:bg-[var(--surface-2)] focus:ring-[var(--brand-500)]',
        success: 'bg-[var(--success)] text-white border border-[var(--success)] hover:brightness-110 hover:-translate-y-0.5 hover:shadow-sm focus:ring-[var(--success)]',
        danger: 'bg-[var(--danger-bg)] text-[var(--danger)] border border-[var(--danger-border)] hover:bg-[var(--danger-subtle)] focus:ring-[var(--danger)]',
        warning: 'bg-[var(--warning)] text-white border border-[var(--warning)] hover:brightness-110 focus:ring-[var(--warning)]',
        ghost: 'bg-transparent text-[var(--text-muted)] border border-transparent hover:bg-[var(--surface-2)] hover:text-[var(--text)] focus:ring-[var(--brand-500)]',
        link: 'bg-transparent text-[var(--brand-500)] hover:text-[var(--brand-400)] underline-offset-4 hover:underline border-none focus:ring-[var(--brand-500)]',
      },
      size: {
        sm: 'px-3 py-2 text-xs rounded-md min-h-[36px]',
        md: 'px-4 py-2.5 text-sm rounded-md min-h-[40px]',
        lg: 'px-5 py-3 text-base rounded-lg min-h-[44px]',
        icon: 'p-2 rounded-md min-h-[40px] min-w-[40px]',
      },
      fullWidth: {
        true: 'w-full',
      },
    },
    defaultVariants: {
      variant: 'primary',
      size: 'md',
    },
  }
);

/**
 * Button component props
 *
 * Extends standard button props with variant props from CVA
 */
export interface ButtonProps
  extends ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  /** Show loading spinner */
  loading?: boolean;

  /** Icon to display */
  icon?: React.ReactNode;

  /** Icon position (only used if icon is provided) */
  iconPosition?: 'left' | 'right';

  /** Enable ripple effect on click (default: true) */
  ripple?: boolean;
}

/**
 * Reusable Button component with variants and sizes
 *
 * Features:
 * - Type-safe variants using CVA
 * - Loading state with spinner
 * - Icon support (left/right positioning)
 * - Full width option
 * - Accessibility features (ARIA, focus states)
 * - Touch-friendly minimum sizes (44px WCAG guideline)
 *
 * @example
 * ```tsx
 * <Button variant="primary" size="lg">Click Me</Button>
 * <Button variant="danger" loading>Deleting...</Button>
 * <Button variant="ghost" icon={<Icon />} iconPosition="left">Search</Button>
 * ```
 */
const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({
    className,
    variant,
    size,
    fullWidth,
    loading,
    icon,
    iconPosition = 'left',
    children,
    disabled,
    ripple = true,
    onClick,
    ...props
  }, ref) => {
    const displayIcon = icon;
    const finalIconPosition = iconPosition;

    // Ripple effect hook - uses CSS tokens from tokens.css
    // Light ripple for primary/success/danger/warning, dark ripple for secondary/ghost/outline
    const isLightRipple = variant === 'primary' || variant === 'success' || variant === 'danger' || variant === 'warning';
    const createRipple = useRipple({
      color: isLightRipple ? 'var(--ripple-light)' : 'var(--ripple-dark)',
      duration: 600,
    });

    // Handle click with ripple effect
    const handleClick = (e: React.MouseEvent<HTMLButtonElement>) => {
      if (ripple && !disabled && !loading) {
        createRipple(e);
      }
      onClick?.(e);
    };

    return (
      <button
        type="button"
        className={clsx(
          buttonVariants({ variant, size, fullWidth }),
          // Add relative and overflow-hidden for ripple effect
          ripple && 'relative overflow-hidden',
          className
        )}
        ref={ref}
        disabled={disabled || loading}
        onClick={handleClick}
        {...props}
      >
        {/* Button content wrapper - no positioning to avoid stacking context issues with axe-core */}
        <span className="inline-flex items-center justify-center gap-2">
          {loading ? (
            <>
              <Spinner size={size} />
              {children && <span>Loading...</span>}
            </>
          ) : (
            <>
              {displayIcon && finalIconPosition === 'left' && (
                <span className="flex-shrink-0" aria-hidden="true">
                  {displayIcon}
                </span>
              )}
              {children}
              {displayIcon && finalIconPosition === 'right' && (
                <span className="flex-shrink-0" aria-hidden="true">
                  {displayIcon}
                </span>
              )}
            </>
          )}
        </span>
      </button>
    );
  }
);

Button.displayName = 'Button';

export default Button;

/**
 * Spinner component props
 */
interface SpinnerProps {
  size?: 'sm' | 'md' | 'lg' | 'icon' | null;
}

/**
 * Loading spinner component
 *
 * Displays an accessible loading spinner with size variations
 */
function Spinner({ size = 'md' }: SpinnerProps) {
  const sizeClasses = {
    sm: 'w-3 h-3',
    md: 'w-4 h-4',
    lg: 'w-5 h-5',
    icon: 'w-4 h-4',
  };

  const spinnerSize = size || 'md';

  return (
    <svg
      className={clsx('animate-spin', sizeClasses[spinnerSize])}
      xmlns="http://www.w3.org/2000/svg"
      fill="none"
      viewBox="0 0 24 24"
      aria-hidden="true"
      role="status"
    >
      <circle
        className="opacity-25"
        cx="12"
        cy="12"
        r="10"
        stroke="currentColor"
        strokeWidth="4"
      />
      <path
        className="opacity-75"
        fill="currentColor"
        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
      />
    </svg>
  );
}
