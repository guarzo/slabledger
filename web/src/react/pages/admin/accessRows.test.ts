import { describe, it, expect } from 'vitest';
import type { AdminUser, AllowedEmail } from '../../../types/admin';
import { mergeAccessRows, type AccessRow } from './accessRows';

function makeUser(overrides: Partial<AdminUser> = {}): AdminUser {
  return {
    id: 1,
    username: 'Alice',
    email: 'alice@example.com',
    avatar_url: 'https://cdn.example.com/alice.png',
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

describe('mergeAccessRows', () => {
  const cases: Array<{
    name: string;
    users: AdminUser[] | undefined;
    allowlist: AllowedEmail[] | undefined;
    want: Array<Partial<AccessRow>>;
  }> = [
    {
      name: 'user with a matching allowlist row collapses to one active row',
      users: [makeUser()],
      allowlist: [makeAllowed({ Notes: 'founder' })],
      want: [
        {
          key: 'email:alice@example.com',
          email: 'alice@example.com',
          status: 'active',
          username: 'Alice',
          isAdmin: false,
          lastLoginAt: '2026-07-01T10:00:00Z',
          notes: 'founder',
          invitedAt: '2026-06-01T09:00:00Z',
          allowlistEmail: 'alice@example.com',
        },
      ],
    },
    {
      name: 'case mismatch between users.email and the allowlist yields one row',
      users: [makeUser({ id: 2, username: 'Foo', email: 'Foo@Bar.com' })],
      allowlist: [makeAllowed({ Email: 'foo@bar.com', Notes: 'invited by ops' })],
      want: [
        {
          key: 'email:foo@bar.com',
          email: 'Foo@Bar.com',
          status: 'active',
          username: 'Foo',
          notes: 'invited by ops',
          allowlistEmail: 'foo@bar.com',
        },
      ],
    },
    {
      name: 'user with no allowlist row is active with nothing to revoke',
      users: [makeUser({ id: 3, username: 'Svc', email: 'svc@example.com', is_admin: true })],
      allowlist: [],
      want: [
        {
          key: 'email:svc@example.com',
          email: 'svc@example.com',
          status: 'active',
          username: 'Svc',
          isAdmin: true,
          notes: '',
          invitedAt: null,
          allowlistEmail: null,
        },
      ],
    },
    {
      name: 'allowlist row with no user is invited',
      users: [],
      allowlist: [makeAllowed({ Email: 'newbie@example.com', Notes: 'starts Monday' })],
      want: [
        {
          key: 'email:newbie@example.com',
          email: 'newbie@example.com',
          status: 'invited',
          username: null,
          avatarUrl: null,
          isAdmin: false,
          lastLoginAt: null,
          notes: 'starts Monday',
          invitedAt: '2026-06-01T09:00:00Z',
          allowlistEmail: 'newbie@example.com',
        },
      ],
    },
    {
      name: 'both inputs undefined yields an empty list',
      users: undefined,
      allowlist: undefined,
      want: [],
    },
    {
      name: 'user with a whitespace email still renders under an id-based key',
      users: [makeUser({ id: 7, username: 'Ghost', email: '   ' })],
      allowlist: [],
      want: [
        {
          key: 'user:7',
          email: '   ',
          status: 'active',
          username: 'Ghost',
          allowlistEmail: null,
        },
      ],
    },
  ];

  it.each(cases)('$name', ({ users, allowlist, want }) => {
    const got = mergeAccessRows(users, allowlist);
    expect(got).toHaveLength(want.length);
    want.forEach((expected, i) => {
      expect(got[i]).toMatchObject(expected);
    });
  });

  it('sorts active rows before invited rows, alphabetically by email within each group', () => {
    const rows = mergeAccessRows(
      [
        makeUser({ id: 1, username: 'Zoe', email: 'zoe@example.com' }),
        makeUser({ id: 2, username: 'Amy', email: 'Amy@Example.com' }),
      ],
      [
        makeAllowed({ Email: 'bob@example.com' }),
        makeAllowed({ Email: 'amy@example.com' }),
        makeAllowed({ Email: 'ann@example.com' }),
      ],
    );

    expect(rows.map(r => r.email)).toEqual([
      'Amy@Example.com',
      'zoe@example.com',
      'ann@example.com',
      'bob@example.com',
    ]);
    expect(rows.map(r => r.status)).toEqual(['active', 'active', 'invited', 'invited']);
  });

  it('drops allowlist rows whose email is blank because they cannot be revoked', () => {
    const rows = mergeAccessRows([], [makeAllowed({ Email: '  ' })]);
    expect(rows).toEqual([]);
  });
});
