import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import Header from './Header';
import { AuthProvider } from '../contexts/AuthContext';
import cardStyles from '../ui/CardShell.module.css';

const mockFetch = vi.fn();
(globalThis as unknown as { fetch: typeof fetch }).fetch = mockFetch;

function renderHeader() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <MemoryRouter initialEntries={['/']}>
      <QueryClientProvider client={queryClient}>
        <AuthProvider>
          <Header />
        </AuthProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

describe('Header — Track I mobile chrome', () => {
  beforeEach(() => {
    mockFetch.mockReset();
    mockFetch.mockResolvedValue({
      ok: false,
      status: 401,
      json: () => Promise.resolve({}),
    });
  });

  it('shows the current page name as the hamburger micro-label', async () => {
    renderHeader();
    const button = await screen.findByRole('button', { name: 'Open menu' });
    expect(button.textContent).toMatch(/Dashboard/);
    expect(button.className).toMatch(/md:hidden/);
  });

  it('opens an elevated-variant CardShell drawer when the hamburger is tapped', async () => {
    const user = userEvent.setup();
    const { container } = renderHeader();
    await user.click(await screen.findByRole('button', { name: 'Open menu' }));

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Close menu' })).toBeInTheDocument();
    });

    const drawer = container.querySelector('header > .md\\:hidden') as HTMLElement | null;
    expect(drawer).not.toBeNull();
    expect(drawer).toHaveClass(cardStyles['v-elevated']);
    expect(drawer!.querySelector('nav[aria-label="Main navigation"]')).not.toBeNull();
  });
});
