import { usePsaExchangeOpportunities } from '../queries/usePsaExchangeQueries';
import { Breadcrumb, GradeBadge } from '../ui';
import CardShell from '../ui/CardShell';
import Button from '../ui/Button';

const dollar = (n: number) =>
  n.toLocaleString('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 0 });
const pct = (n: number) => `${(n * 100).toFixed(1)}%`;

export default function PsaExchangePage() {
  const { data, isLoading, error, refetch } = usePsaExchangeOpportunities();

  return (
    <div className="p-6 space-y-4">
      <Breadcrumb items={[{ label: 'Opportunities' }, { label: 'PSA-Exchange' }]} />
      <header className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold">PSA-Exchange Opportunities</h1>
          <p className="text-sm text-[var(--text-muted)]">
            Pokemon listings ranked by tiered offer × velocity. Read-only — make offers on PSA-Exchange.
          </p>
        </div>
        {data?.categoryUrl ? (
          <a
            href={data.categoryUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1 px-3 py-2 rounded-md bg-[var(--brand-500)] text-white text-sm hover:bg-[var(--brand-600)]"
          >
            Open Pokemon catalog ↗
          </a>
        ) : (
          <span className="text-xs text-[var(--text-muted)]">PSA-Exchange token not configured</span>
        )}
      </header>

      {isLoading && <CardShell><div className="p-4 text-sm text-[var(--text-muted)]">Loading…</div></CardShell>}

      {error && (
        <CardShell>
          <div className="p-4 space-y-2">
            <p className="text-sm text-[var(--danger)]">Failed to load PSA-Exchange opportunities.</p>
            <Button onClick={() => refetch()}>Retry</Button>
          </div>
        </CardShell>
      )}

      {data && (
        <>
          <div className="text-xs text-[var(--text-muted)]">
            Showing {data.afterFilter} of {data.totalCatalogPokemon} Pokemon listings.
            {data.enrichmentErrors > 0 && ` (${data.enrichmentErrors} enrichment errors)`}
          </div>

          {data.opportunities.length === 0 ? (
            <CardShell>
              <div className="p-6 text-center text-sm text-[var(--text-muted)]">
                No PSA-Exchange opportunities match the current filters.
              </div>
            </CardShell>
          ) : (
            <CardShell>
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead className="text-xs uppercase text-[var(--text-muted)] border-b border-[var(--surface-2)]">
                    <tr>
                      <th className="text-left p-2"></th>
                      <th className="text-left p-2">Cert</th>
                      <th className="text-left p-2">Description</th>
                      <th className="text-left p-2">Grade</th>
                      <th className="text-right p-2">List</th>
                      <th className="text-right p-2">Target Offer</th>
                      <th className="text-right p-2">Comp</th>
                      <th className="text-right p-2">Vel 1m / 3m</th>
                      <th className="text-right p-2">Edge @ Offer</th>
                      <th className="text-right p-2">List Runway</th>
                      <th className="text-right p-2">Score</th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.opportunities.map((row) => (
                      <tr key={row.cert} className="border-b border-[var(--surface-2)]/40 hover:bg-[var(--surface-1)]/40">
                        <td className="p-2">
                          {row.frontImage && (
                            <img src={row.frontImage} alt="" className="h-12 w-9 object-cover rounded-sm" loading="lazy" />
                          )}
                        </td>
                        <td className="p-2 font-mono text-xs">{row.cert}</td>
                        <td className="p-2 max-w-[28rem]">
                          <div>{row.description || row.name}</div>
                          {row.mayTakeAtList && (
                            <span className="inline-block mt-1 text-[10px] px-1.5 py-0.5 rounded-md bg-[var(--success)]/15 text-[var(--success)]">
                              May take at list
                            </span>
                          )}
                        </td>
                        <td className="p-2"><GradeBadge grade={row.grade} /></td>
                        <td className="p-2 text-right tabular-nums">{dollar(row.listPrice)}</td>
                        <td className="p-2 text-right tabular-nums">{dollar(row.targetOffer)}</td>
                        <td className="p-2 text-right tabular-nums">{dollar(row.comp)}</td>
                        <td className="p-2 text-right tabular-nums">{row.velocityMonth} / {row.velocityQuarter}</td>
                        <td className="p-2 text-right tabular-nums">{pct(row.edgeAtOffer)}</td>
                        <td className={`p-2 text-right tabular-nums ${row.listRunwayPct < 0 ? 'text-[var(--success)]' : ''}`}>
                          {pct(row.listRunwayPct)}
                        </td>
                        <td className="p-2 text-right tabular-nums">{row.score.toFixed(3)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </CardShell>
          )}
        </>
      )}
    </div>
  );
}
