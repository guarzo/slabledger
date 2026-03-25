import { useState, type FormEvent } from 'react';
import type { AllowedEmail } from '../../../types/admin';
import { useAllowlist, useAddAllowedEmail, useRemoveAllowedEmail } from '../../queries/useAdminQueries';
import { Button } from '../../ui';

function formatMutationError(err: unknown): string {
  if (err instanceof Error) return err.message;
  if (typeof err === 'string') return err;
  try {
    const s = JSON.stringify(err);
    return s ?? 'unknown error';
  } catch { return 'unknown error'; }
}

export function AllowlistTab() {
  const { data: emails, error, isLoading } = useAllowlist();
  const addMutation = useAddAllowedEmail();
  const removeMutation = useRemoveAllowedEmail();
  const [newEmail, setNewEmail] = useState('');
  const [notes, setNotes] = useState('');

  const handleAdd = (e: FormEvent) => {
    e.preventDefault();
    const trimmed = newEmail.trim().toLowerCase();
    if (!trimmed || !trimmed.includes('@')) return;
    addMutation.mutate({ email: trimmed, notes: notes.trim() || undefined }, {
      onSuccess: () => { setNewEmail(''); setNotes(''); },
    });
  };

  const allowlist = emails;

  return (
    <div className="space-y-6">
      {/* Add email form */}
      <form onSubmit={handleAdd} className="flex flex-col sm:flex-row gap-3">
        <input
          type="email"
          value={newEmail}
          onChange={e => setNewEmail(e.target.value)}
          placeholder="email@example.com"
          aria-label="Email address"
          className="flex-1 px-3 py-2 rounded-lg bg-[var(--surface-0)]/60 border border-[var(--surface-2)]/50 text-[var(--text)] text-sm placeholder:text-[var(--text-subtle)] focus:outline-none focus:border-[var(--brand-500)]/50 focus:bg-[var(--surface-0)] transition-all duration-200"
          required
        />
        <input
          type="text"
          value={notes}
          onChange={e => setNotes(e.target.value)}
          placeholder="Notes (optional)"
          aria-label="Notes (optional)"
          className="sm:w-48 px-3 py-2 rounded-lg bg-[var(--surface-0)]/60 border border-[var(--surface-2)]/50 text-[var(--text)] text-sm placeholder:text-[var(--text-subtle)] focus:outline-none focus:border-[var(--brand-500)]/50 focus:bg-[var(--surface-0)] transition-all duration-200"
        />
        <Button type="submit" loading={addMutation.isPending}>
          {addMutation.isPending ? 'Adding...' : 'Add Email'}
        </Button>
      </form>

      {error && (
        <div className="p-3 rounded-lg bg-[var(--danger-bg)] border border-[var(--danger-border)] text-[var(--danger)] text-sm">
          Failed to load allowlist: {formatMutationError(error)}
        </div>
      )}

      {addMutation.isError && (
        <div className="p-3 rounded-lg bg-[var(--danger-bg)] border border-[var(--danger-border)] text-[var(--danger)] text-sm">
          Failed to add email: {formatMutationError(addMutation.error)}
        </div>
      )}

      {removeMutation.isError && (
        <div className="p-3 rounded-lg bg-[var(--danger-bg)] border border-[var(--danger-border)] text-[var(--danger)] text-sm">
          Failed to remove email: {formatMutationError(removeMutation.error)}
        </div>
      )}

      {/* Email list */}
      <div className="glass-table">
        {isLoading ? (
          <div className="px-5 py-8 text-center text-[var(--text-muted)] text-sm">Loading...</div>
        ) : !error && Array.isArray(allowlist) && allowlist.length === 0 ? (
          <div className="px-5 py-8 text-center text-[var(--text-muted)] text-sm">
            No emails in allowlist. Add one above to allow users to log in.
          </div>
        ) : allowlist && allowlist.length > 0 ? (
          <table className="w-full text-sm">
            <thead>
              <tr className="glass-table-header">
                <th className="glass-table-th text-left">Email</th>
                <th className="glass-table-th text-left hidden sm:table-cell">Notes</th>
                <th className="glass-table-th text-left hidden md:table-cell">Added</th>
                <th className="glass-table-th w-16"></th>
              </tr>
            </thead>
            <tbody>
              {allowlist.map((ae: AllowedEmail) => (
                <tr key={ae.Email} className="glass-table-row">
                  <td className="glass-table-td text-[var(--text)]">{ae.Email}</td>
                  <td className="glass-table-td text-[var(--text-muted)] hidden sm:table-cell">{ae.Notes || '-'}</td>
                  <td className="glass-table-td text-[var(--text-muted)] hidden md:table-cell">
                    {new Date(ae.CreatedAt).toLocaleDateString()}
                  </td>
                  <td className="glass-table-td text-right">
                    <Button
                      variant="danger"
                      size="sm"
                      onClick={() => removeMutation.mutate(ae.Email)}
                      disabled={removeMutation.isPending}
                    >
                      Remove
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : null}
      </div>
    </div>
  );
}
