import { forwardRef, InputHTMLAttributes } from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { clsx } from 'clsx';

/**
 * Input variant definitions using CVA
 *
 * Provides consistent styling for text input fields with state variations
 */
const inputVariants = cva(
  'w-full px-4 py-2 text-sm text-[var(--text)] bg-[var(--surface-2)] border rounded-lg transition-colors focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-offset-[var(--surface-1)] disabled:opacity-50 disabled:cursor-not-allowed placeholder:text-[var(--text-muted)]',
  {
    variants: {
      state: {
        default: 'border-[var(--surface-2)] hover:border-[var(--brand-500)]/30 focus:border-[var(--brand-500)] focus:ring-[var(--brand-500)]/20 focus:shadow-[0_0_0_3px_rgba(99,102,241,0.1)]',
        error: 'border-[var(--danger)] focus:border-[var(--danger)] focus:ring-[var(--danger)]/20',
        success: 'border-green-500 focus:border-green-500 focus:ring-green-500/20',
      },
      inputSize: {
        sm: 'px-3 py-1.5 text-xs',
        md: 'px-4 py-2 text-sm',
        lg: 'px-5 py-3 text-base',
      },
    },
    defaultVariants: {
      state: 'default',
      inputSize: 'md',
    },
  }
);

/**
 * Input component props
 */
export interface InputProps
  extends Omit<InputHTMLAttributes<HTMLInputElement>, 'size'>,
    VariantProps<typeof inputVariants> {
  /** Input label */
  label?: string;
  /** Error message to display */
  error?: string;
  /** Helper text to display */
  helper?: string;
  /** Whether the field is required */
  required?: boolean;
  /** Left icon/addon */
  leftAddon?: React.ReactNode;
  /** Right icon/addon */
  rightAddon?: React.ReactNode;
}

/**
 * Input component for text entry
 *
 * Features:
 * - Type-safe variants using CVA
 * - Error and success states
 * - Optional label and helper text
 * - Support for addons/icons
 * - Accessible ARIA attributes
 *
 * @example
 * ```tsx
 * <Input label="Email" type="email" placeholder="you@example.com" />
 * <Input label="Name" error="Name is required" />
 * <Input label="Search" leftAddon={<SearchIcon />} />
 * ```
 */
const Input = forwardRef<HTMLInputElement, InputProps>(
  ({
    className,
    state,
    inputSize,
    label,
    error,
    helper,
    required,
    leftAddon,
    rightAddon,
    ...props
  }, ref) => {
    const inputState = error ? 'error' : state;
    const inputId = props.id || props.name;

    return (
      <div className="space-y-2">
        {label && (
          <label
            htmlFor={inputId}
            className="block text-xs text-[var(--text-muted)] mb-1"
          >
            {label}
            {required && <span className="text-danger ml-1">*</span>}
          </label>
        )}
        <div className="relative">
          {leftAddon && (
            <div className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--text-muted)] pointer-events-none">
              {leftAddon}
            </div>
          )}
          <input
            ref={ref}
            id={inputId}
            className={clsx(
              inputVariants({ state: inputState, inputSize }),
              leftAddon && 'pl-10',
              rightAddon && 'pr-10',
              className
            )}
            aria-invalid={error ? 'true' : undefined}
            aria-describedby={
              error
                ? `${inputId}-error`
                : helper
                ? `${inputId}-helper`
                : undefined
            }
            {...props}
          />
          {rightAddon && (
            <div className="absolute right-3 top-1/2 -translate-y-1/2 text-[var(--text-muted)] pointer-events-none">
              {rightAddon}
            </div>
          )}
        </div>
        {error && (
          <p id={`${inputId}-error`} className="text-xs text-[var(--danger)]">
            {error}
          </p>
        )}
        {helper && !error && (
          <p id={`${inputId}-helper`} className="text-xs text-[var(--text-muted)]">
            {helper}
          </p>
        )}
      </div>
    );
  }
);

Input.displayName = 'Input';

export default Input;
