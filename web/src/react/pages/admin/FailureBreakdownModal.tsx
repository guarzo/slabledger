import { useId } from 'react';
import type { IntegrationFailuresReport } from '../../../types/admin';
import { formatAdminDate } from './adminUtils';

interface Props {
  title: string;
  report: IntegrationFailuresReport | null;
  onClose: () => void;
}

// Short descriptions keyed off the backend's reason tags. Keeps the UI
// human-readable without coupling to a full i18n layer — add to this map
// when new reason tags are introduced in the schedulers.
const REASON_DESCRIPTIONS: Record<string, string> = {
  // Market Movers
  no_card_name: 'Purchase has no card name to search',
  no_cert_results: 'Cert-based search returned zero MM results',
  cert_token_mismatch: 'Cert search returned hits but none matched the card name',
  no_name_results: 'Name-based fallback search returned zero MM results',
  name_token_mismatch: 'Name search top result rejected by token match',
  no_30d_sales: 'Card is mapped but has no sales in the last 30 days',
  // Shared — the modal is used for both MM and CL so this description must
  // be provider-neutral.
  api_error: 'External API returned an error',
  unprocessed: 'No value and no error tag — scheduler never tagged the row',
  // Card Ladder
  no_image_match: 'No CL card matched the purchase by image URL or cert',
  no_cert_match: 'Purchase has no cert number to fallback-match',
  no_value: 'CL collection and cards catalog both reported $0',
  catalog_fallback: 'CL collection reported $0 — priced from the CL cards catalog instead',
};

export function FailureBreakdownModal({ title, report, onClose }: Props) {
  const titleId = useId();
  const byReason = report?.byReason ?? {};
  const reasons = Object.entries(byReason).sort((a, b) => b[1] - a[1]);
  const samples = report?.samples ?? [];
  const totalFailures = reasons.reduce((sum, [, count]) => sum + count, 0);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
    >
      <div
        className="bg-[var(--surface-0)] border border-[var(--surface-2)] rounded-xl shadow-xl max-w-3xl w-full max-h-[80vh] overflow-auto"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between p-4 border-b border-[var(--surface-2)] sticky top-0 bg-[var(--surface-0)]">
          <h2 id={titleId} className="text-lg font-semibold">{title}</h2>
          <button
            type="button"
            onClick={onClose}
            className="text-[var(--text-muted)] hover:text-[var(--text)]"
            aria-label="Close"
          >
            ✕
          </button>
        </div>

        <div className="p-4 space-y-4">
          {!report && (
            <p className="text-sm text-[var(--text-muted)]">Loading failure data...</p>
          )}

          {report && totalFailures === 0 && (
            <p className="text-sm text-[var(--text-muted)]">
              No failures recorded. Either nothing has failed since the last run, or the
              refresh scheduler has not yet written failure reasons for this integration.
            </p>
          )}

          {totalFailures > 0 && (
            <>
              <div>
                <h3 className="text-sm font-semibold mb-2">Failures by reason</h3>
                <ul className="space-y-1">
                  {reasons.map(([reason, count]) => (
                    <li
                      key={reason}
                      className="flex items-start justify-between gap-4 rounded bg-[var(--surface-1)] border border-[var(--surface-2)] p-2"
                    >
                      <div>
                        <div className="text-sm font-mono">{reason}</div>
                        <div className="text-xs text-[var(--text-muted)]">
                          {REASON_DESCRIPTIONS[reason] ?? 'Unknown reason tag'}
                        </div>
                      </div>
                      <div className="text-sm font-semibold">{count}</div>
                    </li>
                  ))}
                </ul>
              </div>

              {samples.length > 0 && (
                <div>
                  <h3 className="text-sm font-semibold mb-2">
                    Recent samples ({samples.length})
                  </h3>
                  <div className="overflow-x-auto">
                    <table className="w-full text-xs border-collapse">
                      <thead>
                        <tr className="text-left text-[var(--text-muted)] border-b border-[var(--surface-2)]">
                          <th className="py-1 pr-2">Cert</th>
                          <th className="py-1 pr-2">Card</th>
                          <th className="py-1 pr-2">Reason</th>
                          <th className="py-1">Logged</th>
                        </tr>
                      </thead>
                      <tbody>
                        {samples.map((s, i) => (
                          <tr key={`${s.purchaseId}-${i}`} className="border-b border-[var(--surface-2)]">
                            <td className="py-1 pr-2 font-mono">{s.certNumber || '—'}</td>
                            <td className="py-1 pr-2">{s.cardName || '—'}</td>
                            <td className="py-1 pr-2 font-mono">{s.reason}</td>
                            <td className="py-1 text-[var(--text-muted)]">
                              {s.errorAt ? formatAdminDate(s.errorAt) : '—'}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}
