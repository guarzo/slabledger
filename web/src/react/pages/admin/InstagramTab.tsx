import { useInstagramStatus, useConnectInstagram, useDisconnectInstagram } from '../../queries/useSocialQueries';
import { useToast } from '../../contexts/ToastContext';
import { CardShell } from '../../ui/CardShell';
import Button from '../../ui/Button';

export function InstagramTab({ enabled = true }: { enabled?: boolean }) {
  const { data: status, isLoading, error } = useInstagramStatus(enabled);
  const connectMutation = useConnectInstagram();
  const disconnectMutation = useDisconnectInstagram();
  const toast = useToast();

  const handleConnect = async () => {
    try {
      const result = await connectMutation.mutateAsync();
      // Redirect to Instagram authorization
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

  if (error && !status) {
    return (
      <CardShell padding="lg">
        <p className="text-red-400 text-sm">Failed to load Instagram status. Please try refreshing the page.</p>
      </CardShell>
    );
  }

  return (
    <div className="space-y-4 mt-4">
      <CardShell padding="lg">
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">Instagram Connection</h3>

        {status?.connected ? (
          <div className="space-y-4">
            <div className="flex items-center gap-3">
              <div className="w-2 h-2 rounded-full bg-emerald-400" />
              <span className="text-sm text-[var(--text)]">
                Connected as <strong>@{status.username}</strong>
              </span>
            </div>

            {status.expiresAt && (
              <p className="text-xs text-[var(--text-muted)]">
                Token expires: {new Date(status.expiresAt).toLocaleDateString('en-US', {
                  month: 'long', day: 'numeric', year: 'numeric'
                })}
                {' '}(auto-refreshes before expiry)
              </p>
            )}

            {status.connectedAt && (
              <p className="text-xs text-[var(--text-muted)]">
                Connected since: {new Date(status.connectedAt).toLocaleDateString('en-US', {
                  month: 'long', day: 'numeric', year: 'numeric'
                })}
              </p>
            )}

            <Button
              variant="danger"
              size="sm"
              onClick={handleDisconnect}
              loading={disconnectMutation.isPending}
            >
              Disconnect Instagram
            </Button>
          </div>
        ) : (
          <div className="space-y-4">
            <div className="flex items-center gap-3">
              <div className="w-2 h-2 rounded-full bg-gray-500" />
              <span className="text-sm text-[var(--text-muted)]">Not connected</span>
            </div>

            <p className="text-sm text-[var(--text-muted)]">
              Connect your Instagram Business account to publish posts directly from the Content page.
              Requires an Instagram Business or Creator account.
            </p>

            <Button
              variant="primary"
              size="sm"
              onClick={handleConnect}
              loading={connectMutation.isPending}
            >
              Connect Instagram
            </Button>
          </div>
        )}
      </CardShell>

      <CardShell padding="lg">
        <h3 className="text-base font-semibold text-[var(--text)] mb-2">Setup Requirements</h3>
        <ul className="text-sm text-[var(--text-muted)] space-y-1 list-disc list-inside">
          <li>Instagram Business or Creator account</li>
          <li>Meta App with <code>instagram_business_basic</code> and <code>instagram_business_content_publish</code> permissions</li>
          <li>Environment variables: <code>INSTAGRAM_APP_ID</code>, <code>INSTAGRAM_APP_SECRET</code></li>
        </ul>
      </CardShell>
    </div>
  );
}
