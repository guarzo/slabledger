import { useMemo, useState } from 'react';
import { usePsaExchangeOpportunities } from '../queries/usePsaExchangeQueries';
import { Breadcrumb } from '../ui';
import CardShell from '../ui/CardShell';
import Button from '../ui/Button';
import OpportunitiesTable from './psa-exchange/OpportunitiesTable';
import OpportunitiesTableSkeleton from './psa-exchange/OpportunitiesTableSkeleton';
import Toolbar from './psa-exchange/Toolbar';
import {
  applyFilters,
  applySort,
  defaultDirFor,
  defaultFilters,
  groupByDescription,
  topDecileThreshold,
  type Filters,
  type QuickView,
  type SortDir,
  type SortKey,
} from './psa-exchange/utils';

export default function PsaExchangePage() {
  const { data, isLoading, error, refetch } = usePsaExchangeOpportunities();

  const [filters, setFilters] = useState<Filters>(defaultFilters);
  const [quickView, setQuickView] = useState<QuickView>('all');
  const [sortKey, setSortKey] = useState<SortKey>('score');
  const [sortDir, setSortDir] = useState<SortDir>('desc');
  const [groupDuplicates, setGroupDuplicates] = useState(true);

  const allRows = useMemo(() => data?.opportunities ?? [], [data?.opportunities]);

  const filtered = useMemo(
    () => applyFilters(allRows, filters, quickView),
    [allRows, filters, quickView],
  );

  const sorted = useMemo(
    () => applySort(filtered, sortKey, sortDir),
    [filtered, sortKey, sortDir],
  );

  const groups = useMemo(
    () => (groupDuplicates ? groupByDescription(sorted) : null),
    [sorted, groupDuplicates],
  );

  const topDecileScore = useMemo(
    () => topDecileThreshold(allRows.map((r) => r.score)),
    [allRows],
  );

  const handleQuickView = (v: QuickView) => {
    setQuickView(v);
    if (v === 'takeAtList') {
      setSortKey('edgeAtOffer');
      setSortDir('desc');
    } else {
      setSortKey('score');
      setSortDir('desc');
    }
  };

  const handleSort = (k: SortKey) => {
    if (sortKey === k) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'));
    } else {
      setSortKey(k);
      setSortDir(defaultDirFor(k));
    }
  };

  const visibleCount = groups ? groups.length : sorted.length;

  return (
    <div className="p-6 space-y-4">
      <Breadcrumb items={[{ label: 'Opportunities' }, { label: 'PSA-Exchange' }]} />
      <header className="flex items-start justify-between gap-4">
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
        ) : !isLoading && !error ? (
          <span className="text-xs text-[var(--text-muted)]">PSA-Exchange token not configured</span>
        ) : null}
      </header>

      {isLoading && <OpportunitiesTableSkeleton />}

      {error && (
        <CardShell>
          <div className="p-4 space-y-2">
            <p className="text-sm text-[var(--danger)]">Failed to load PSA-Exchange opportunities.</p>
            <Button onClick={() => refetch()}>Retry</Button>
          </div>
        </CardShell>
      )}

      {data && !isLoading && !error && (
        <>
          <CardShell>
            <div className="p-3">
              <Toolbar
                quickView={quickView}
                onQuickViewChange={handleQuickView}
                filters={filters}
                onFiltersChange={setFilters}
                groupDuplicates={groupDuplicates}
                onGroupDuplicatesChange={setGroupDuplicates}
              />
            </div>
          </CardShell>

          <div className="text-xs text-[var(--text-muted)]">
            Showing {visibleCount} of {data.afterFilter} listings · {data.totalCatalogPokemon} total Pokemon
            {data.enrichmentErrors > 0 && ` · ${data.enrichmentErrors} enrichment errors`}
          </div>

          <CardShell>
            <OpportunitiesTable
              rows={sorted}
              groups={groups}
              sortKey={sortKey}
              sortDir={sortDir}
              onSort={handleSort}
              topDecileScore={topDecileScore}
            />
          </CardShell>
        </>
      )}
    </div>
  );
}
