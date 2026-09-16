import type { AgingItem } from '../../../types/campaigns';
import type { ShowEvaluation } from '../../../types/showprep';
import { isShowEvaluation } from '../../../js/api/showprep';
import GradeBadge from '../../ui/GradeBadge';
import { formatCents } from '../../utils/formatters';
import { assessmentLabels, gapLabel, priceGroup } from './priceReviewModel';

interface PriceReviewQueueProps {
  rows: AgingItem[]; evaluations: Record<string, ShowEvaluation>; activeId: string | null;
  selected: ReadonlySet<string>; onToggleSelected: (id: string) => void; onFocus: (id: string) => void;
  registerFocusControl: (id: string, node: HTMLButtonElement | null) => void;
}
export function PriceReviewQueue({ rows, evaluations, activeId, selected, onToggleSelected, onFocus, registerFocusControl }: PriceReviewQueueProps) {
  return <div className="price-review-queue">
    <div className="price-review-queue-heading"><span>Card / assessment</span><span>Asking / recent</span></div>
    <ul aria-label="Price review queue">{rows.map(({ purchase: p, campaignName }) => {
      const raw = evaluations[p.id]; const e = isShowEvaluation(raw) ? raw : undefined;
      const recent = e?.recent;
      return <li key={p.id} data-active={activeId === p.id}>
        <label className="price-review-checkbox"><input type="checkbox" aria-label={`Select ${p.cardName}`} checked={selected.has(p.id)} onChange={() => onToggleSelected(p.id)} /></label>
        <button type="button" className="price-review-row" ref={node => registerFocusControl(p.id, node)}
          aria-label={`Review ${p.cardName}`} aria-current={activeId === p.id ? 'true' : undefined} onClick={() => onFocus(p.id)}>
          <span className="price-review-card">
            {campaignName && <span className="price-review-meta">{campaignName}</span>}
            <strong>{p.cardName}</strong>
            <span className="price-review-identity"><GradeBadge grader={p.grader} grade={p.gradeValue} /><span>{p.setName} · {p.certNumber}</span></span>
          </span>
          <span className="price-review-amount"><strong className="num">{!e ? 'Unavailable' : e.localPriceCents > 0 ? formatCents(e.localPriceCents) : 'Not set'}</strong>
            <span className="price-review-meta">{recent && recent.count > 0 ? `${formatCents(recent.medianCents)} recent` : 'No recent reference'}</span></span>
          <span className={`price-review-status price-review-status-${priceGroup(e)}`}>{e ? assessmentLabels[e.status] : 'Evaluation unavailable'}</span>
          <span className="price-review-row-context">{recent && recent.count > 0 ? <>
            <span className={recent.count === 1 && recent.gapPct !== null && recent.gapPct > 0 ? 'price-review-low' : ''}>{gapLabel(recent.gapPct)}</span>
            <span>{recent.count} {recent.count === 1 ? 'sale' : 'sales'} · {recent.latestSaleDate}</span>
          </> : <span>Recent sales unavailable</span>}{e?.evidenceNeedsReview && <span className="price-review-warning">Stored facts, partial or stale</span>}</span>
        </button>
      </li>;
    })}</ul>
  </div>;
}
