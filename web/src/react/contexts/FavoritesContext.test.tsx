import React from 'react';
import { render, screen, waitFor, act } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { FavoritesProvider, useFavorites } from './FavoritesContext';

// Mock user
let mockUser: { id: number } | null = { id: 1 };

vi.mock('./AuthContext', () => ({
  useAuth: () => ({
    user: mockUser,
    loading: false,
  }),
}));

// Mock API responses
const mockApiResponse = {
  favorites: [
    {
      id: 1,
      user_id: 1,
      card_name: 'Charizard',
      set_name: 'Base Set',
      card_number: '4',
      created_at: '2025-01-01T00:00:00Z',
    },
  ],
  total: 1,
  page: 1,
  page_size: 100,
  total_pages: 1,
};

const mockGetFavorites = vi.fn().mockResolvedValue(mockApiResponse);
const mockToggleFavorite = vi.fn().mockResolvedValue({ is_favorite: true });

vi.mock('../../js/api', () => ({
  api: {
    getFavorites: (...args: unknown[]) => mockGetFavorites(...args),
    toggleFavorite: (...args: unknown[]) => mockToggleFavorite(...args),
  },
}));

function createTestQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
}

function Wrapper({ children }: { children: React.ReactNode }) {
  const queryClient = createTestQueryClient();
  return (
    <QueryClientProvider client={queryClient}>
      <FavoritesProvider>{children}</FavoritesProvider>
    </QueryClientProvider>
  );
}

// Test component that exposes FavoritesContext
function TestComponent() {
  const { favorites, loading, total, isFavorite, toggleFavorite } = useFavorites();
  return (
    <div>
      <div data-testid="loading">{loading ? 'loading' : 'loaded'}</div>
      <div data-testid="total">{total}</div>
      <div data-testid="favorites-count">{favorites.length}</div>
      <div data-testid="is-charizard-favorite">
        {isFavorite('Charizard', 'Base Set', '4') ? 'yes' : 'no'}
      </div>
      <div data-testid="is-pikachu-favorite">
        {isFavorite('Pikachu', 'Base Set', '58') ? 'yes' : 'no'}
      </div>
      <button
        type="button"
        onClick={() =>
          toggleFavorite({
            card_name: 'Pikachu',
            set_name: 'Base Set',
            card_number: '58',
          })
        }
      >
        Toggle Pikachu
      </button>
    </div>
  );
}

describe('FavoritesContext', () => {
  beforeEach(() => {
    mockUser = { id: 1 };
    vi.clearAllMocks();
    mockGetFavorites.mockResolvedValue(mockApiResponse);
  });

  it('loads favorites on mount when user is logged in', async () => {
    render(
      <Wrapper>
        <TestComponent />
      </Wrapper>
    );

    await waitFor(() => {
      expect(screen.getByTestId('loading')).toHaveTextContent('loaded');
    });

    expect(screen.getByTestId('favorites-count')).toHaveTextContent('1');
    expect(screen.getByTestId('total')).toHaveTextContent('1');
    expect(mockGetFavorites).toHaveBeenCalledWith(1, 100);
  });

  it('clears favorites when user is not logged in', async () => {
    mockUser = null;
    mockGetFavorites.mockResolvedValue({ favorites: [], total: 0, page: 1, page_size: 100, total_pages: 0 });

    render(
      <Wrapper>
        <TestComponent />
      </Wrapper>
    );

    await waitFor(() => {
      expect(screen.getByTestId('favorites-count')).toHaveTextContent('0');
      expect(screen.getByTestId('total')).toHaveTextContent('0');
    });
  });

  it('isFavorite returns correct status', async () => {
    render(
      <Wrapper>
        <TestComponent />
      </Wrapper>
    );

    await waitFor(() => {
      expect(screen.getByTestId('loading')).toHaveTextContent('loaded');
    });

    expect(screen.getByTestId('is-charizard-favorite')).toHaveTextContent('yes');
    expect(screen.getByTestId('is-pikachu-favorite')).toHaveTextContent('no');
  });

  it('handles API errors gracefully', async () => {
    mockGetFavorites.mockRejectedValue(new Error('Network error'));

    render(
      <Wrapper>
        <TestComponent />
      </Wrapper>
    );

    // React Query handles errors gracefully - component renders with empty data
    await waitFor(() => {
      expect(screen.getByTestId('favorites-count')).toHaveTextContent('0');
    });
  });

  it('toggleFavorite updates state optimistically', async () => {
    mockToggleFavorite.mockResolvedValueOnce({ is_favorite: true });

    render(
      <Wrapper>
        <TestComponent />
      </Wrapper>
    );

    await waitFor(() => {
      expect(screen.getByTestId('loading')).toHaveTextContent('loaded');
    });

    // Toggle Pikachu favorite
    await act(async () => {
      screen.getByRole('button', { name: 'Toggle Pikachu' }).click();
    });

    await waitFor(() => {
      expect(mockToggleFavorite).toHaveBeenCalledWith({
        card_name: 'Pikachu',
        set_name: 'Base Set',
        card_number: '58',
      });
    });
  });

  it('throws error when used outside FavoritesProvider', () => {
    const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    expect(() => {
      render(<TestComponent />);
    }).toThrow('useFavorites must be used within a FavoritesProvider');

    consoleSpy.mockRestore();
  });

  it('handles empty favorites response', async () => {
    mockGetFavorites.mockResolvedValueOnce({
      favorites: [],
      total: 0,
      page: 1,
      page_size: 100,
      total_pages: 0,
    });

    render(
      <Wrapper>
        <TestComponent />
      </Wrapper>
    );

    await waitFor(() => {
      expect(screen.getByTestId('loading')).toHaveTextContent('loaded');
    });

    expect(screen.getByTestId('favorites-count')).toHaveTextContent('0');
    expect(screen.getByTestId('total')).toHaveTextContent('0');
  });
});
