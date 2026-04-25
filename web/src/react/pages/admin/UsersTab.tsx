import type { AdminUser } from '../../../types/admin';
import PokeballLoader from '../../PokeballLoader';
import { useAdminUsers } from '../../queries/useAdminQueries';
import { AllowlistTab } from './AllowlistTab';
import { StatusPill } from '../../ui';

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
          <StatusPill tone="brand">Admin</StatusPill>
        ) : (
          <StatusPill tone="neutral">User</StatusPill>
        )}
      </td>
      <td className="glass-table-td text-[var(--text-muted)] hidden md:table-cell">
        {formatLastLogin(u.last_login_at)}
      </td>
    </tr>
  );
}

export function UsersTab({ enabled = true }: { enabled?: boolean }) {
  const { data: users, error, isLoading } = useAdminUsers({ enabled });

  return (
    <div className="space-y-8 mt-4">
      <section>
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">Registered Users</h3>
        {isLoading ? (
          <div className="py-8" role="status" aria-live="polite" aria-atomic="true">
            <PokeballLoader />
          </div>
        ) : error ? (
          <div className="p-3 rounded-lg bg-[var(--danger-bg)] border border-[var(--danger-border)] text-[var(--danger)] text-sm" role="alert" aria-live="assertive" aria-atomic="true">Failed to load users</div>
        ) : (
          <div className="glass-table max-h-[min(400px,calc(100vh-450px))] overflow-y-auto scrollbar-dark">
            {!users || users.length === 0 ? (
              <div className="px-5 py-8 text-center text-[var(--text-muted)] text-sm">No registered users.</div>
            ) : (
              <table className="w-full text-sm">
                <thead className="sticky top-0 z-10">
                  <tr className="glass-table-header" style={{ backdropFilter: 'blur(12px)' }}>
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
        )}
      </section>

      <hr className="border-[var(--surface-2)]" />

      <section>
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">Email Allowlist</h3>
        <AllowlistTab enabled={enabled} />
      </section>
    </div>
  );
}
