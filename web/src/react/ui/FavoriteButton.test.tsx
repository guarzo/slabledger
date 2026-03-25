import { render, screen, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { FavoriteButton } from './FavoriteButton';

// Mock contexts
const mockUser = { id: 1, username: 'testuser' };
let currentUser: typeof mockUser | null = mockUser;
let mockFavoriteSet = new Set<string>();
const mockToggleFavorite = vi.fn().mockResolvedValue(true);

vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({
    user: currentUser,
    loading: false,
  }),
}));

vi.mock('../contexts/FavoritesContext', () => ({
  useFavorites: () => ({
    isFavorite: (cardName: string, setName: string, cardNumber: string) => {
      return mockFavoriteSet.has(`${cardName}|${setName}|${cardNumber}`);
    },
    toggleFavorite: mockToggleFavorite,
  }),
}));

describe('FavoriteButton', () => {
  beforeEach(() => {
    currentUser = mockUser;
    mockFavoriteSet = new Set();
    vi.clearAllMocks();
  });

  it('renders unfavorited state correctly', () => {
    render(
      <FavoriteButton
        cardName="Charizard"
        setName="Base Set"
        cardNumber="4"
      />
    );

    const button = screen.getByRole('button');
    expect(button).toHaveAttribute('aria-pressed', 'false');
    expect(button).toHaveAttribute('aria-label', 'Add to favorites');
  });

  it('renders favorited state correctly', () => {
    mockFavoriteSet.add('Charizard|Base Set|4');

    render(
      <FavoriteButton
        cardName="Charizard"
        setName="Base Set"
        cardNumber="4"
      />
    );

    const button = screen.getByRole('button');
    expect(button).toHaveAttribute('aria-pressed', 'true');
    expect(button).toHaveAttribute('aria-label', 'Remove from favorites');
  });

  it('shows disabled state when not logged in', () => {
    currentUser = null;

    render(
      <FavoriteButton
        cardName="Charizard"
        setName="Base Set"
        cardNumber="4"
      />
    );

    const button = screen.getByRole('button');
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('title', 'Sign in to save favorites');
  });

  it('prevents event propagation on click', () => {
    const parentClickHandler = vi.fn();
    const parentKeyDownHandler = (e: React.KeyboardEvent) => {
      if (e.key === 'Enter' || e.key === ' ') {
        parentClickHandler();
      }
    };

    render(
      <div
        role="button"
        tabIndex={0}
        onClick={parentClickHandler}
        onKeyDown={parentKeyDownHandler}
      >
        <FavoriteButton
          cardName="Charizard"
          setName="Base Set"
          cardNumber="4"
        />
      </div>
    );

    const button = screen.getByRole('button', { name: /favorites/i });
    fireEvent.click(button);

    expect(parentClickHandler).not.toHaveBeenCalled();
  });

  it('applies size classes correctly', () => {
    const { rerender } = render(
      <FavoriteButton
        cardName="Charizard"
        setName="Base Set"
        cardNumber="4"
        size="sm"
      />
    );

    expect(screen.getByRole('button').className).toContain('p-0.5');

    rerender(
      <FavoriteButton
        cardName="Charizard"
        setName="Base Set"
        cardNumber="4"
        size="lg"
      />
    );

    expect(screen.getByRole('button').className).toContain('p-1.5');
  });

  it('calls toggleFavorite with correct input on click', async () => {
    const user = userEvent.setup();

    render(
      <FavoriteButton
        cardName="Charizard"
        setName="Base Set"
        cardNumber="4"
        imageUrl="https://example.com/image.png"
      />
    );

    const button = screen.getByRole('button');
    await user.click(button);

    expect(mockToggleFavorite).toHaveBeenCalledWith({
      card_name: 'Charizard',
      set_name: 'Base Set',
      card_number: '4',
      image_url: 'https://example.com/image.png',
    });
  });

  it('does not call toggleFavorite when not logged in', () => {
    currentUser = null;

    render(
      <FavoriteButton
        cardName="Charizard"
        setName="Base Set"
        cardNumber="4"
      />
    );

    const button = screen.getByRole('button');
    fireEvent.click(button);

    expect(mockToggleFavorite).not.toHaveBeenCalled();
  });

  it('applies custom className', () => {
    render(
      <FavoriteButton
        cardName="Charizard"
        setName="Base Set"
        cardNumber="4"
        className="custom-class"
      />
    );

    expect(screen.getByRole('button').className).toContain('custom-class');
  });

  it('renders heart icon', () => {
    render(
      <FavoriteButton
        cardName="Charizard"
        setName="Base Set"
        cardNumber="4"
      />
    );

    // Check that SVG heart icon is rendered
    const svg = screen.getByRole('button').querySelector('svg');
    expect(svg).toBeInTheDocument();
    expect(svg).toHaveAttribute('viewBox', '0 0 24 24');
  });
});
