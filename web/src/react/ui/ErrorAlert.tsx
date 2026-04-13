interface ErrorAlertProps {
  message: string | null | undefined;
  className?: string;
}

/**
 * Accessible inline error alert. Renders nothing if message is falsy.
 */
export function ErrorAlert({ message, className = '' }: ErrorAlertProps) {
  if (!message) return null;
  return (
    <div
      role="alert"
      aria-live="polite"
      className={`text-sm text-[var(--danger)] ${className}`}
    >
      {message}
    </div>
  );
}
