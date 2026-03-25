/**
 * Tests for ErrorBoundary component
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ErrorBoundary from '../../src/react/ErrorBoundary';

// Component that throws an error
function BrokenComponent({ shouldThrow = true }) {
  if (shouldThrow) {
    throw new Error('Test error message');
  }
  return <div>Working component</div>;
}

// Component that works
function WorkingComponent() {
  return <div>This component works</div>;
}

describe('ErrorBoundary', () => {
  let consoleError;

  beforeEach(() => {
    // Suppress console.error for these tests
    consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    consoleError.mockRestore();
    vi.clearAllMocks();
  });

  describe('normal operation', () => {
    it('should render children when there is no error', () => {
      render(
        <ErrorBoundary>
          <WorkingComponent />
        </ErrorBoundary>
      );

      expect(screen.getByText('This component works')).toBeInTheDocument();
    });

    it('should not render fallback UI when there is no error', () => {
      render(
        <ErrorBoundary>
          <WorkingComponent />
        </ErrorBoundary>
      );

      expect(screen.queryByText(/something went wrong/i)).not.toBeInTheDocument();
    });
  });

  describe('error catching', () => {
    it('should catch errors and display default fallback UI', () => {
      render(
        <ErrorBoundary>
          <BrokenComponent />
        </ErrorBoundary>
      );

      expect(screen.getByText(/something went wrong/i)).toBeInTheDocument();
    });

    it('should display error message from default fallback', () => {
      render(
        <ErrorBoundary>
          <BrokenComponent />
        </ErrorBoundary>
      );

      expect(
        screen.getByText(/an unexpected error occurred/i)
      ).toBeInTheDocument();
    });

    it('should display custom title when provided', () => {
      render(
        <ErrorBoundary title="Custom Error Title">
          <BrokenComponent />
        </ErrorBoundary>
      );

      expect(screen.getByText('Custom Error Title')).toBeInTheDocument();
    });

    it('should display custom message when provided', () => {
      render(
        <ErrorBoundary message="Custom error message for testing">
          <BrokenComponent />
        </ErrorBoundary>
      );

      expect(screen.getByText('Custom error message for testing')).toBeInTheDocument();
    });

    it('should not render children when error occurs', () => {
      render(
        <ErrorBoundary>
          <BrokenComponent />
        </ErrorBoundary>
      );

      expect(screen.queryByText('Working component')).not.toBeInTheDocument();
    });
  });

  describe('custom fallback', () => {
    it('should render custom fallback component when provided', () => {
      render(
        <ErrorBoundary fallback={<div>Custom error UI</div>}>
          <BrokenComponent />
        </ErrorBoundary>
      );

      expect(screen.getByText('Custom error UI')).toBeInTheDocument();
      expect(screen.queryByText(/something went wrong/i)).not.toBeInTheDocument();
    });

    it('should render custom fallback function when provided', () => {
      const fallbackFn = (error, reset) => (
        <div>
          <div>Error: {error?.message}</div>
          <button onClick={reset}>Reset</button>
        </div>
      );

      render(
        <ErrorBoundary fallback={fallbackFn}>
          <BrokenComponent />
        </ErrorBoundary>
      );

      expect(screen.getByText('Error: Test error message')).toBeInTheDocument();
      expect(screen.getByText('Reset')).toBeInTheDocument();
    });
  });

  describe('onError callback', () => {
    it('should call onError callback when error occurs', () => {
      const onError = vi.fn();

      render(
        <ErrorBoundary onError={onError}>
          <BrokenComponent />
        </ErrorBoundary>
      );

      expect(onError).toHaveBeenCalledTimes(1);
      expect(onError).toHaveBeenCalledWith(
        expect.any(Error),
        expect.objectContaining({
          componentStack: expect.any(String),
        })
      );
    });

    it('should pass correct error object to onError', () => {
      const onError = vi.fn();

      render(
        <ErrorBoundary onError={onError}>
          <BrokenComponent />
        </ErrorBoundary>
      );

      const [error] = onError.mock.calls[0];
      expect(error.message).toBe('Test error message');
    });
  });

  describe('reset functionality', () => {
    it('should show "Try Again" button by default', () => {
      render(
        <ErrorBoundary>
          <BrokenComponent />
        </ErrorBoundary>
      );

      expect(screen.getByText('Try Again')).toBeInTheDocument();
    });

    it('should hide "Try Again" button when showReset is false', () => {
      render(
        <ErrorBoundary showReset={false}>
          <BrokenComponent />
        </ErrorBoundary>
      );

      expect(screen.queryByText('Try Again')).not.toBeInTheDocument();
    });

    it('should reset error state and re-render children on reset', async () => {
      const user = userEvent.setup();
      let shouldThrow = true;

      function ConditionalComponent() {
        if (shouldThrow) {
          throw new Error('Test error');
        }
        return <div>Component fixed!</div>;
      }

      render(
        <ErrorBoundary>
          <ConditionalComponent />
        </ErrorBoundary>
      );

      expect(screen.getByText(/something went wrong/i)).toBeInTheDocument();

      // Fix the error
      shouldThrow = false;

      // Click reset button
      await user.click(screen.getByText('Try Again'));

      // Should show working component now
      expect(screen.getByText('Component fixed!')).toBeInTheDocument();
      expect(screen.queryByText(/something went wrong/i)).not.toBeInTheDocument();
    });

    it('should call onReset callback when reset is triggered', async () => {
      const user = userEvent.setup();
      const onReset = vi.fn();

      render(
        <ErrorBoundary onReset={onReset}>
          <BrokenComponent />
        </ErrorBoundary>
      );

      await user.click(screen.getByText('Try Again'));

      expect(onReset).toHaveBeenCalledTimes(1);
    });
  });

  describe('reload functionality', () => {
    it('should show "Reload Page" button by default', () => {
      render(
        <ErrorBoundary>
          <BrokenComponent />
        </ErrorBoundary>
      );

      expect(screen.getByText('Reload Page')).toBeInTheDocument();
    });

    it('should hide "Reload Page" button when showReload is false', () => {
      render(
        <ErrorBoundary showReload={false}>
          <BrokenComponent />
        </ErrorBoundary>
      );

      expect(screen.queryByText('Reload Page')).not.toBeInTheDocument();
    });

    it('should reload page when reload button is clicked', async () => {
      const user = userEvent.setup();
      const reloadMock = vi.fn();
      Object.defineProperty(window, 'location', {
        writable: true,
        value: { reload: reloadMock },
      });

      render(
        <ErrorBoundary>
          <BrokenComponent />
        </ErrorBoundary>
      );

      await user.click(screen.getByText('Reload Page'));

      expect(reloadMock).toHaveBeenCalledTimes(1);
    });
  });

  describe('go home functionality', () => {
    it('should show "Go Home" button by default', () => {
      render(
        <ErrorBoundary>
          <BrokenComponent />
        </ErrorBoundary>
      );

      expect(screen.getByText('Go Home')).toBeInTheDocument();
    });

    it('should hide "Go Home" button when showHome is false', () => {
      render(
        <ErrorBoundary showHome={false}>
          <BrokenComponent />
        </ErrorBoundary>
      );

      expect(screen.queryByText('Go Home')).not.toBeInTheDocument();
    });

    it('should navigate to home when go home button is clicked', async () => {
      const user = userEvent.setup();
      const originalHref = window.location.href;
      Object.defineProperty(window, 'location', {
        writable: true,
        value: { href: originalHref },
      });

      render(
        <ErrorBoundary>
          <BrokenComponent />
        </ErrorBoundary>
      );

      await user.click(screen.getByText('Go Home'));

      expect(window.location.href).toBe('/');
    });
  });

  describe('development mode', () => {
    // Note: import.meta.env.DEV is a build-time constant in Vite
    // It cannot be changed at test runtime, so we skip testing this behavior
    it.skip('should show error details in development mode', () => {
      render(
        <ErrorBoundary>
          <BrokenComponent />
        </ErrorBoundary>
      );

      expect(screen.getByText('Error Details')).toBeInTheDocument();
    });

    it.skip('should not show error details in production mode', () => {
      render(
        <ErrorBoundary>
          <BrokenComponent />
        </ErrorBoundary>
      );

      expect(screen.queryByText('Error Details')).not.toBeInTheDocument();
    });
  });

  describe('multiple boundaries', () => {
    it('should isolate errors to the nearest boundary', () => {
      function ParentWithError() {
        return (
          <div>
            <div>Parent component</div>
            <ErrorBoundary fallback={<div>Child error</div>}>
              <BrokenComponent />
            </ErrorBoundary>
          </div>
        );
      }

      render(
        <ErrorBoundary fallback={<div>Parent error</div>}>
          <ParentWithError />
        </ErrorBoundary>
      );

      // Should show child boundary error, not parent
      expect(screen.getByText('Child error')).toBeInTheDocument();
      expect(screen.getByText('Parent component')).toBeInTheDocument();
      expect(screen.queryByText('Parent error')).not.toBeInTheDocument();
    });
  });
});
