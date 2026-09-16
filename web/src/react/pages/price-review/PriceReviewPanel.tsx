import { useEffect } from 'react';
import { useIsMutating, useMutation, useQueryClient } from '@tanstack/react-query';
import type { AgingItem } from '../../../types/campaigns';
import type { ShowEvaluation, ShowSale } from '../../../types/showprep';
import { isShowEvaluation } from '../../../js/api/showprep';
import { usePricePreview } from '../../queries/usePricePreview';
import { showPrepKeys, useShowEvidence } from '../../queries/useShowPrepQueries';
import { useDebounce } from '../../hooks/useDebounce';
import Button from '../../ui/Button';
import Input from '../../ui/Input';
import GradeBadge from '../../ui/GradeBadge';
import { centsToDollars, formatCents, getErrorMessage } from '../../utils/formatters';
import { costBasis } from '../campaign-detail/inventory/utils';
import { availabilityLabels, evidenceLabel, listingTypeLabel, safeSourceURL, showTime } from '../show-preparation/showPrepLabels';
import { assessmentLabels, gapLabel, parsePriceDraft, priceGroup, type PriceDraft } from './priceReviewModel';
import type { PriceSaveResult } from './usePriceReviewState';

export interface PriceReviewPanelProps {
  purchaseId: string; item?: AgingItem; evaluation?: ShowEvaluation; draft?: PriceDraft;
  onDraftChange: (draft: PriceDraft) => void; onClearDraft: () => void;
  onSavePrice: (id: string, priceCents: number) => Promise<void>;
  onRecheckInventory?: () => Promise<boolean>;
  save?: PriceSaveResult; onSaveResultChange: (result: PriceSaveResult) => void; onSaveRechecked: (rechecked: boolean) => void;
}

export function PriceReviewPanel({ purchaseId, item, evaluation, draft, onDraftChange, onClearDraft, onSavePrice,
  save, onSaveResultChange, onSaveRechecked, onRecheckInventory }: PriceReviewPanelProps) {
  const aggregate = isShowEvaluation(evaluation) ? evaluation : undefined;
  const evidence = useShowEvidence(purchaseId, !!item, aggregate?.version);
  const detail = !evidence.isFetching && !evidence.isError ? evidence.data?.evaluation : undefined;
  // Detail reads may observe newer saved inputs. The shared hook also publishes them
  // to the aggregate; hypothetical previews never participate in this reconciliation.
  const e = detail && detail.version !== aggregate?.version ? detail : aggregate;
  const savedCents = e?.localPriceCents ?? 0;
  const value = draft?.value ?? (savedCents > 0 ? centsToDollars(savedCents) : '');
  const cents = parsePriceDraft(value);
  const debouncedValue = useDebounce(value, 300);
  const editable = !!item && !!e && !item.purchase.wasRefunded && item.purchase.dhStatus !== 'sold'
    && (e.availability === 'ready' || e.availability === 'not_received');
  const changed = cents !== null && cents !== savedCents;
  const currentInput = debouncedValue === value;
  const trial = usePricePreview(purchaseId, editable && currentInput && changed ? cents : null, e);
  const result = currentInput && changed && !trial.isFetching && !trial.isError ? trial.data : undefined;
  const previewBaselineChanged = !!result && result.currentPriceCents !== savedCents;
  const qc = useQueryClient();
  const mutationKey = ['price-review', 'save-price', purchaseId];
  const saving = useIsMutating({ mutationKey }) > 0;
  const mutation = useMutation({ mutationKey, mutationFn: persist, retry: false });
  const needsRecheck = !!save && !save.rechecked && !saving;
  const canSave = editable && changed && !saving && !needsRecheck && !previewBaselineChanged && !evidence.isError;

  useEffect(() => {
    if (save?.state === 'saved' && save.rechecked && savedCents === save.cents && draft?.value === save.value) onClearDraft();
  }, [save, savedCents, draft, onClearDraft]);

  function changeDraft(next: string) {
    onDraftChange({ value: next, baselinePriceCents: draft?.baselinePriceCents ?? savedCents });
  }
  async function recheck() { onSaveRechecked(await recheckSaved()); }
  async function recheckSaved() {
    const inventoryCurrent = onRecheckInventory ? await onRecheckInventory() : true;
    // Inventory may settle after focus moves. Recheck the captured card using the
    // installed evidence query, including its aggregate publication rules.
    const queryKey = [...showPrepKeys.evidence(purchaseId), aggregate?.version ?? ''];
    await qc.refetchQueries({ queryKey, exact: true, type: 'all' });
    return inventoryCurrent && qc.getQueryState(queryKey)?.status === 'success';
  }
  async function persist() {
    if (cents === null || !editable) return;
    const pending: PriceSaveResult = { state: 'saving', cents, value, rechecked: false };
    onSaveResultChange(pending);
    let outcome: PriceSaveResult;
    try {
      await onSavePrice(purchaseId, cents);
      outcome = { ...pending, state: 'saved' };
    } catch (error) {
      outcome = { ...pending, state: 'error', message: getErrorMessage(error, 'Price save failed') };
    }
    // Record the write outcome in the persistent owner before read-back. A failed
    // read must not erase a confirmed write, even if this panel has unmounted.
    onSaveResultChange(outcome);
    // An uncertain result is read back, never automatically replayed. Keep the draft
    // until an authoritative read confirms a successful local save.
    onSaveRechecked(await recheckSaved());
  }
  const recent = e?.recent;
  const recentIDs = new Set(recent?.saleIds);
  const saleRows = evidence.data?.sales ?? [];
  const recentSales = saleRows.filter(sale => recentIDs.has(sale.id));
  const unavailable = !item ? 'Card unavailable in current inventory. Unsaved text is retained; price actions are disabled.'
    : item.purchase.wasRefunded ? 'Unavailable: refunded' : item.purchase.dhStatus === 'sold' ? 'Unavailable: sold'
    : e && e.availability !== 'ready' && e.availability !== 'not_received' ? availabilityLabels[e.availability] : undefined;
  const cost = item ? costBasis(item.purchase) : null;
  const singleLow = recent?.count === 1 && recent.gapPct !== null && recent.gapPct > 0;
  const newestLow = !!recent && savedCents > 0 && recent.latestSaleMinCents < savedCents;

  return <div className="price-review-panel">
    {item && <header>
      {item.campaignName && <p className="price-review-meta">{item.campaignName}</p>}
      <h3 tabIndex={-1} data-price-detail-heading>{item.purchase.cardName}</h3>
      <p className="price-review-identity"><GradeBadge grader={item.purchase.grader} grade={item.purchase.gradeValue} />
        <span>{item.purchase.setName} · Cert {item.purchase.certNumber}</span></p>
    </header>}
    {unavailable && <p role="status" className="price-review-warning">{unavailable}</p>}
    <section aria-label="Saved price assessment" className="price-review-saved" aria-busy={saving || needsRecheck}>
      {saving || needsRecheck ? <p role="status" className="price-review-warning">Assessment pending refresh. Previously loaded facts may be stale.</p>
        : <strong className={`price-review-status price-review-status-${priceGroup(e)}`}>{e ? assessmentLabels[e.status] : 'Evaluation unavailable'}</strong>}
      {e?.reason && <p>{e.reason}</p>}
      {e?.evidenceNeedsReview && <p className="price-review-warning">Stored sales may be partial or stale. They are not a verified current window. {e.evidenceReason}</p>}
      <dl className="price-review-facts">
        <div><dt>SlabLedger asking</dt><dd><output aria-label="Saved asking price">{!e ? 'Unavailable' : savedCents > 0 ? formatCents(savedCents) : 'Not set'}</output></dd></div>
        <div><dt>{recent?.count === 1 ? 'Recent single sale' : 'Recent median'}</dt>
          <dd><span aria-label={recent?.count === 1 ? 'Recent single sale' : 'Recent median'} className={singleLow ? 'price-review-low' : ''}>
            {recent && recent.count > 0 ? formatCents(recent.medianCents) : 'No recent reference'}</span></dd>
          {recent && <dd className="price-review-meta">{recent.count} {recent.count === 1 ? 'sale' : 'sales'} · {recent.windowStart} to {recent.windowEnd} UTC</dd>}
        </div>
      </dl>
      {recent && recent.count > 0 && <p className={singleLow ? 'price-review-low' : 'price-review-meta'}>{gapLabel(recent.gapPct)}</p>}
      {recent?.latestSaleDate && <p className="price-review-newest">Newest sale day · {recent.latestSaleDate} · <strong className={newestLow ? 'price-review-low' : ''}>
        {formatCents(recent.latestSaleMinCents)}{recent.latestSaleMaxCents !== recent.latestSaleMinCents && ` to ${formatCents(recent.latestSaleMaxCents)}`}
      </strong> · {recent.latestSaleCount} {recent.latestSaleCount === 1 ? 'sale' : 'sales'}{newestLow && ' · Below asking'}</p>}
    </section>

    <section className="price-review-editor" aria-label="Price editor">
      <h4>Test an asking price</h4>
      <Input label="Asking price" inputMode="decimal" leftAddon="$" value={value} onChange={event => changeDraft(event.target.value)}
        disabled={!editable || saving} error={draft && cents === null ? 'Enter a positive USD amount with no more than two decimal places.' : undefined} />
      {draft && draft.baselinePriceCents !== savedCents && e && <p role="status" className="price-review-warning">
        Saved asking changed from {formatCents(draft.baselinePriceCents)} to {formatCents(savedCents)}. Draft retained. Review before saving.
      </p>}
      <div className="price-review-actions">
        {recent && recent.count > 0 && <Button variant="secondary" size="sm" disabled={!editable || saving || e?.evidenceNeedsReview}
          onClick={() => changeDraft(centsToDollars(recent.medianCents))}>Try recent reference {formatCents(recent.medianCents)}</Button>}
        {draft && <Button variant="ghost" size="sm" disabled={saving} onClick={onClearDraft}>Reset draft</Button>}
      </div>
      <section aria-label="Trial price assessment" aria-live="polite" className="price-review-trial">
        {!editable ? <p>Trial assessment unavailable for this card.</p> : cents === null ? <p>Enter a valid trial price.</p>
          : !changed ? <p>Enter a different price to test. The saved assessment is shown above.</p>
          : !currentInput ? <p>Waiting for current input…</p>
          : trial.isFetching ? <p>Checking trial price…</p>
          : trial.isError ? <div><p role="alert">Trial assessment unavailable: {getErrorMessage(trial.error)}</p>
            <Button variant="secondary" size="sm" onClick={() => trial.refetch()}>Retry preview</Button></div>
          : result ? <><p className="price-review-meta">Trial only · {formatCents(result.trialPriceCents)}</p>
            <strong className={`price-review-status price-review-status-${result.status === 'supported' ? 'supported' : result.status === 'below_target' ? 'above' : 'mixed'}`}>{assessmentLabels[result.status]}</strong>
            <p>{result.reason}</p>{result.evidenceNeedsReview && <p className="price-review-warning">{result.evidenceReason}</p>}
            <p className="price-review-meta">{gapLabel(result.recent.gapPct)} · Not saved</p></> : <p>Checking trial price…</p>}
      </section>
      {previewBaselineChanged && <p role="status" className="price-review-warning">Preview observed a different saved asking price: {formatCents(result.currentPriceCents)}. Recheck saved state before saving.</p>}
      {(needsRecheck || previewBaselineChanged) && <Button variant="secondary" size="sm" disabled={!item || evidence.isFetching} onClick={recheck}>Recheck saved state</Button>}
      {cost !== null && <p className="price-review-meta">Cost {formatCents(cost)}{cents !== null && ` · Before-fee margin at trial ${formatCents(cents - cost)}`}. Not a sale-profit guarantee.</p>}
      <p className="price-review-consequence">Saving syncs DH and can list eligible inventory. DH processing is asynchronous; a local save does not confirm remote completion.</p>
      <Button disabled={!canSave} loading={saving} onClick={() => {
        if (canSave && qc.isMutating({ mutationKey }) === 0) mutation.mutate();
      }}>{saving ? 'Saving price…' : 'Save price'}</Button>
      {save?.state === 'saved' && needsRecheck && onRecheckInventory && <p role="status" className="price-review-warning">Price saved; assessment could not be refreshed. Retry the read to confirm current inventory and evidence.</p>}
      {save?.state === 'saved' && <p role="status" className="price-review-success">Price saved locally. {save.rechecked ? 'Saved state rechecked.' : 'Recheck saved state to confirm the current amount.'} DH processing may still be pending.</p>}
      {save?.state === 'error' && <p role="alert" className="price-review-warning">Save not confirmed: {save.message}. Draft retained. {save.rechecked ? 'Saved state rechecked; review it before retrying.' : 'Recheck saved state before retrying.'}</p>}
    </section>

    {item && <section className="price-review-evidence">
      <h4>Recent matching sales</h4>
      {evidence.isFetching && <p role="status">Loading stored sale evidence…</p>}
      {evidence.isError && <div role="alert"><p>Evidence unavailable: {getErrorMessage(evidence.error)}</p>
        <Button variant="secondary" size="sm" onClick={() => evidence.refetch()}>Retry evidence</Button></div>}
      {(evidence.isFetching || evidence.isError) && evidence.data && <p className="price-review-warning">Previously loaded sales, not confirmed current.</p>}
      {recentSales.length ? <SaleRows sales={recentSales} /> : !evidence.isFetching && <p className="price-review-meta">{e && !e.evidenceNeedsReview && !evidence.isError ? 'No matching recent sales in stored evidence.' : 'No verified recent sale detail available.'}</p>}
      <details><summary>30-day sale history</summary>
        <p className="price-review-meta">Older sales are context only; they cannot override recent contradictions.</p>
        <p className="price-review-meta">{e?.windowStart} to {e?.windowEnd} UTC · {e?.compCount ?? 0} sales · Context median {e && e.compCount > 0 ? formatCents(e.medianCents) : 'Unavailable'}</p>
        <SaleRows sales={saleRows} />
      </details>
      <p className="price-review-meta">CardLadder source-reported USD sold amounts, not independently verified settlement. Stored evidence only; navigation does not acquire comps.</p>
      <details><summary>Source diagnostics</summary>
        {e ? <dl className="price-review-diagnostics">
          <div><dt>Evidence</dt><dd>{evidenceLabel(e)}{e.evidenceReason && ` · ${e.evidenceReason}`}</dd></div>
          <div><dt>Physical availability</dt><dd>{availabilityLabels[e.availability]}{!e.canPack && ' · Not eligible to pack'}</dd></div>
          <div><dt>CardLadder refreshed</dt><dd>{showTime(e.refreshedAt)}</dd></div>
          <div><dt>Stored DH listed price</dt><dd>{e.listedPriceCents > 0 ? formatCents(e.listedPriceCents) : 'Not set'}{e.priceMismatch && ' · Differs from asking'}{e.priceAssociationUnclear && ' · Association unverified'}</dd></div>
          <div><dt>DH last synced</dt><dd>{showTime(e.listingSyncedAt)}</dd></div>
          <div><dt>Assessment policy</dt><dd>{e.policyVersion}</dd></div>
        </dl> : <p>Evaluation unavailable. Retry the evidence read.</p>}
      </details>
    </section>}
  </div>;
}

function SaleRows({ sales }: { sales: ShowSale[] }) {
  return <ul className="price-review-sales">{sales.map(sale => {
    const url = safeSourceURL(sale.url);
    return <li key={sale.id}>
      <time dateTime={sale.date}>{sale.date}</time><strong className="num">{formatCents(sale.priceCents)}</strong>
      <span>{url ? <a href={url} target="_blank" rel="noopener noreferrer">{sale.platform || 'Source'} sale ↗</a> : `${sale.platform || 'Source'} · Link unavailable`}</span>
      <span className="price-review-meta">{listingTypeLabel(sale.listingType)}</span>
    </li>;
  })}</ul>;
}
