import { clsx } from 'clsx';
import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from 'react';
import styles from './Button.module.css';

export type ButtonVariant =
  | 'primary'
  | 'success'
  | 'secondary'
  | 'danger'
  | 'ghost'
  | 'ai'
  | 'gold'
  | 'fab';

export type ButtonSize = 'sm' | 'md' | 'lg';

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  icon?: ReactNode;
  kbd?: string;
  loading?: boolean;
  fullWidth?: boolean;
}

const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ variant = 'primary', size = 'md', icon, kbd, loading, fullWidth, children, className, disabled, ...rest }, ref) => {
    return (
      <button
        ref={ref}
        type="button"
        className={clsx(
          styles.btn,
          styles[`v-${variant}`],
          styles[`s-${size}`],
          fullWidth && styles.full,
          className,
        )}
        disabled={disabled || loading}
        {...rest}
      >
        {loading ? (
          <>
            <Spinner size={size} />
            {children && <span>Loading...</span>}
          </>
        ) : (
          <>
            {icon && <span className={styles.icon} aria-hidden="true">{icon}</span>}
            {children}
            {kbd && <span className={styles.kbd}>{kbd}</span>}
          </>
        )}
      </button>
    );
  },
);
Button.displayName = 'Button';

export default Button;

function Spinner({ size = 'md' }: { size?: ButtonSize }) {
  const px = size === 'sm' ? 12 : size === 'lg' ? 20 : 16;
  return (
    <svg
      className={styles.spinner}
      style={{ width: px, height: px }}
      xmlns="http://www.w3.org/2000/svg"
      fill="none"
      viewBox="0 0 24 24"
      aria-hidden="true"
    >
      <circle className={styles.spinnerTrack} cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
      <path
        className={styles.spinnerHead}
        fill="currentColor"
        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
      />
    </svg>
  );
}
