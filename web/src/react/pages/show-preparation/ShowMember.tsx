import { useState } from 'react';
import type { ShowListItem } from '../../../types/showprep';
import { isShowEvaluation } from '../../../js/api/showprep';
import { useShowListWrites } from '../../queries/useShowPrepQueries';
import { Button, GradeBadge } from '../../ui';
import { formatCents } from '../../utils/formatters';
import ShowEvidenceDisclosure from './ShowEvidence';
import { availabilityLabels, showError, showTime } from './showPrepLabels';

export default function ShowMember({ item, listId }: { item: ShowListItem; listId: string }) {
  const { remove } = useShowListWrites();
  const [error, setError] = useState('');
  const e = isShowEvaluation(item.evaluation) ? item.evaluation : undefined;
  async function removeMember() {
    setError('');
    try { await remove.mutateAsync({ id: listId, itemId: item.id }); }
    catch (err) { setError(showError(err)); }
  }
  return <article className="show-member" aria-label={`Show member ${item.certNumber}`}>
    <div className="show-member-header">
      <div className="show-member-identity">
        <h3>{item.cardName || 'Saved card'}</h3>
        <div className="text-xs text-[var(--text-muted)] tabular-nums">Cert {item.certNumber || 'Unknown'} · <GradeBadge grader={item.grader} grade={item.grade} size="sm" /></div>
      </div>
      <Button variant="ghost" size="sm" aria-label={`Remove ${item.certNumber} from show`} disabled={remove.isPending} onClick={() => void removeMember()}>Remove</Button>
    </div>
    {!e && <p className="text-[var(--warning)]">Availability unknown. Check inventory before scanning.</p>}
    {e && e.availability !== 'ready' && <p className="text-xs text-[var(--warning)] mt-2">{availabilityLabels[e.availability] ?? availabilityLabels.unknown}. Confirmation history retained; not currently ready for this show.</p>}
    <p className="text-xs text-[var(--text-muted)]">{item.packedAt ? `Confirmed ${showTime(item.packedAt)}` : 'Unconfirmed. Scan the checked slab to confirm.'}</p>
    <div className="show-warnings">
      {item.priceChanged && <p>Price changed. Previously acknowledged {item.acknowledgedPriceCents > 0 ? formatCents(item.acknowledgedPriceCents) : 'no price'}.</p>}
      {item.supportChanged && <p>Support changed. Previously recorded status: {item.acknowledgedStatus || 'unknown'}.</p>}
      {(item.priceChanged || item.supportChanged) && <p>Check sticker and rescan to confirm.</p>}
    </div>
    <ShowEvidenceDisclosure purchaseId={item.purchaseId} certNumber={item.certNumber} evaluation={e} />
    {remove.isPending && <p role="status">Removing…</p>}
    {error && <p role="alert" className="text-[var(--danger)]">{error}</p>}
  </article>;
}
