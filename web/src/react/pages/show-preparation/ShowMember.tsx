import { useState } from 'react';
import type { ShowListItem } from '../../../types/showprep';
import { isShowEvaluation } from '../../../js/api/showprep';
import { useShowRefreshState } from '../../queries/useShowReadiness';
import { useShowListWrites } from '../../queries/useShowPrepQueries';
import { Button, GradeBadge } from '../../ui';
import { formatCents } from '../../utils/formatters';
import ShowEvidenceDisclosure from './ShowEvidence';
import { availabilityLabels, showError, showTime, supportLabels } from './showPrepLabels';

export default function ShowMember({ item, listId, selected, onSelect, stale }: {
  item: ShowListItem; listId: string; selected: boolean; onSelect: () => void; stale: boolean;
}) {
  const { update, remove } = useShowListWrites();
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const refresh = useShowRefreshState();
  const saving = update.isPending || remove.isPending;
  const busy = saving || refresh.busy;
  const e = isShowEvaluation(item.evaluation) ? item.evaluation : undefined;
  const packed = !!item.packedAt;
  const available = e?.availability === 'ready';
  async function change(input: { packed?: boolean; acknowledge?: boolean }) {
    setError(''); setMessage('');
    try {
      await update.mutateAsync({ id: listId, itemId: item.id,
        input: { version: item.version, evaluationVersion: e?.version || '', ...input } });
      setMessage(input.acknowledge ? 'Changes acknowledged.' : input.packed ? 'Packing saved.' : 'Unpacked.');
    } catch (err) { setError(`${showError(err)} Review updated data before retrying.`); }
  }
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
      <div className="show-actions">
        <label className="show-check"><input type="checkbox" aria-label={`Packed ${item.certNumber}`} checked={packed}
          disabled={busy || (!packed && (stale || !e?.canPack || !available))}
          onChange={event => void change({ packed: event.target.checked })} />Packed</label>
        <Button variant="ghost" size="sm" aria-label={`Remove ${item.certNumber} from show`} disabled={busy} onClick={() => void removeMember()}>Remove</Button>
      </div>
    </div>
    {!e && <p className="text-[var(--warning)]">Availability unknown. Evaluation missing; packing disabled.</p>}
    {e && !available && <p className="text-xs text-[var(--warning)] mt-2">{availabilityLabels[e.availability] ?? availabilityLabels.unknown}. {packed ? 'Packing history retained. Unpack or remove explicitly.' : 'Cannot pack in this state.'}</p>}
    {packed && <p className="text-xs text-[var(--text-muted)]">Packed {showTime(item.packedAt)}</p>}
    <div className="show-warnings">
      {item.priceChanged && <p>Price changed{packed ? ': check the physical sticker' : ''}. Previously acknowledged {item.acknowledgedPriceCents > 0 ? formatCents(item.acknowledgedPriceCents) : 'no listed price'}.</p>}
      {item.supportChanged && <p>Support changed. Previously {supportLabels[item.acknowledgedStatus] ?? 'unknown'}.</p>}
    </div>
    <ShowEvidenceDisclosure purchaseId={item.purchaseId} certNumber={item.certNumber} evaluation={e}
      actions={<label className="show-check text-[var(--text-muted)]"><input type="checkbox" checked={selected} onChange={onSelect} aria-label={`Refresh evidence for ${item.certNumber}`} />Select for evidence refresh</label>} />
    {(item.priceChanged || item.supportChanged) && <Button variant="secondary" size="sm" aria-label={`Acknowledge changes ${item.certNumber}`}
      disabled={busy || stale || !e?.version} onClick={() => void change({ acknowledge: true })}>Acknowledge changes</Button>}
    {saving && <p role="status">Saving…</p>}
    {error && <p role="alert" className="text-[var(--danger)]">{error}</p>}
    {message && <p role="status" className="text-[var(--text-muted)]">{message}</p>}
  </article>;
}
