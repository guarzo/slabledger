import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import type { ShowEvaluation, SupportStatus } from '../../../types/showprep';
import { supportLabels } from './showPrepLabels';
import './show-preparation.css';

export interface ShowFilters {
  support: SupportStatus | 'all';
  evaluations: Record<string, ShowEvaluation>;
}
export function ShowInventoryFilters({ filters, setSupport, count, pending, failed, onRetry, fetching, children }: {
  filters: ShowFilters; setSupport: (value: SupportStatus | 'all') => void;
  count: number; pending: boolean; failed: number; onRetry: () => void; fetching: boolean; children?: ReactNode;
}) {
  return <div className="show-toolbar">
    <label>Support
      <select aria-label="Price support" className="show-input" value={filters.support} onChange={e => setSupport(e.target.value as SupportStatus | 'all')}>
        <option value="all">All price support</option>
        {Object.entries(supportLabels).map(([key, label]) => <option key={key} value={key}>{label}</option>)}
      </select>
    </label>
    <span className="show-match-count tabular-nums" role="status">{count} {count === 1 ? 'card' : 'cards'} shown{pending ? ' · Reading…' : ''}</span>
    <Link className="show-link" to="/shows">Shows</Link>
    {failed > 0 && <p role="alert" className="w-full text-[var(--warning)]">{failed} evaluations unavailable. <button className="show-link" disabled={fetching} onClick={onRetry}>Retry evaluation</button></p>}
    {children}
  </div>;
}
