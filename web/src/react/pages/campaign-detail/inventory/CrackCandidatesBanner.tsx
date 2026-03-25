import { useState } from 'react';
import type { CrackAnalysis } from '../../../../types/campaigns';
import { formatCents } from '../../../utils/formatters';
import { useCrackCandidates } from '../../../queries/useCampaignQueries';

export default function CrackCandidatesBanner({ campaignId }: { campaignId: string }) {
  const { data: allCandidates = [] } = useCrackCandidates(campaignId);
  const [expanded, setExpanded] = useState(false);

  const candidates = allCandidates.filter((c: CrackAnalysis) => c.isCrackCandidate);

  if (candidates.length === 0) return null;

  return (
    <div className="mb-3 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)] overflow-hidden">
      <button
        type="button"
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center justify-between px-4 py-2.5 text-left hover:bg-[var(--surface-2)]/30 transition-colors"
      >
        <span className="text-sm text-[var(--text)]">
          <span className="font-semibold text-[var(--warning)]">{candidates.length}</span>
          {' '}crack candidate{candidates.length !== 1 ? 's' : ''} found
          <span className="text-[var(--text-muted)]"> — cards where selling raw may be more profitable than selling graded</span>
        </span>
        <span className="text-xs text-[var(--text-muted)]">{expanded ? '\u25B2' : '\u25BC'}</span>
      </button>
      {expanded && (
        <div className="px-4 pb-3 overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-[var(--text-muted)] text-xs">
                <th className="text-left py-2 px-2 font-medium">Card Name</th>
                <th className="text-center py-2 px-2 font-medium">Grade</th>
                <th className="text-right py-2 px-2 font-medium">Cost</th>
                <th className="text-right py-2 px-2 font-medium">Raw Market</th>
                <th className="text-right py-2 px-2 font-medium">Graded Net</th>
                <th className="text-right py-2 px-2 font-medium">Crack Net</th>
                <th className="text-right py-2 px-2 font-medium">Advantage</th>
              </tr>
            </thead>
            <tbody>
              {candidates.map((c: CrackAnalysis) => (
                <tr key={c.purchaseId} className="border-t border-[var(--surface-2)]">
                  <td className="py-2 px-2 text-[var(--text)]">{c.cardName}</td>
                  <td className="py-2 px-2 text-center text-[var(--text)]">PSA {c.grade}</td>
                  <td className="py-2 px-2 text-right text-[var(--text)]">{formatCents(c.costBasisCents)}</td>
                  <td className="py-2 px-2 text-right text-[var(--text)]">{formatCents(c.rawMarketCents)}</td>
                  <td className={`py-2 px-2 text-right ${c.gradedNetCents >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
                    {formatCents(c.gradedNetCents)}
                  </td>
                  <td className={`py-2 px-2 text-right ${c.crackNetCents >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
                    {formatCents(c.crackNetCents)}
                  </td>
                  <td className="py-2 px-2 text-right text-[var(--success)] font-medium">
                    +{formatCents(c.crackAdvantageCents)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
