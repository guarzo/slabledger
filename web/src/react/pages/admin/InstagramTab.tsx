import { useState } from 'react';
import { useInstagramStatus, useConnectInstagram, useDisconnectInstagram } from '../../queries/useSocialQueries';
import { useToast } from '../../contexts/ToastContext';
import { CardShell } from '../../ui/CardShell';
import Button from '../../ui/Button';

export function InstagramTab({ enabled = true }: { enabled?: boolean }) {
  const { data: status, isLoading, error } = useInstagramStatus(enabled);
  const connectMutation = useConnectInstagram();
  const disconnectMutation = useDisconnectInstagram();
  const toast = useToast();
  const [confirmDisconnect, setConfirmDisconnect] = useState(false);

  const handleConnect = async () => {
    try {
      const result = await connectMutation.mutateAsync();
      window.location.href = result.url;
    } catch {
      toast.error('Failed to initiate Instagram connection');
    }
  };

  const handleDisconnect = async () => {
    try {
      await disconnectMutation.mutateAsync();
      toast.success('Instagram disconnected');
    } catch {
      toast.error('Failed to disconnect Instagram');
    }
  };

  if (isLoading) {
    return (
      <CardShell padding="lg">
        <p className="text-[var(--text-muted)]">Loading Instagram status...</p>
      </CardShell>
    );
  }

  const setupRequirements = (
    <details className="mt-3">
      <summary className="text-xs text-[var(--brand-400)] cursor-pointer select-none">Setup requirements</summary>
      <ul className="text-xs text-[var(--text-muted)] space-y-1 list-disc list-inside mt-2">
        <li>Instagram Business or Creator account</li>
        <li>Meta App with <code>instagram_business_basic</code> and <code>instagram_business_content_publish</code> permissions</li>
        <li>Environment variables: <code>INSTAGRAM_APP_ID</code>, <code>INSTAGRAM_APP_SECRET</code></li>
      </ul>
    </details>
  );

  const notConnectedView = (
    <CardShell padding="lg">
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <span className="w-2 h-2 rounded-full bg-gray-500 shrink-0" />
          <span className="text-sm font-semibold text-[var(--text)]">Not connected</span>
        </div>
      </div>

      <p className="text-xs text-[var(--text-muted)] mb-3">
        Connect your Instagram Business account to publish posts directly from the Content page.
      </p>

      <Button
        variant="primary"
        size="sm"
        onClick={handleConnect}
        loading={connectMutation.isPending}
      >
        Connect Instagram
      </Button>

      {setupRequirements}
    </CardShell>
  );

  if (error && !status) {
    return (
      <div className="space-y-4 mt-4">
        <CardShell padding="lg">
          <p className="text-red-400 text-sm mb-2">Failed to load Instagram status.</p>
        </CardShell>
        {notConnectedView}
      </div>
    );
  }

  return (
    <div className="space-y-4 mt-4">
      {status?.connected ? (
        <CardShell padding="lg">
          {/* Header row */}
          <div className="flex items-center justify-between mb-3">
            <div className="flex items-center gap-2">
              <span className="w-2 h-2 rounded-full bg-emerald-400 shrink-0" />
              <span className="text-sm font-semibold text-[var(--text)]">Connected</span>
            </div>
            <span className="text-xs text-[var(--text-muted)]">@{status.username}</span>
          </div>

          {/* Info rows */}
          <div className="space-y-1 mb-3">
            {status.expiresAt && (
              <p className="text-xs text-[var(--text-muted)]">
                Token expires: {new Date(status.expiresAt).toLocaleDateString('en-US', {
                  month: 'long', day: 'numeric', year: 'numeric'
                })} (auto-refreshes)
              </p>
            )}
            {status.connectedAt && (
              <p className="text-xs text-[var(--text-muted)]">
                Connected since: {new Date(status.connectedAt).toLocaleDateString('en-US', {
                  month: 'long', day: 'numeric', year: 'numeric'
                })}
              </p>
            )}
          </div>

          {/* Disconnect — collapsible danger zone */}
          <details>
            <summary className="text-xs text-red-400 cursor-pointer select-none">Disconnect</summary>
            <div className="mt-3">
              {!confirmDisconnect ? (
                <Button variant="danger" size="sm" onClick={() => setConfirmDisconnect(true)}>
                  Disconnect Instagram
                </Button>
              ) : (
                <div className="rounded-lg bg-[var(--danger-bg)] border border-[var(--danger-border)] p-3 space-y-2">
                  <p className="text-sm text-[var(--danger)]">Disconnect Instagram? This will stop all scheduled posts.</p>
                  <div className="flex gap-2">
                    <Button variant="danger" size="sm" onClick={handleDisconnect} loading={disconnectMutation.isPending}>
                      Yes, disconnect
                    </Button>
                    <Button variant="secondary" size="sm" onClick={() => setConfirmDisconnect(false)}>
                      Cancel
                    </Button>
                  </div>
                </div>
              )}
            </div>
          </details>
        </CardShell>
      ) : (
        notConnectedView
      )}
    </div>
  );
}
