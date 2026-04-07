import type { ShopifyPriceSyncMatch } from '../../../types/campaigns';
import { formatCents } from '../../utils/formatters';

export function IntelDetail({ intel }: { intel: NonNullable<ShopifyPriceSyncMatch['intel']> }) {
  return (
    <div className="grid grid-cols-3 gap-6 text-xs">
      {/* Left: Insights */}
      <div>
        {intel.insightHeadline && (
          <>
            <div className="font-semibold text-[var(--text)] mb-1">{intel.insightHeadline}</div>
            {intel.insightDetail && (
              <div className="text-[var(--text-muted)] leading-relaxed">{intel.insightDetail}</div>
            )}
          </>
        )}
        {intel.fetchedAt && (
          <div className="text-[10px] text-[var(--text-muted)] mt-2">
            Updated: {new Date(intel.fetchedAt).toLocaleDateString()}
          </div>
        )}
      </div>

      {/* Center: Recent Sales */}
      <div>
        <div className="font-semibold text-[var(--text-muted)] uppercase tracking-wide text-[10px] mb-2">Recent Sales</div>
        {intel.recentSales && intel.recentSales.length > 0 ? (
          <table className="w-full text-[11px]">
            <thead>
              <tr className="text-[var(--text-muted)]">
                <th className="text-left font-medium pb-1">Date</th>
                <th className="text-left font-medium pb-1">Grade</th>
                <th className="text-right font-medium pb-1">Price</th>
                <th className="text-right font-medium pb-1">Platform</th>
              </tr>
            </thead>
            <tbody>
              {intel.recentSales.map((sale, i) => (
                <tr key={i} className="text-[var(--text)]">
                  <td className="py-0.5">{new Date(sale.soldAt).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}</td>
                  <td className="py-0.5">{sale.grade}</td>
                  <td className="py-0.5 text-right">{formatCents(sale.priceCents)}</td>
                  <td className="py-0.5 text-right text-[var(--text-muted)]">{sale.platform}</td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <div className="text-[var(--text-muted)] italic">No recent sales</div>
        )}
      </div>

      {/* Right: Population & ROI */}
      <div>
        {intel.population && intel.population.length > 0 && (
          <div className="mb-3">
            <div className="font-semibold text-[var(--text-muted)] uppercase tracking-wide text-[10px] mb-1">PSA Population</div>
            <div className="flex flex-wrap gap-x-3 gap-y-0.5 text-[var(--text)]">
              {intel.population.map((p) => (
                <span key={p.grade}>PSA {p.grade}: <span className="font-semibold">{p.count.toLocaleString()}</span></span>
              ))}
            </div>
          </div>
        )}
        {intel.gradingROI && intel.gradingROI.length > 0 && (
          <div>
            <div className="font-semibold text-[var(--text-muted)] uppercase tracking-wide text-[10px] mb-1">Grading ROI</div>
            <div className="flex flex-col gap-0.5 text-[var(--text)]">
              {intel.gradingROI.map((r) => (
                <span key={r.grade}>
                  PSA {r.grade}: <span className={r.roi >= 0 ? 'text-[var(--success)]' : 'text-red-400'}>
                    {r.roi >= 0 ? '+' : ''}{(r.roi * 100).toFixed(0)}% ROI
                  </span>
                  <span className="text-[var(--text-muted)]"> ({formatCents(r.avgSaleCents)} avg)</span>
                </span>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
