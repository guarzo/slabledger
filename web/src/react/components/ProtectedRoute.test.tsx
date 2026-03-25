import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom';
import { ProtectedRoute } from './ProtectedRoute';
import { AuthProvider } from '../contexts/AuthContext';

// Mock fetch
const mockFetch = vi.fn();
(globalThis as unknown as { fetch: typeof fetch }).fetch = mockFetch;

// Mock PokeballLoader
vi.mock('../PokeballLoader', () => ({
  default: () => <div data-testid="pokeball-loader">Loading...</div>
}));

// Helper to render with router
function renderWithRouter(
  ui: React.ReactNode,
  { route = '/' } = {}
) {
  return render(
    <MemoryRouter initialEntries={[route]}>
      <AuthProvider>
        {ui}
      </AuthProvider>
    </MemoryRouter>
  );
}

describe('ProtectedRoute', () => {
  beforeEach(() => {
    mockFetch.mockReset();
  });

  describe('Loading State', () => {
    it('shows loading indicator while checking auth', async () => {
      // Never resolving promise to keep loading state
      mockFetch.mockImplementation(() => new Promise(() => {}));

      renderWithRouter(
        <Routes>
          <Route
            path="/"
            element={
              <ProtectedRoute>
                <div>Protected Content</div>
              </ProtectedRoute>
            }
          />
        </Routes>
      );

      expect(screen.getByTestId('pokeball-loader')).toBeInTheDocument();
      expect(screen.getByText('Checking authentication...')).toBeInTheDocument();
    });
  });

  describe('Authenticated User', () => {
    it('renders children when user is authenticated', async () => {
      const mockUser = {
        id: 1,

        username: 'Test User',
        email: 'test@gmail.com',
        avatar_url: null,
        last_login_at: null
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockUser)
      });

      renderWithRouter(
        <Routes>
          <Route
            path="/"
            element={
              <ProtectedRoute>
                <div>Protected Content</div>
              </ProtectedRoute>
            }
          />
        </Routes>
      );

      await waitFor(() => {
        expect(screen.getByText('Protected Content')).toBeInTheDocument();
      });
    });
  });

  describe('Unauthenticated User', () => {
    it('redirects to login when user is not authenticated', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 401
      });

      renderWithRouter(
        <Routes>
          <Route
            path="/"
            element={
              <ProtectedRoute>
                <div>Protected Content</div>
              </ProtectedRoute>
            }
          />
          <Route path="/login" element={<div>Login Page</div>} />
        </Routes>
      );

      await waitFor(() => {
        expect(screen.getByText('Login Page')).toBeInTheDocument();
      });

      expect(screen.queryByText('Protected Content')).not.toBeInTheDocument();
    });

    it('preserves location state for redirect after login', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 401
      });

      // Test component that displays location state
      function LoginPageWithState() {
        const location = useLocation();
        return (
          <div>
            <div>Login Page</div>
            <div data-testid="from-path">{(location.state as { from?: { pathname: string } })?.from?.pathname || 'no path'}</div>
          </div>
        );
      }

      render(
        <MemoryRouter initialEntries={['/protected-route']}>
          <AuthProvider>
            <Routes>
              <Route
                path="/protected-route"
                element={
                  <ProtectedRoute>
                    <div>Protected Content</div>
                  </ProtectedRoute>
                }
              />
              <Route path="/login" element={<LoginPageWithState />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      );

      await waitFor(() => {
        expect(screen.getByText('Login Page')).toBeInTheDocument();
      });

      expect(screen.getByTestId('from-path')).toHaveTextContent('/protected-route');
    });
  });

  describe('Multiple Protected Routes', () => {
    it('allows navigation between protected routes when authenticated', async () => {
      const mockUser = {
        id: 1,

        username: 'Test User',
        email: 'test@gmail.com',
        avatar_url: null,
        last_login_at: null
      };

      mockFetch.mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(mockUser)
      });

      renderWithRouter(
        <Routes>
          <Route
            path="/"
            element={
              <ProtectedRoute>
                <div>Page One</div>
              </ProtectedRoute>
            }
          />
          <Route
            path="/page-two"
            element={
              <ProtectedRoute>
                <div>Page Two</div>
              </ProtectedRoute>
            }
          />
        </Routes>
      );

      await waitFor(() => {
        expect(screen.getByText('Page One')).toBeInTheDocument();
      });
    });
  });
});
