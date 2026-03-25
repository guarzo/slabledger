import { forwardRef, SelectHTMLAttributes } from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { clsx } from 'clsx';

/**
 * Select variant definitions using CVA
 *
 * Provides consistent styling for select dropdowns with state variations
 */
const selectVariants = cva(
  'w-full px-4 py-2 text-sm text-[var(--text)] bg-[var(--surface-2)] border rounded-lg transition-colors focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-offset-[var(--surface-1)] disabled:opacity-50 disabled:cursor-not-allowed appearance-none bg-no-repeat bg-right pr-10',
  {
    variants: {
      state: {
        default: 'border-[var(--surface-2)] hover:border-[var(--brand-500)]/30 focus:border-[var(--brand-500)] focus:ring-[var(--brand-500)]/20',
        error: 'border-red-500 focus:border-red-500 focus:ring-red-500/20',
        success: 'border-green-500 focus:border-green-500 focus:ring-green-500/20',
      },
      selectSize: {
        sm: 'px-3 py-1.5 text-xs',
        md: 'px-4 py-2 text-sm',
        lg: 'px-5 py-3 text-base',
      },
    },
    defaultVariants: {
      state: 'default',
      selectSize: 'md',
    },
  }
);

/**
 * Select component props
 */
export interface SelectProps
  extends Omit<SelectHTMLAttributes<HTMLSelectElement>, 'size'>,
    VariantProps<typeof selectVariants> {
  /** Select label */
  label?: string;
  /** Error message to display */
  error?: string;
  /** Helper text to display */
  helper?: string;
  /** Whether the field is required */
  required?: boolean;
  /** Option items */
  options?: Array<{ value: string | number; label: string; disabled?: boolean }>;
}

/**
 * Select dropdown component
 *
 * Features:
 * - Type-safe variants using CVA
 * - Error and success states
 * - Optional label and helper text
 * - Custom chevron icon
 * - Accessible ARIA attributes
 *
 * @example
 * ```tsx
 * <Select label="Country" options={countries} />
 * <Select label="Status" error="Please select a status">
 *   <option value="">Choose...</option>
 *   <option value="active">Active</option>
 * </Select>
 * ```
 */
const Select = forwardRef<HTMLSelectElement, SelectProps>(
  ({
    className,
    state,
    selectSize,
    label,
    error,
    helper,
    required,
    options,
    children,
    ...props
  }, ref) => {
    const selectState = error ? 'error' : state;
    const selectId = props.id || props.name;

    return (
      <div className="space-y-2">
        {label && (
          <label
            htmlFor={selectId}
            className="block text-xs text-[var(--text-muted)] mb-1"
          >
            {label}
            {required && <span className="text-danger ml-1">*</span>}
          </label>
        )}
        <div className="relative">
          <select
            ref={ref}
            id={selectId}
            className={clsx(
              selectVariants({ state: selectState, selectSize }),
              className
            )}
            style={{
              backgroundImage: `url("data:image/svg+xml,%3csvg xmlns='http://www.w3.org/2000/svg' fill='none' viewBox='0 0 20 20'%3e%3cpath stroke='%236b7280' stroke-linecap='round' stroke-linejoin='round' stroke-width='1.5' d='M6 8l4 4 4-4'/%3e%3c/svg%3e")`,
              backgroundPosition: 'right 0.5rem center',
              backgroundSize: '1.5em 1.5em',
            }}
            aria-invalid={error ? 'true' : undefined}
            aria-describedby={
              error
                ? `${selectId}-error`
                : helper
                ? `${selectId}-helper`
                : undefined
            }
            {...props}
          >
            {options
              ? options.map((option) => (
                  <option
                    key={option.value}
                    value={option.value}
                    disabled={option.disabled}
                  >
                    {option.label}
                  </option>
                ))
              : children}
          </select>
        </div>
        {error && (
          <p id={`${selectId}-error`} className="text-xs text-[var(--danger)]">
            {error}
          </p>
        )}
        {helper && !error && (
          <p id={`${selectId}-helper`} className="text-xs text-[var(--text-muted)]">
            {helper}
          </p>
        )}
      </div>
    );
  }
);

Select.displayName = 'Select';

export default Select;
