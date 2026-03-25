import type { AdminUser } from '../../../types/admin';
import { useAdminUsers } from '../../queries/useAdminQueries';

function formatLastLogin(value: string | null | undefined): string {
  if (!value) return 'Never';
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? 'Never' : d.toLocaleDateString();
}

function UserRow({ u }: { u: AdminUser }) {
  return (
    <tr key={u.id} className="glass-table-row">
      <td className="glass-table-td">
        <div className="flex items-center gap-2">
          {u.avatar_url ? (
            <img src={u.avatar_url} alt="" className="w-6 h-6 rounded-full" />
          ) : (
            <div className="w-6 h-6 rounded-full bg-[var(--brand-500)] flex items-center justify-center text-white text-xs font-bold">
              {u.username.charAt(0).toUpperCase()}
            </div>
          )}
          <span className="text-[var(--text)]">{u.username}</span>
        </div>
      </td>
      <td className="glass-table-td text-[var(--text-muted)] hidden sm:table-cell">{u.email}</td>
      <td className="glass-table-td">
        {u.is_admin ? (
          <span className="px-2 py-0.5 text-xs font-medium rounded-full bg-[var(--brand-500)]/20 text-[var(--brand-400)]">Admin</span>
        ) : (
          <span className="px-2 py-0.5 text-xs font-medium rounded-full bg-[var(--surface-2)] text-[var(--text-muted)]">User</span>
        )}
      </td>
      <td className="glass-table-td text-[var(--text-muted)] hidden md:table-cell">
        {formatLastLogin(u.last_login_at)}
      </td>
    </tr>
  );
}

export function UsersTab() {
  const { data: users, error, isLoading } = useAdminUsers();

  if (isLoading) return <div className="text-center text-[var(--text-muted)] py-8" role="status" aria-live="polite" aria-atomic="true">Loading...</div>;
  if (error) return <div className="p-3 rounded-lg bg-[var(--danger-bg)] border border-[var(--danger-border)] text-[var(--danger)] text-sm" role="alert" aria-live="assertive" aria-atomic="true">Failed to load users</div>;

  return (
    <div className="glass-table">
      {!users || users.length === 0 ? (
        <div className="px-5 py-8 text-center text-[var(--text-muted)] text-sm">No registered users.</div>
      ) : (
        <table className="w-full text-sm">
          <thead>
            <tr className="glass-table-header">
              <th className="glass-table-th text-left">User</th>
              <th className="glass-table-th text-left hidden sm:table-cell">Email</th>
              <th className="glass-table-th text-left">Role</th>
              <th className="glass-table-th text-left hidden md:table-cell">Last Login</th>
            </tr>
          </thead>
          <tbody>
            {users.map((u: AdminUser) => (
              <UserRow key={u.id} u={u} />
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
