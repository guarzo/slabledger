import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { AuthProvider, useAuth } from './AuthContext';
import { writePendingScans, readPendingScans } from '../pages/show-preparation/showScanRecovery';
import { listId } from '../pages/show-preparation/fixtures.test-support';

// Mock fetch - the centralized API client uses fetch internally
const mockFetch = vi.fn();
(globalThis as unknown as { fetch: typeof fetch }).fetch = mockFetch;

// Test component that exposes AuthContext
function TestComponent() {
  const { user, loading, logout, refetchUser } = useAuth();
  return (
    <div>
      <div data-testid="loading">{loading ? 'loading' : 'loaded'}</div>
      <div data-testid="user">{user ? JSON.stringify(user) : 'no user'}</div>
      <button onClick={logout}>Logout</button>
      <button onClick={refetchUser}>Refetch</button>
    </div>
  );
}

describe('AuthContext', () => {
  beforeEach(() => {
    mockFetch.mockReset();
    sessionStorage.clear();
    // Mock window.location
    Object.defineProperty(window, 'location', {
      value: { href: '' },
      writable: true
    });
  });

  describe('Initial Load', () => {
    it('fetches current user on mount', async () => {
      const mockUser = {
        id: 1,

        username: 'Test User',
        email: 'test@gmail.com',
        avatar_url: 'https://example.com/avatar.jpg',
        last_login_at: null
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockUser)
      });

      render(
        <AuthProvider>
          <TestComponent />
        </AuthProvider>
      );

      // Initially loading
      expect(screen.getByTestId('loading')).toHaveTextContent('loading');

      // Wait for fetch to complete
      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('loaded');
      });

      expect(screen.getByTestId('user')).toHaveTextContent('test@gmail.com');
      // Verify the API client called the correct endpoint
      expect(mockFetch).toHaveBeenCalledWith(
        '/api/auth/user',
        expect.objectContaining({ credentials: 'include' })
      );
    });

    it('handles unauthenticated state (401)', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 401,
        statusText: 'Unauthorized',
        json: () => Promise.resolve({ error: 'Unauthorized' })
      });

      render(
        <AuthProvider>
          <TestComponent />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('loaded');
      });

      expect(screen.getByTestId('user')).toHaveTextContent('no user');
    });

    it('handles fetch error gracefully', async () => {
      const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
      // Simulate a server error; the API client will throw an APIError for 400 status
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        statusText: 'Bad Request',
        json: () => Promise.resolve({ error: 'Bad Request' })
      });

      render(
        <AuthProvider>
          <TestComponent />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('loaded');
      });

      expect(screen.getByTestId('user')).toHaveTextContent('no user');
      consoleError.mockRestore();
    });
  });

  describe('Logout', () => {
    it('clears user and redirects on logout', async () => {
      const user = userEvent.setup();
      const mockUser = {
        id: 1,

        username: 'Test User',
        email: 'test@gmail.com',
        avatar_url: null,
        last_login_at: null
      };

      // Initial fetch returns user
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockUser)
      });

      render(
        <AuthProvider>
          <TestComponent />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId('user')).toHaveTextContent('test@gmail.com');
      });

      // Mock logout response
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ message: 'Logged out' })
      });

      await user.click(screen.getByRole('button', { name: 'Logout' }));

      await waitFor(() => {
        expect(screen.getByTestId('user')).toHaveTextContent('no user');
      });

      // Verify the API client called the correct endpoint
      expect(mockFetch).toHaveBeenCalledWith(
        '/api/auth/logout',
        expect.objectContaining({ method: 'POST', credentials: 'include' })
      );
      expect(window.location.href).toBe('/login');
    });

    it('handles logout error gracefully', async () => {
      const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
      const user = userEvent.setup();
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

      render(
        <AuthProvider>
          <TestComponent />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId('user')).toHaveTextContent('test@gmail.com');
      });

      // Simulate a non-retryable error so the API client fails immediately
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        statusText: 'Bad Request',
        json: () => Promise.resolve({ error: 'Logout failed' })
      });

      await user.click(screen.getByRole('button', { name: 'Logout' }));

      // Verify redirect still happens despite error
      await waitFor(() => {
        expect(window.location.href).toBe('/login');
      });
      consoleError.mockRestore();
    });
  });

  it.each([200, 400])('clears recovery before every logout attempt, even status %s and same-user login', async status => {
    const operator = { id: 7, username: 'Operator', email: 'operator@example.test', avatar_url: '', is_admin: false, last_login_at: null };
    mockFetch.mockResolvedValueOnce(Response.json(operator));
    render(<AuthProvider><TestComponent /></AuthProvider>);
    await waitFor(() => expect(screen.getByTestId('loading')).toHaveTextContent('loaded'));
    writePendingScans('7', [{ listId, command: { commandId: listId, certNumber: '00123AB', withdrawalGeneration: 0 } }]);
    mockFetch.mockImplementationOnce(async () => {
      expect(readPendingScans('7')).toEqual([]);
      return Response.json(status === 200 ? {} : { error: 'Logout failed' }, { status });
    });
    await userEvent.click(screen.getByRole('button', { name: 'Logout' }));
    expect(readPendingScans('7')).toEqual([]);
    mockFetch.mockResolvedValueOnce(Response.json(operator));
    await userEvent.click(screen.getByRole('button', { name: 'Refetch' }));
    expect(readPendingScans('7')).toEqual([]);
  });
  it.each([8, null])('clears recovery on accepted account identity %s', async id => {
    mockFetch.mockResolvedValueOnce(Response.json({ id: 7 }));
    render(<AuthProvider><TestComponent /></AuthProvider>);
    await waitFor(() => expect(screen.getByTestId('loading')).toHaveTextContent('loaded'));
    writePendingScans('7', [{ listId, command: { commandId: listId, certNumber: '00123AB', withdrawalGeneration: 0 } }]);
    mockFetch.mockResolvedValueOnce(id ? Response.json({ id }) : Response.json({ error: 'Unauthorized' }, { status: 401 }));
    await userEvent.click(screen.getByRole('button', { name: 'Refetch' }));
    await waitFor(() => expect(readPendingScans('7')).toEqual([]));
  });

  describe('useAuth hook', () => {
    it('throws error when used outside AuthProvider', () => {
      const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});

      expect(() => {
        render(<TestComponent />);
      }).toThrow('useAuth must be used within an AuthProvider');

      consoleError.mockRestore();
    });
  });

  describe('User data structure', () => {
    it('stores all user fields correctly', async () => {
      const mockUser = {
        id: 42,

        username: 'Pokemon Master',
        email: 'master@pokemon.com',
        avatar_url: 'https://lh3.googleusercontent.com/avatar.jpg',
        last_login_at: '2025-01-01T00:00:00Z'
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockUser)
      });

      render(
        <AuthProvider>
          <TestComponent />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('loaded');
      });

      const userData = screen.getByTestId('user').textContent;
      expect(userData).toContain('Pokemon Master');
      expect(userData).toContain('master@pokemon.com');
      expect(userData).toContain('avatar.jpg');
    });
  });
});
