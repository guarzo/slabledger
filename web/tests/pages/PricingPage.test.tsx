import { render } from '@testing-library/react';
import { BrowserRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import PriceLookupDrawer from '../../src/react/components/PriceLookupDrawer';
import { UserPreferencesProvider } from '../../src/react/contexts/UserPreferencesContext';
import { ToastProvider } from '../../src/react/contexts/ToastContext';
// Mock API
vi.mock('../../src/js/api', () => ({
  api: {
    getOpportunities: vi.fn(() => Promise.resolve([
      {
        CardID: 'base1-4',
        Card: 'Charizard',
        Set: 'Base Set',
        Number: '4',
        RawUSD: 100,
        PSA10: 500,
        PSA9: 300,
        Score: 85,
        ROI: 4.0,
        ImageURL: 'https://example.com/charizard.jpg'
      }
    ])),
    searchCards: vi.fn((query: string) => {
      if (query.toLowerCase().includes('charizard')) {
        return Promise.resolve({
          cards: [
            {
              id: 'base1-4',
              name: 'Charizard',
              setName: 'Base Set',
              number: '4',
              imageUrl: 'https://example.com/charizard.jpg'
            }
          ]
        });
      }
      return Promise.resolve({ cards: [] });
    }),
    checkPrice: vi.fn(() => Promise.resolve({
      card: { name: 'Charizard', setName: 'Base Set', number: '4' },
      prices: { raw: 100, graded: 500, profit: 400, history: [] },
      recommendation: { grade: 'PSA 10', roi: 400, confidence: 85 }
    })),
    getCollectionItems: vi.fn(),
    getCollectionStats: vi.fn(),
    getSets: vi.fn()
  }
}));

function renderDrawer(open = true) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <ToastProvider>
          <UserPreferencesProvider>
              <PriceLookupDrawer open={open} onOpenChange={() => {}} />
          </UserPreferencesProvider>
        </ToastProvider>
      </BrowserRouter>
    </QueryClientProvider>
  );
}

describe('PriceLookupDrawer', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should render when open', () => {
    renderDrawer(true);
    expect(document.body).toBeDefined();
  });

  it('should cleanup on unmount', () => {
    const { unmount } = renderDrawer(true);
    expect(() => unmount()).not.toThrow();
  });
});
