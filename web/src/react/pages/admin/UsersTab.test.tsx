import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { AdminUser, AllowedEmail } from '../../../types/admin';
import { UsersTab } from './UsersTab';

vi.mock('../../../js/api', () => ({
  api: {
    getAdminUsers: vi.fn(),
    getAdminAllowlist: vi.fn(),
    addAllowedEmail: vi.fn(),
    removeAllowedEmail: vi.fn(),
  },
  isAPIError: (err: unknown): err is { status: number } =>
    typeof err === 'object' && err !== null && 'status' in err,
}));

import { api } from '../../../js/api';

const mockApi = api as unknown as {
  getAdminUsers: ReturnType<typeof vi.fn>;
  getAdminAllowlist: ReturnType<typeof vi.fn>;
  addAllowedEmail: ReturnType<typeof vi.fn>;
  removeAllowedEmail: ReturnType<typeof vi.fn>;
};

function makeUser(overrides: Partial<AdminUser> = {}): AdminUser {
  return {
    id: 1,
    username: 'Alice',
    email: 'alice@example.com',
    avatar_url: '',
    is_admin: false,
    last_login_at: '2026-07-01T10:00:00Z',
    ...overrides,
  };
}

function makeAllowed(overrides: Partial<AllowedEmail> = {}): AllowedEmail {
  return {
    Email: 'alice@example.com',
    AddedBy: 9,
    CreatedAt: '2026-06-01T09:00:00Z',
    Notes: '',
    ...overrides,
  };
}

function renderTab(users: AdminUser[], allowlist: AllowedEmail[]) {
  mockApi.getAdminUsers.mockResolvedValue(users);
  mockApi.getAdminAllowlist.mockResolvedValue(allowlist);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <UsersTab />
    </QueryClientProvider>
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  mockApi.removeAllowedEmail.mockResolvedValue(undefined);
  mockApi.addAllowedEmail.mockResolvedValue(undefined);
});

describe('UsersTab', () => {
  it('renders a registered user and an invited email in one table', async () => {
    renderTab([makeUser()], [makeAllowed(), makeAllowed({ Email: 'newbie@example.com' })]);

    expect(await screen.findByText('Alice')).toBeInTheDocument();
    expect(await screen.findByText('newbie@example.com')).toBeInTheDocument();
    expect(screen.getAllByRole('table')).toHaveLength(1);
    expect(screen.getByText('Active')).toBeInTheDocument();
    expect(screen.getByText('Invited')).toBeInTheDocument();
  });

  it('renders the notes recorded when the email was invited', async () => {
    // The Add form writes `notes`. If the table does not read it back, the
    // field is write-only and the operator can never see why someone was
    // invited. This test is what keeps the column from being dropped again.
    renderTab([], [makeAllowed({ Email: 'newbie@example.com', Notes: 'starts Monday' })]);

    expect(await screen.findByText('starts Monday')).toBeInTheDocument();
  });

  it('does not call the API on the first Remove from allowlist click', async () => {
    renderTab([makeUser()], [makeAllowed()]);

    fireEvent.click(
      await screen.findByRole('button', { name: /remove alice@example\.com from the allowlist/i })
    );

    expect(mockApi.removeAllowedEmail).not.toHaveBeenCalled();
    expect(
      screen.getByRole('button', { name: /confirm removing alice@example\.com from the allowlist/i })
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /cancel removing alice@example\.com from the allowlist/i })
    ).toBeInTheDocument();
  });

  it('calls removeAllowedEmail with the exact allowlist email on confirm', async () => {
    // The user row carries Google's casing; the allowlist row is lowercased.
    // The delete must use the allowlist value, not the display value.
    renderTab([makeUser({ email: 'Alice@Example.com' })], [makeAllowed()]);

    fireEvent.click(
      await screen.findByRole('button', { name: /remove alice@example\.com from the allowlist/i })
    );
    fireEvent.click(
      screen.getByRole('button', { name: /confirm removing alice@example\.com from the allowlist/i })
    );

    await waitFor(() => {
      expect(mockApi.removeAllowedEmail).toHaveBeenCalledWith('alice@example.com');
    });
    expect(mockApi.removeAllowedEmail).toHaveBeenCalledTimes(1);
  });

  it('restores the Remove from allowlist button and calls nothing on cancel', async () => {
    renderTab([makeUser()], [makeAllowed()]);

    fireEvent.click(
      await screen.findByRole('button', { name: /remove alice@example\.com from the allowlist/i })
    );
    fireEvent.click(
      screen.getByRole('button', { name: /cancel removing alice@example\.com from the allowlist/i })
    );

    expect(
      screen.getByRole('button', { name: /remove alice@example\.com from the allowlist/i })
    ).toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: /confirm removing/i })
    ).not.toBeInTheDocument();
    expect(mockApi.removeAllowedEmail).not.toHaveBeenCalled();
  });

  it('shows no remove control for a user with no allowlist entry', async () => {
    renderTab([makeUser({ id: 5, username: 'Svc', email: 'svc@example.com' })], []);

    expect(await screen.findByText('Svc')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /from the allowlist/i })).not.toBeInTheDocument();
  });

  it('normalizes the email before adding it to the allowlist', async () => {
    renderTab([], []);
    await screen.findByRole('table');

    fireEvent.change(screen.getByLabelText('Email address'), {
      target: { value: '  New.Person@Example.COM  ' },
    });
    fireEvent.change(screen.getByLabelText('Notes (optional)'), {
      target: { value: ' starts Monday ' },
    });
    fireEvent.click(screen.getByRole('button', { name: /add email/i }));

    await waitFor(() => {
      expect(mockApi.addAllowedEmail).toHaveBeenCalledWith('new.person@example.com', 'starts Monday');
    });
  });
});
