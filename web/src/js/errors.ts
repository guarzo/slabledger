/**
 * Global Error Handler
 * Simplified TypeScript version for React application
 */

/**
 * Report an error from application code through the central handler.
 * This is the single funnel for ad-hoc error logging — use it instead of
 * `console.error` / `console.warn` so future Sentry/telemetry wiring only
 * has to hook one place. The eslint `no-console` rule is enforced for all
 * files except this one, ErrorBoundary.tsx, SectionErrorBoundary.tsx, and
 * main.tsx (which need raw console access for the outermost crash paths).
 */
export function reportError(context: string, error: unknown): void {
  const err = error instanceof Error ? error : new Error(String(error));
  handleError(err, context);
}

/**
 * Global error handler
 */
function handleError(error: Error, context = 'An error occurred'): {
  context: string;
  message: string;
  originalError: Error;
} {
  console.error(`[${context}]`, error);

  // Extract meaningful error message
  let message = error.message || 'Unknown error occurred';

  // Handle network errors
  if (error.name === 'TypeError' && message.includes('fetch')) {
    message = 'Network error: Unable to connect to server';
  }

  // Handle API errors
  if (message.includes('API error')) {
    const statusMatch = message.match(/(\d{3})/);
    if (statusMatch) {
      const status = parseInt(statusMatch[1]);
      if (status === 404) {
        message = 'Resource not found';
      } else if (status === 500) {
        message = 'Server error occurred';
      } else if (status === 403) {
        message = 'Access denied';
      } else if (status === 401) {
        message = 'Authentication required';
      }
    }
  }

  return { context, message, originalError: error };
}

/**
 * Setup global error handlers for unhandled errors and promise rejections
 */
export function setupGlobalErrorHandler(): void {
  window.addEventListener('error', (event) => {
    console.error('Global error:', event.error);
    handleError(event.error, 'Unhandled error');
  });

  window.addEventListener('unhandledrejection', (event) => {
    console.error('Unhandled promise rejection:', event.reason);
    const error = event.reason instanceof Error ? event.reason : new Error(String(event.reason));
    handleError(error, 'Unhandled promise rejection');
  });
}
