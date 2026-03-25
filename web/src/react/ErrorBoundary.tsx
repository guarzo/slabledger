import { Component, ReactNode, ErrorInfo, CSSProperties } from 'react';

export interface ErrorBoundaryProps {
  children: ReactNode;
  fallback?: ReactNode | ((error: Error | null, reset: () => void) => ReactNode);
  onError?: (error: Error, errorInfo: ErrorInfo) => void;
  onReset?: () => void;
  title?: string;
  message?: string;
  showReset?: boolean;
  showReload?: boolean;
  showHome?: boolean;
}

export interface ErrorBoundaryState {
  hasError: boolean;
  error: Error | null;
  errorInfo: ErrorInfo | null;
}

/**
 * Error Boundary Component
 * Catches JavaScript errors anywhere in the child component tree,
 * logs the errors, and displays a fallback UI
 *
 * @example
 * // Global error boundary (wrap entire app)
 * <ErrorBoundary>
 *   <App />
 * </ErrorBoundary>
 *
 * // Feature-specific error boundary with custom fallback
 * <ErrorBoundary
 *   fallback={<div>Unable to load opportunities. <button onClick={reload}>Retry</button></div>}
 *   onError={(error, errorInfo) => logToService(error, errorInfo)}
 * >
 *   <OpportunityGrid items={items} />
 * </ErrorBoundary>
 */
class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  constructor(props: ErrorBoundaryProps) {
    super(props);
    this.state = {
      hasError: false,
      error: null,
      errorInfo: null,
    };
  }

  static getDerivedStateFromError(_error: Error): Partial<ErrorBoundaryState> {
    // Update state so the next render will show the fallback UI
    return { hasError: true };
  }

  componentDidCatch(err: Error, errorInfo: ErrorInfo): void {
    // Log error to console
    console.error('ErrorBoundary caught an error:', err, errorInfo);

    // Update state with error details
    this.setState({
      error: err,
      errorInfo,
    });

    // Call custom error handler if provided
    if (this.props.onError) {
      this.props.onError(err, errorInfo);
    }
  }

  handleReset = (): void => {
    this.setState({
      hasError: false,
      error: null,
      errorInfo: null,
    });

    // Call custom reset handler if provided
    if (this.props.onReset) {
      this.props.onReset();
    }
  };

  handleReload = (): void => {
    window.location.reload();
  };

  handleGoHome = (): void => {
    window.location.href = '/';
  };

  render(): ReactNode {
    if (this.state.hasError) {
      // Use custom fallback if provided
      if (this.props.fallback) {
        if (typeof this.props.fallback === 'function') {
          return this.props.fallback(this.state.error, this.handleReset);
        }
        return this.props.fallback;
      }

      // Default error UI
      return (
        <div className="error-boundary" style={defaultStyles.container} role="alert" aria-live="assertive">
          <div className="error-boundary__content" style={defaultStyles.content}>
            <div className="error-boundary__icon" style={defaultStyles.icon}>
              ⚠️
            </div>

            <h2 className="error-boundary__title" style={defaultStyles.title}>
              {this.props.title || 'Something went wrong'}
            </h2>

            <p className="error-boundary__message" style={defaultStyles.message}>
              {this.props.message ||
                'An unexpected error occurred. The error has been logged and we\'ll look into it.'}
            </p>

            {/* Error details (only show in development) */}
            {typeof window !== 'undefined' && window.location.hostname === 'localhost' && this.state.error && (
              <details className="error-boundary__details" style={defaultStyles.details}>
                <summary style={defaultStyles.summary}>Error Details</summary>
                <pre style={defaultStyles.pre}>
                  <code>
                    {this.state.error.toString()}
                    {'\n\n'}
                    {this.state.errorInfo?.componentStack}
                  </code>
                </pre>
              </details>
            )}

            <div className="error-boundary__actions" style={defaultStyles.actions} role="group" aria-label="Error recovery actions">
              {this.props.showReset !== false && (
                <button
                  className="error-boundary__button error-boundary__button--primary"
                  style={{ ...defaultStyles.button, ...defaultStyles.buttonPrimary }}
                  onClick={this.handleReset}
                  type="button"
                  aria-label="Try again to reload component"
                >
                  Try Again
                </button>
              )}

              {this.props.showReload !== false && (
                <button
                  className="error-boundary__button error-boundary__button--secondary"
                  style={{ ...defaultStyles.button, ...defaultStyles.buttonSecondary }}
                  onClick={this.handleReload}
                  type="button"
                  aria-label="Reload entire page"
                >
                  Reload Page
                </button>
              )}

              {this.props.showHome !== false && (
                <button
                  className="error-boundary__button error-boundary__button--tertiary"
                  style={{ ...defaultStyles.button, ...defaultStyles.buttonTertiary }}
                  onClick={this.handleGoHome}
                  type="button"
                  aria-label="Navigate to home page"
                >
                  Go Home
                </button>
              )}
            </div>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}

// Default inline styles (can be overridden with CSS)
const defaultStyles: Record<string, CSSProperties> = {
  container: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    minHeight: '400px',
    padding: 'var(--space-8, 32px)',
    backgroundColor: 'var(--color-bg-secondary, #f9fafb)',
    borderRadius: 'var(--radius-lg, 12px)',
  },
  content: {
    maxWidth: '600px',
    textAlign: 'center',
  },
  icon: {
    fontSize: '64px',
    marginBottom: 'var(--space-4, 16px)',
  },
  title: {
    fontSize: 'var(--font-size-2xl, 24px)',
    fontWeight: '600',
    color: 'var(--color-text-primary, #111827)',
    marginBottom: 'var(--space-2, 8px)',
  },
  message: {
    fontSize: 'var(--font-size-base, 16px)',
    color: 'var(--color-text-secondary, #6b7280)',
    marginBottom: 'var(--space-6, 24px)',
    lineHeight: '1.5',
  },
  details: {
    marginTop: 'var(--space-4, 16px)',
    marginBottom: 'var(--space-4, 16px)',
    textAlign: 'left',
    backgroundColor: 'var(--color-bg-primary, #ffffff)',
    borderRadius: 'var(--radius-md, 8px)',
    padding: 'var(--space-4, 16px)',
    border: '1px solid var(--color-border, #e5e7eb)',
  },
  summary: {
    cursor: 'pointer',
    fontWeight: '500',
    marginBottom: 'var(--space-2, 8px)',
  },
  pre: {
    overflow: 'auto',
    fontSize: 'var(--font-size-sm, 14px)',
    fontFamily: 'monospace',
    color: 'var(--color-danger, #dc2626)',
    maxHeight: '300px',
  },
  actions: {
    display: 'flex',
    gap: 'var(--space-3, 12px)',
    justifyContent: 'center',
    flexWrap: 'wrap',
  },
  button: {
    padding: 'var(--space-3, 12px) var(--space-5, 20px)',
    borderRadius: 'var(--radius-md, 8px)',
    fontSize: 'var(--font-size-base, 16px)',
    fontWeight: '500',
    border: 'none',
    cursor: 'pointer',
    transition: 'all 0.2s',
  },
  buttonPrimary: {
    backgroundColor: 'var(--color-primary, #3b82f6)',
    color: 'var(--color-text-inverse, #ffffff)',
  },
  buttonSecondary: {
    backgroundColor: 'var(--color-bg-elevated, #ffffff)',
    color: 'var(--color-text-primary, #111827)',
    border: '1px solid var(--color-border, #e5e7eb)',
  },
  buttonTertiary: {
    backgroundColor: 'transparent',
    color: 'var(--color-text-secondary, #6b7280)',
    textDecoration: 'underline',
  },
};

export default ErrorBoundary;
