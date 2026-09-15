import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import type { ShowEvaluation, SupportStatus } from '../../../types/showprep';
import { Button } from '../../ui';
import { supportLabels } from './showPrepLabels';
import './show-preparation.css';

export { default as ShowSelectionActions } from './ShowSelectionBar';
export interface ShowFilters {
  support: SupportStatus | 'all';
  selecting: boolean;
  includeNotReceived: boolean;
  evaluations: Record<string, ShowEvaluation>;
}
export function ShowInventoryFilters({ filters, setSupport, setSelecting, setIncludeNotReceived, count, pending, failed, onRetry, fetching, children }: {
  filters: ShowFilters; setSupport: (value: SupportStatus | 'all') => void;
  setSelecting: (value: boolean) => void; setIncludeNotReceived: (value: boolean) => void;
  count: number; pending: boolean; failed: number; onRetry: () => void; fetching: boolean; children?: ReactNode;
}) {
  return <div className="show-toolbar">
    <label>Support
      <select aria-label="Price support" className="show-input" value={filters.support} onChange={e => setSupport(e.target.value as SupportStatus | 'all')}>
        <option value="all">All price support</option>
        {Object.entries(supportLabels).map(([key, label]) => <option key={key} value={key}>{label}</option>)}
      </select>
    </label>
    <Button className="show-mode-toggle" variant={filters.selecting ? 'primary' : 'secondary'} size="sm" aria-pressed={filters.selecting} onClick={() => setSelecting(!filters.selecting)}>Show selection</Button>
    {filters.selecting && <label className="show-check"><input type="checkbox" checked={filters.includeNotReceived} onChange={e => setIncludeNotReceived(e.target.checked)} />Include not received</label>}
    <span className="show-match-count tabular-nums" role="status">{count} {count === 1 ? 'card' : 'cards'} shown{pending ? ' · Reading…' : ''}</span>
    <Link className="show-link" to="/shows">Shows</Link>
    {failed > 0 && <p role="alert" className="w-full text-[var(--warning)]">{failed} evaluations unavailable. <button className="show-link" disabled={fetching} onClick={onRetry}>Retry evaluation</button></p>}
    {children}
  </div>;
}
