import { useId, useState, type ReactNode } from 'react';
import type { ShowEvaluation, ShowEvidence as Evidence } from '../../../types/showprep';
import { useShowEvidence } from '../../queries/useShowPrepQueries';
import { formatCents } from '../../utils/formatters';
import { availabilityLabels, evidenceLabel, listingTypeLabel, safeSourceURL, showError, showTime, supportIndicator } from './showPrepLabels';
import './show-preparation.css';

export function ShowSupport({ evaluation: e }: { evaluation: ShowEvaluation }) {
  const indicator = supportIndicator(e);
  const lifecycle = evidenceLabel(e);
  return <div className="show-support tabular-nums">
    <strong className={`show-tone-${indicator.tone}`}>{indicator.label}</strong>
    {lifecycle !== indicator.label && <span>{lifecycle}</span>}
    <span>SlabLedger asking <b>{e.localPriceCents > 0 ? formatCents(e.localPriceCents) : 'Missing'}</b></span>
    {!e.evidenceNeedsReview && <span>Recent median <b>{e.recent.count > 0 ? formatCents(e.recent.medianCents) : 'No matching sales'}</b> · {e.recent.count} sales</span>}
    {!e.evidenceNeedsReview && e.recent.latestSaleDate && <span>Newest sale date {e.recent.latestSaleDate}</span>}
    <span>Stored DH listed <b>{e.listedPriceCents > 0 ? formatCents(e.listedPriceCents) : 'Missing'}</b>{e.priceAssociationUnclear && ' (unverified)'}</span>
    <span>{availabilityLabels[e.availability] ?? availabilityLabels.unknown}</span>
    {e.priceAssociationUnclear && <span className="text-[var(--warning)]">DH price association unclear; asking assessment is independent</span>}
    {e.priceMismatch && <span className="text-[var(--warning)]">Stored DH price differs from SlabLedger asking</span>}
  </div>;
}

export function EvidenceDetails({ data }: { data: Evidence }) {
  const e = data.evaluation;
  return <div className="show-evidence">
    {e.reason && <p className={e.status === 'supported' ? 'text-[var(--text-muted)]' : 'text-[var(--warning)]'}>{e.reason}</p>}
    {e.evidenceNeedsReview && e.evidenceReason !== e.reason && <p className="text-[var(--warning)]">{e.evidenceReason}</p>}
    <dl className="show-evidence-meta">
      <div><dt>Matching identity</dt><dd>{e.cardName} · {e.grader} {e.grade} · Cert {e.certNumber}</dd></div>
      <div><dt>Recent assessment window (UTC)</dt><dd>{e.recent.windowStart} to {e.recent.windowEnd} (inclusive)</dd></div>
      <div><dt>30-day context median</dt><dd>{e.compCount > 0 ? formatCents(e.medianCents) : 'No matching sales'} · {e.compCount} sales</dd></div>
      <div><dt>30-day window (UTC)</dt><dd>{e.windowStart || 'Unknown'} to {e.windowEnd || 'Unknown'} (inclusive)</dd></div>
      <div><dt>CardLadder refreshed</dt><dd>{showTime(e.refreshedAt)}</dd></div>
      <div><dt>DH last synced</dt><dd>{showTime(e.listingSyncedAt)}</dd></div>
    </dl>
    <p className="text-[var(--text-muted)]">Support uses the newest five matching sales in the past seven UTC dates, including every sale tied on the fifth sale’s date. At least two sales, a median ≥90% of SlabLedger asking, and no newest-day sale more than 20% below asking are required. The wider 30-day history is context only. Complete evidence must cover the current 30-day window and be no older than 24 hours.</p>
    {e.evidenceNeedsReview && <p className="text-[var(--warning)]">Stored sales may be partial or stale. They are not a verified current window.</p>}
    {data.sales.length > 0 ? <ul className="show-sales" aria-label="Individual matching sales">
      {data.sales.map(sale => {
        const url = safeSourceURL(sale.url);
        return <li key={sale.id} className="tabular-nums">
          <span>{sale.date}</span><strong>{formatCents(sale.priceCents)}</strong>
          <span>{url ? <a href={url} target="_blank" rel="noopener noreferrer">{sale.platform || 'Source'} sale ↗</a> : (sale.platform || 'Source link unavailable')}</span>
          <span className="text-[var(--text-muted)]">{listingTypeLabel(sale.listingType)}</span>
        </li>;
      })}
    </ul> : <p>{!e.evidenceNeedsReview ? 'Complete current lookup: no matching sales in this window.' : 'No detailed sales available. This does not establish no recent comps.'}</p>}
    <p className="text-[var(--text-muted)]">CardLadder source-reported USD sold amounts, not asking prices or independently verified settlement. All returned eligible sales are shown.</p>
  </div>;
}

export function ShowEvidenceButton({ purchaseId, certNumber, evaluation, loading, expanded, onClick, showListedPrice = false }: {
  purchaseId: string; certNumber: string; evaluation?: ShowEvaluation; loading?: boolean; expanded: boolean; onClick: () => void; showListedPrice?: boolean;
}) {
  const indicator = evaluation ? supportIndicator(evaluation) : { label: loading ? 'Loading price support…' : 'Evaluation unavailable', tone: 'muted' };
  const askingPrice = !evaluation ? 'Unavailable' : evaluation.localPriceCents > 0 ? formatCents(evaluation.localPriceCents) : 'Missing';
  return <span className="show-price-support">
    {showListedPrice && <span className="show-listed-price">SlabLedger asking {askingPrice}</span>}
    <button type="button" className={`show-evidence-trigger show-tone-${indicator.tone}`}
    aria-expanded={expanded} aria-controls={`show-evidence-${purchaseId}`}
    aria-label={`${expanded ? 'Hide' : 'Show'} 30-day evidence ${certNumber}: ${indicator.label}`}
    title={`${indicator.label}: recent price support with 30-day context`} onClick={event => { event.stopPropagation(); onClick(); }}>
    <strong>{indicator.label}</strong><span aria-hidden="true">{expanded ? '▴' : '▾'}</span>
  </button></span>;
}

export default function ShowEvidenceDisclosure({ purchaseId, certNumber, evaluation, loading, error, actions, expanded, onExpandedChange, detailsOnly = false }: {
  purchaseId: string; certNumber: string; evaluation?: ShowEvaluation; loading?: boolean; error?: string; actions?: ReactNode;
  expanded?: boolean; onExpandedChange?: (expanded: boolean) => void; detailsOnly?: boolean;
}) {
  const [localOpen, setLocalOpen] = useState(false);
  const open = expanded ?? localOpen;
  const setOpen = onExpandedChange ?? setLocalOpen;
  const localRegionId = useId();
  const regionId = detailsOnly ? `show-evidence-${purchaseId}` : localRegionId;
  const query = useShowEvidence(purchaseId, open, evaluation?.version);
  const detail = open && query.data && !query.isFetching && !query.isError ? query.data.evaluation : undefined;
  // Readiness can change without changing the business fingerprint. Use the
  // latest aggregate observation for that version, retaining the detailed sales.
  const e = detail && detail.version !== evaluation?.version ? detail : evaluation;
  return <div className="show-disclosure">
    {e ? <ShowSupport evaluation={e} /> : <p className="text-[var(--text-muted)]">{loading ? 'Loading price support…' : `Evaluation unavailable: ${error || 'missing result'}`}</p>}
    {!detailsOnly && <div className="show-actions"><button type="button" className="show-link" aria-expanded={open} aria-controls={regionId}
      aria-label={`${open ? 'Hide' : 'Show'} 30-day evidence ${certNumber}`} onClick={() => setOpen(!open)}>
      {open ? 'Hide' : 'Show'} 30-day evidence
    </button>{actions}</div>}
    {open && <section id={regionId} aria-label={`30-day evidence ${certNumber}`}>
      {query.isFetching && <p role="status">Loading sale evidence…</p>}
      {query.isError && <div role="alert">Evidence unavailable: {showError(query.error)} <button className="show-link" onClick={() => query.refetch()} disabled={query.isFetching}>Retry evidence</button></div>}
      {query.data && <EvidenceDetails data={query.data} />}
    </section>}
  </div>;
}
