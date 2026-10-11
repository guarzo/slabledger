import { Button } from '../../ui';
import { formatCents } from '../../utils/formatters';
import type { ShowScanActivity as Activity } from './useShowScanCommands';

const rejections: Record<string, string> = {
  contents_changed: 'Show contents changed after this scan; rescan to confirm.',
  no_match: 'No inventory match. Check cert and rescan.', ambiguous_cert: 'Multiple cert matches. Resolve identity before rescanning.',
  identity_changed: 'Card identity changed. Check inventory before rescanning.', not_received: 'Not received. Record receipt before rescanning.',
  sold: 'Sold. Cannot confirm for this show.', refunded: 'Refunded. Cannot confirm for this show.',
  campaign_closed: 'Campaign closed. Cannot confirm for this show.', removed: 'Purchase removed. Cannot confirm for this show.',
  unknown_availability: 'Availability unknown. Check inventory before rescanning.',
};
function feedback(row: Activity) {
  if (row.state === 'recovery') return 'Recovery required. Replay this original scan or dismiss it.';
  if (row.state === 'uncertain') return `Not confirmed. Retry the same scan; do not create a new intent. ${row.error ?? ''}`;
  if (row.state !== 'settled') return 'Pending';
  const result = row.result!;
  if (result.original.outcome === 'rejected') return `Failed: ${rejections[result.original.code]}`;
  const label = { confirmed: 'Confirmed', already_confirmed: 'Already confirmed', rechecked: 'Rechecked' }[result.original.outcome];
  if (result.current.state !== 'present') return `${label} previously. Current state: ${result.current.state}. Not current confirmation.`;
  return label;
}
export default function ShowScanActivity({ activity, listId, retry, dismiss, sending }: {
  activity: Activity[]; listId: string; retry: (id: string) => void; dismiss: (id: string) => void; sending: boolean;
}) {
  if (!activity.length) return null;
  return <section aria-label="Recent scans" className="mt-4">
    <h3 className="text-sm font-semibold">Recent scans</h3>
    <ul className="show-scan-rows" aria-live="polite" aria-relevant="additions text">{activity.map(row => <li key={row.command.commandId} className="show-scan-row">
      <span className="tabular-nums">{row.command.certNumber}</span>
      <span className="show-scan-name break-words">{row.result?.original.cardName || 'Scanned cert'}{row.listId !== listId && <small className="block">Submitted show: {row.listId}</small>}
        {row.result && row.result.original.outcome !== 'rejected' && <small className="block text-[var(--text-muted)]">Recorded asking: {row.result.original.acknowledgedPriceCents > 0 ? formatCents(row.result.original.acknowledgedPriceCents) : 'Missing asking price'}</small>}
      </span>
      <span className={row.state === 'settled' && row.result?.original.outcome !== 'rejected' && row.result?.current.state === 'present' ? 'text-[var(--success)]' : 'text-[var(--warning)]'}>{feedback(row)}
        {row.result?.current.evaluation?.evidenceNeedsReview && <small className="block">Evidence needs review. Confirmation does not approve a price change.</small>}
      </span>
      <span className="show-actions">{['recovery', 'uncertain'].includes(row.state) && <Button variant="secondary" size="sm" disabled={sending} aria-label={`${row.state === 'recovery' ? 'Recover' : 'Retry'} scan ${row.command.certNumber}`} onClick={() => retry(row.command.commandId)}>{row.state === 'recovery' ? 'Recover scan' : 'Retry scan'}</Button>}
        {row.state !== 'sending' && row.state !== 'settled' && <Button variant="ghost" size="sm" aria-label={`Dismiss scan ${row.command.certNumber}`} onClick={() => dismiss(row.command.commandId)}>Dismiss</Button>}
      </span>
    </li>)}</ul>
  </section>;
}
