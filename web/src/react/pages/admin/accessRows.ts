import type { AdminUser, AllowedEmail } from '../../../types/admin';

/** Whether this person has signed in (`active`) or only holds an invite (`invited`). */
export type AccessStatus = 'active' | 'invited';

/** One person in the merged access table: a registered user, an invite, or both. */
export interface AccessRow {
  key: string;
  email: string;
  status: AccessStatus;
  username: string | null;
  avatarUrl: string | null;
  isAdmin: boolean;
  lastLoginAt: string | null;
  notes: string;
  invitedAt: string | null;
  /** The exact allowlist email to pass to removeAllowedEmail. Null when this
      person has no allowlist row, in which case there is nothing to revoke. */
  allowlistEmail: string | null;
}

/**
 * The allowlist stores emails lowercased; users.email is stored verbatim from
 * Google. Both sides must be normalized the same way or one person renders twice.
 */
function normalize(email: string | null | undefined): string {
  return (email ?? '').trim().toLowerCase();
}

/**
 * Unions registered users with allowlist entries into one row per person.
 *
 * This is a union, not a join: a user can exist with no allowlist row (env-var
 * admins bypass the allowlist, the service user was never invited, and removal
 * from the allowlist does not delete the user), and an allowlist row can exist
 * with no user (the ordinary "invited" case).
 *
 * When both sides exist the user side wins for identity (email casing, name,
 * avatar, role) and the allowlist side supplies notes, invite date, and the
 * exact string the delete endpoint expects.
 */
export function mergeAccessRows(
  users: AdminUser[] | undefined,
  allowlist: AllowedEmail[] | undefined,
): AccessRow[] {
  const byEmail = new Map<string, AccessRow>();
  // Users whose email is blank cannot participate in the merge, but they are
  // real accounts with access, so they are still rendered under an id-based key.
  const unkeyed: AccessRow[] = [];

  for (const u of users ?? []) {
    const norm = normalize(u.email);
    const row: AccessRow = {
      key: norm ? `email:${norm}` : `user:${u.id}`,
      email: u.email,
      status: 'active',
      username: u.username,
      avatarUrl: u.avatar_url || null,
      isAdmin: u.is_admin,
      lastLoginAt: u.last_login_at ?? null,
      notes: '',
      invitedAt: null,
      allowlistEmail: null,
    };
    if (norm) {
      byEmail.set(norm, row);
    } else {
      unkeyed.push(row);
    }
  }

  for (const ae of allowlist ?? []) {
    const norm = normalize(ae.Email);
    // The delete endpoint addresses a row by its email, so a blank one is not
    // revocable. Rendering it would offer an action that cannot work.
    if (!norm) continue;

    const existing = byEmail.get(norm);
    if (existing) {
      existing.allowlistEmail = ae.Email;
      existing.notes = ae.Notes ?? '';
      existing.invitedAt = ae.CreatedAt ?? null;
      continue;
    }

    byEmail.set(norm, {
      key: `email:${norm}`,
      email: ae.Email,
      status: 'invited',
      username: null,
      avatarUrl: null,
      isAdmin: false,
      lastLoginAt: null,
      notes: ae.Notes ?? '',
      invitedAt: ae.CreatedAt ?? null,
      allowlistEmail: ae.Email,
    });
  }

  return [...byEmail.values(), ...unkeyed].sort((a, b) => {
    if (a.status !== b.status) return a.status === 'active' ? -1 : 1;
    return normalize(a.email).localeCompare(normalize(b.email));
  });
}
