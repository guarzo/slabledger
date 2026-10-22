import type { CertRow } from './cardIntakeTypes';
import { hasReturnEpisode, returnNeedsDiagnosis, rowCanRetryListing } from './cardIntakeTypes';

export default function CardIntakeReturnNotice({ row }: { row: CertRow }) {
  const state = row.returnState;
  const operation = state?.operation;
  const earlier = state?.precedingAttempt;
  const diagnosis = state ? returnNeedsDiagnosis(state) : false;
  const readError = !!row.returnStatusError;
  const error = row.returnStatusError ?? row.returnError;
  const listingAttempt = earlier?.kind === 'list' && !hasReturnEpisode(row);
  const listingRetrying = earlier?.kind === 'list' && earlier.phase === 'intake_patch_sync'
    && earlier.retryable && earlier.outcome === 'open' && row.listingStatus === 'listing';
  const listingStalled = listingAttempt && !listingRetrying && Date.now() - Date.parse(earlier.startedAt) >= 60_000;
  const warning = listingStalled || (diagnosis && !listingAttempt && !listingRetrying);
  let message: string | undefined;
  if (rowCanRetryListing(row)) {
    message = 'Listing failed. Retry the same approved listing.';
  } else if (readError && !hasReturnEpisode(row)) {
    message = 'DH state read failed. Listing paused; retrying automatically.';
  } else if (listingRetrying) {
    message = 'Retrying the same approved listing…';
  } else if (diagnosis && listingAttempt && earlier.phase === 'intake_patch_sync' && !listingStalled) {
    message = 'DH listing sync in progress. Listing paused until the attempt settles.';
  } else if (diagnosis && listingAttempt) {
    message = `DH mutation unresolved (list: ${earlier.phase}). Investigate before listing.`;
  } else if (diagnosis && earlier) {
    message = `Earlier DH mutation unresolved (${earlier.kind}: ${earlier.phase}). Diagnose before returning or listing.`;
  } else if (diagnosis) {
    message = 'Return identity conflict. Diagnose before retrying.';
  } else if (state?.awaitingListing && row.listingStatus !== 'listed') {
    message = 'Returned; review the price and list explicitly.';
  } else if (operation?.state === 'pending') {
    message = 'Return pending. Retry the same confirmed return.';
  }
  if (!message && !error) return null;
  return (
    <div className="border-t border-[var(--surface-2)] px-4 py-2 text-xs" role={error || warning ? 'alert' : 'status'}>
      {message && <p className={warning ? 'text-[var(--warning)]' : 'text-[var(--text-muted)]'}>{message}</p>}
      {error && <p className="text-[var(--danger)]">{error}</p>}
    </div>
  );
}
