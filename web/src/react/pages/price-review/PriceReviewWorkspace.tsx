import { useLayoutEffect, useRef, useState } from 'react';
import type { AgingItem } from '../../../types/campaigns';
import type { ShowEvaluation } from '../../../types/showprep';
import Button from '../../ui/Button';
import Select from '../../ui/Select';
import { reviewFilters, type PriceReviewSort } from './priceReviewModel';
import type { usePriceReviewState } from './usePriceReviewState';
import { PriceReviewPanel } from './PriceReviewPanel';
import { PriceReviewQueue } from './PriceReviewQueue';
import './price-review.css';

export interface PriceReviewWorkspaceProps {
  items: AgingItem[]; evaluations: Record<string, ShowEvaluation>;
  review: ReturnType<typeof usePriceReviewState>;
  selected: ReadonlySet<string>; onToggleSelected: (id: string) => void;
  onSavePrice: (id: string, priceCents: number) => Promise<void>;
  onRecheckInventory?: () => Promise<boolean>;
  detailNavigationKey?: string;
}
export function PriceReviewWorkspace({ items, evaluations, review, selected, onToggleSelected, onSavePrice, onRecheckInventory, detailNavigationKey }: PriceReviewWorkspaceProps) {
  const [mobileDetail, setMobileDetail] = useState(!!detailNavigationKey);
  const focusControls = useRef(new Map<string, HTMLButtonElement>());
  const panel = useRef<HTMLElement>(null);
  const queueScroll = useRef(0);
  const returning = useRef(false);
  const returnId = useRef<string | null>(null);
  const allFilterControl = useRef<HTMLButtonElement>(null);
  const active = items.find(item => item.purchase.id === review.activeId);
  const position = review.rows.findIndex(item => item.purchase.id === review.activeId);
  const orderedSort = review.sort === 'attention' || review.sort === 'supported';

  // URL history is explicit detail intent; changing saved facts is not.
  useLayoutEffect(() => {
    if (detailNavigationKey) setMobileDetail(true);
  }, [detailNavigationKey]);

  useLayoutEffect(() => {
    if (!mobileDetail && returning.current) {
      returning.current = false;
      const activeControl = review.activeId ? focusControls.current.get(review.activeId) : undefined;
      const target = activeControl ?? (returnId.current ? focusControls.current.get(returnId.current) : undefined) ?? allFilterControl.current;
      target?.focus({ preventScroll: true });
      if (window.matchMedia('(max-width: 899px)').matches) {
        window.scrollTo({ top: queueScroll.current, behavior: 'instant' });
        // A removed row's old scroll position need not contain its surviving neighbor.
        if (!activeControl) target?.scrollIntoView({ block: 'nearest', behavior: 'instant' });
      }
    } else if (mobileDetail && window.matchMedia('(max-width: 899px)').matches) {
      panel.current?.querySelector<HTMLElement>('[data-price-detail-heading]')?.focus({ preventScroll: true });
      panel.current?.scrollIntoView({ block: 'start', behavior: 'instant' });
    }
  }, [mobileDetail, review.activeId]);

  function focus(id: string) {
    queueScroll.current = window.scrollY;
    review.focus(id); setMobileDetail(true);
  }
  return <section aria-label="Price review" className="price-review" data-mobile-detail={mobileDetail}>
    <div className="price-review-controls">
      <div className="price-review-filters" role="group" aria-label="Price assessment filters">
        {reviewFilters.map(filter => <Button key={filter.value} ref={filter.value === 'all' ? allFilterControl : undefined} variant={review.filter === filter.value ? 'secondary' : 'ghost'} size="sm"
          aria-pressed={review.filter === filter.value} onClick={() => review.setFilter(filter.value)}>{filter.label} <span className="num">{review.counts[filter.value]}</span></Button>)}
      </div>
      <div className="price-review-toolbar">
        <span className="price-review-meta">{review.rows.length} of {review.counts.all} cards</span>
        <Select aria-label="Price review sort" value={review.sort} selectSize="sm" onChange={event => review.setSort(event.target.value as PriceReviewSort)} options={[
          { value: 'attention', label: 'Pricing attention first' }, { value: 'supported', label: 'Supported first' },
          { value: 'asking', label: 'Asking price' }, { value: 'recent', label: 'Recent reference' }, { value: 'gap', label: 'Gap below asking' },
        ]} />
        {!orderedSort && <Button variant="ghost" size="sm" onClick={() => review.setDescending(!review.descending)}>{review.descending ? 'High to low' : 'Low to high'}</Button>}
        <span className="price-review-meta">Recent: newest matching sales within seven UTC dates</span>
      </div>
    </div>
    <div className="price-review-columns">
      <div className="price-review-list">
        {items.length === 0 ? <p className="price-review-empty">No inventory to review</p> : review.rows.length === 0
          ? <p className="price-review-empty">No cards match. Change the price filter or search.</p>
          : <PriceReviewQueue rows={review.rows} evaluations={evaluations} activeId={review.activeId} selected={selected}
            onToggleSelected={onToggleSelected} onFocus={focus} registerFocusControl={(id, node) => {
              if (node) focusControls.current.set(id, node); else focusControls.current.delete(id);
            }} />}
      </div>
      <section aria-label="Price details" className="price-review-detail" ref={panel}>
        <div className="price-review-detail-nav">
          <Button className="price-review-back" variant="secondary" size="sm" onClick={() => {
            returnId.current = review.returnFocusId(); returning.current = true; setMobileDetail(false);
          }}>Back to inventory list</Button>
          <span className="price-review-meta">{position >= 0 ? `Price review · ${position + 1} of ${review.rows.length}` : 'Price review'}</span>
          <div className="price-review-actions">
            <Button variant="ghost" size="sm" aria-label="Previous card" disabled={!review.rows.length || position === 0} onClick={() => review.move(-1)}>↑</Button>
            <Button variant="ghost" size="sm" aria-label="Next card" disabled={!review.rows.length || position === review.rows.length - 1} onClick={() => review.move(1)}>↓</Button>
          </div>
        </div>
        {review.outsideFilter && active && <p role="status" className="price-review-warning">Outside the current filter. This card stays open until you move on.</p>}
        {review.activeId ? <PriceReviewPanel purchaseId={review.activeId} item={active} evaluation={evaluations[review.activeId]}
          draft={review.drafts[review.activeId]} onDraftChange={draft => review.setDraft(review.activeId!, draft)}
          onClearDraft={() => review.clearDraft(review.activeId!)} onSavePrice={onSavePrice} onRecheckInventory={onRecheckInventory}
          save={review.saves[review.activeId]} onSaveResultChange={result => review.setSaveResult(review.activeId!, result)}
          onSaveRechecked={rechecked => review.markSaveRechecked(review.activeId!, rechecked)} />
          : <p className="price-review-empty">Select a card to review its saved asking price.</p>}
      </section>
    </div>
  </section>;
}
