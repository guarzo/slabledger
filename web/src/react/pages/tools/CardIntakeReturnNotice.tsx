import type { CertRow } from './cardIntakeTypes';
import { returnNeedsDiagnosis } from './cardIntakeTypes';

export default function CardIntakeReturnNotice({ row }: { row: CertRow }) {
  const state = row.returnState;
  const operation = state?.operation;
  const earlier = state?.precedingAttempt;
  const diagnosis = state ? returnNeedsDiagnosis(state) : false;
  const error = row.returnStatusError ?? row.returnError;
  const message = diagnosis && earlier
    ? `Earlier DH mutation unresolved (${earlier.kind}: ${earlier.phase}). Diagnose before returning or listing.`
    : diagnosis ? 'Return identity conflict. Diagnose before retrying.'
      : state?.awaitingListing && row.listingStatus !== 'listed' ? 'Returned; review the price and list explicitly.'
        : operation?.state === 'pending' ? 'Return pending. Retry the same confirmed return.'
          : undefined;
  if (!message && !error) return null;
  return (
    <div className="border-t border-[var(--surface-2)] px-4 py-2 text-xs" role={error || diagnosis ? 'alert' : 'status'}>
      {message && <p className={diagnosis ? 'text-[var(--warning)]' : 'text-[var(--text-muted)]'}>{message}</p>}
      {error && <p className="text-[var(--danger)]">{error}</p>}
    </div>
  );
}
