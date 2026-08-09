import { useMemo, useState, type FormEvent } from 'react';
import PokeballLoader from '../../PokeballLoader';
import {
  useAdminUsers,
  useAllowlist,
  useAddAllowedEmail,
  useRemoveAllowedEmail,
} from '../../queries/useAdminQueries';
import { Button, StatusPill } from '../../ui';
import { mergeAccessRows, type AccessRow } from './accessRows';

function formatMutationError(err: unknown): string {
  if (err instanceof Error) return err.message;
  if (typeof err === 'string') return err;
  try {
    const s = JSON.stringify(err);
    return s ?? 'unknown error';
  } catch { return 'unknown error'; }
}

function formatLastLogin(value: string | null | undefined): string {
  if (!value) return 'Never';
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? 'Never' : d.toLocaleDateString();
}

function formatInvitedAt(value: string | null | undefined): string {
  if (!value) return '—';
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleDateString();
}

function AccessTableRow({
  row,
  confirming,
  pending,
  onAskConfirm,
  onCancelConfirm,
  onRevoke,
}: {
  row: AccessRow;
  confirming: boolean;
  pending: boolean;
  onAskConfirm: () => void;
  onCancelConfirm: () => void;
  onRevoke: () => void;
}) {
  const displayEmail = row.email.trim() || '—';
  const isActive = row.status === 'active';

  return (
    <tr className="glass-table-row">
      <td className="glass-table-td">
        {row.username ? (
          <div className="flex items-center gap-2">
            {row.avatarUrl ? (
              <img src={row.avatarUrl} alt="" className="w-6 h-6 rounded-full" />
            ) : (
              <div className="w-6 h-6 rounded-full bg-[var(--brand-500)] flex items-center justify-center text-white text-xs font-bold">
                {row.username.charAt(0).toUpperCase()}
              </div>
            )}
            <span className="text-[var(--text)]">{row.username}</span>
          </div>
        ) : (
          <span className="text-[var(--text-muted)] italic">{displayEmail}</span>
        )}
      </td>
      <td className="glass-table-td text-[var(--text-muted)] hidden sm:table-cell">
        {row.username ? displayEmail : '—'}
      </td>
      <td className="glass-table-td">
        {isActive ? (
          <StatusPill tone="success">Active</StatusPill>
        ) : (
          <StatusPill tone="neutral">Invited</StatusPill>
        )}
      </td>
      <td className="glass-table-td">
        {!isActive ? (
          <span className="text-[var(--text-muted)]">—</span>
        ) : row.isAdmin ? (
          <StatusPill tone="brand">Admin</StatusPill>
        ) : (
          <StatusPill tone="neutral">User</StatusPill>
        )}
      </td>
      <td className="glass-table-td text-[var(--text-muted)] hidden md:table-cell">
        {isActive ? formatLastLogin(row.lastLoginAt) : '—'}
      </td>
      {/* Notes and Invited are the two columns the old AllowlistTab showed
          (AllowlistTab.tsx:96-120). The Add form still writes `notes`, so
          dropping the column would make the field write-only: an operator
          could record why someone was invited and then never read it back. */}
      <td className="glass-table-td text-[var(--text-muted)] hidden lg:table-cell">
        {row.notes.trim() || '—'}
      </td>
      <td className="glass-table-td text-[var(--text-muted)] hidden lg:table-cell">
        {formatInvitedAt(row.invitedAt)}
      </td>
      <td className="glass-table-td text-right">
        {/* Copy accuracy, not squeamishness. This button calls
            DELETE /admin/allowlist/{email} and nothing else. It does NOT revoke
            access:
              - Session validation never rechecks the allowlist
                (internal/adapters/httpserver/middleware/auth.go:86+), so an
                existing session keeps working until it expires.
              - ADMIN_EMAILS accounts skip the allowlist check entirely
                (internal/adapters/httpserver/handlers/auth.go:186-200), so for
                them removal does not even block the next sign-in.
            Labelling it "Revoke access" would promise an operator something the
            backend does not do, and this plan is frontend-only. */}
        {row.allowlistEmail === null ? null : confirming ? (
          <div className="flex items-center justify-end gap-2">
            <span className="text-xs text-[var(--text-muted)] hidden sm:inline">
              Remove {row.allowlistEmail}?
            </span>
            <Button
              variant="danger"
              size="sm"
              onClick={onRevoke}
              loading={pending}
              aria-label={`Confirm removing ${row.allowlistEmail} from the allowlist`}
            >
              Confirm
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={onCancelConfirm}
              aria-label={`Cancel removing ${row.allowlistEmail} from the allowlist`}
            >
              Cancel
            </Button>
          </div>
        ) : (
          <Button
            variant="danger"
            size="sm"
            onClick={onAskConfirm}
            aria-label={`Remove ${row.allowlistEmail} from the allowlist`}
          >
            Remove from allowlist
          </Button>
        )}
      </td>
    </tr>
  );
}

export function UsersTab({ enabled = true }: { enabled?: boolean }) {
  const { data: users, error: usersError, isLoading: usersLoading } = useAdminUsers({ enabled });
  const { data: allowlist, error: allowlistError, isLoading: allowlistLoading } = useAllowlist({ enabled });
  const addMutation = useAddAllowedEmail();
  const removeMutation = useRemoveAllowedEmail();

  const [newEmail, setNewEmail] = useState('');
  const [notes, setNotes] = useState('');
  // Only one row may be in the confirming state at a time.
  const [confirmingKey, setConfirmingKey] = useState<string | null>(null);

  const rows = useMemo(() => mergeAccessRows(users, allowlist), [users, allowlist]);

  const isLoading = usersLoading || allowlistLoading;
  const loadError = usersError ?? allowlistError;

  const handleAdd = (e: FormEvent) => {
    e.preventDefault();
    const trimmed = newEmail.trim().toLowerCase();
    if (!trimmed || !trimmed.includes('@')) return;
    addMutation.mutate({ email: trimmed, notes: notes.trim() || undefined }, {
      onSuccess: () => { setNewEmail(''); setNotes(''); },
    });
  };

  const handleRevoke = (row: AccessRow) => {
    if (!row.allowlistEmail) return;
    removeMutation.mutate(row.allowlistEmail, {
      onSettled: () => setConfirmingKey(null),
    });
  };

  return (
    <div className="space-y-6 mt-4">
      <h3 className="text-base font-semibold text-[var(--text)]">Users &amp; Access</h3>

      <form onSubmit={handleAdd} className="flex flex-col sm:flex-row gap-3">
        <input
          type="email"
          value={newEmail}
          onChange={e => setNewEmail(e.target.value)}
          placeholder="email@example.com"
          aria-label="Email address"
          className="flex-1 px-3 py-2 rounded-lg bg-[var(--surface-0)]/60 border border-[var(--surface-2)]/50 text-[var(--text)] text-sm placeholder:text-[var(--text-subtle)] focus:outline-none focus:border-[var(--brand-500)]/50 focus:bg-[var(--surface-0)] transition-colors duration-200"
          required
        />
        <input
          type="text"
          value={notes}
          onChange={e => setNotes(e.target.value)}
          placeholder="Notes (optional)"
          aria-label="Notes (optional)"
          className="sm:w-48 px-3 py-2 rounded-lg bg-[var(--surface-0)]/60 border border-[var(--surface-2)]/50 text-[var(--text)] text-sm placeholder:text-[var(--text-subtle)] focus:outline-none focus:border-[var(--brand-500)]/50 focus:bg-[var(--surface-0)] transition-colors duration-200"
        />
        <Button type="submit" loading={addMutation.isPending}>
          {addMutation.isPending ? 'Adding...' : 'Add Email'}
        </Button>
      </form>

      {loadError && (
        <div
          className="p-3 rounded-lg bg-[var(--danger-bg)] border border-[var(--danger-border)] text-[var(--danger)] text-sm"
          role="alert"
          aria-live="assertive"
          aria-atomic="true"
        >
          Failed to load access list: {formatMutationError(loadError)}
        </div>
      )}

      {addMutation.isError && (
        <div className="p-3 rounded-lg bg-[var(--danger-bg)] border border-[var(--danger-border)] text-[var(--danger)] text-sm" role="alert">
          Failed to add email: {formatMutationError(addMutation.error)}
        </div>
      )}

      {removeMutation.isError && (
        <div className="p-3 rounded-lg bg-[var(--danger-bg)] border border-[var(--danger-border)] text-[var(--danger)] text-sm" role="alert">
          Failed to remove email from the allowlist: {formatMutationError(removeMutation.error)}
        </div>
      )}

      <div className="glass-table">
        {isLoading ? (
          <div className="px-5 py-8" role="status" aria-live="polite" aria-atomic="true">
            <span className="sr-only">Loading users and access…</span>
            <PokeballLoader />
          </div>
        ) : loadError ? null : (
          <table className="w-full text-sm">
            <thead className="sticky top-0 z-10">
              <tr className="glass-table-header" style={{ backdropFilter: 'blur(12px)' }}>
                <th className="glass-table-th text-left">User</th>
                <th className="glass-table-th text-left hidden sm:table-cell">Email</th>
                <th className="glass-table-th text-left">Status</th>
                <th className="glass-table-th text-left">Role</th>
                <th className="glass-table-th text-left hidden md:table-cell">Last Login</th>
                <th className="glass-table-th text-left hidden lg:table-cell">Notes</th>
                <th className="glass-table-th text-left hidden lg:table-cell">Invited On</th>
                <th className="glass-table-th w-56"></th>
              </tr>
            </thead>
            <tbody>
              {rows.length === 0 ? (
                <tr>
                  <td colSpan={8} className="px-5 py-8 text-center text-[var(--text-muted)] text-sm">
                    No users or invites yet. Add an email above to let someone log in.
                  </td>
                </tr>
              ) : (
                rows.map(row => (
                  <AccessTableRow
                    key={row.key}
                    row={row}
                    confirming={confirmingKey === row.key}
                    pending={removeMutation.isPending}
                    onAskConfirm={() => setConfirmingKey(row.key)}
                    onCancelConfirm={() => setConfirmingKey(null)}
                    onRevoke={() => handleRevoke(row)}
                  />
                ))
              )}
            </tbody>
          </table>
        )}
      </div>

      {/* Say what the control actually does. Both limits are real and neither
          is fixable from the frontend. */}
      <p className="text-xs text-[var(--text-muted)]">
        Removing an email blocks the next sign-in. It does not end sessions that
        are already open, and accounts listed in ADMIN_EMAILS can sign in
        whether or not they appear here.
      </p>
    </div>
  );
}
