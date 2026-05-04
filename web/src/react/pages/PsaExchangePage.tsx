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
    <div className="max-w-6xl mx-auto px-4 space-y-6">
      <Breadcrumb items={[{ label: 'Opportunities' }, { label: 'PSA-Exchange' }]} />
      <header className="flex items-start justify-between gap-4">
        <div>
          <h1 className="page-title">PSA-Exchange Opportunities</h1>
          <p className="text-sm text-[var(--text-muted)] mt-1">
            Pokemon listings ranked by tiered offer × velocity. Read-only; make offers on PSA-Exchange.
          </p>
        </div>
        {data?.categoryUrl ? (
          <a
            href={data.categoryUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1.5 px-3 py-2 rounded-md border border-[var(--surface-3)] bg-[var(--surface-1)] text-sm text-[var(--text)] hover:bg-[var(--surface-2)] hover:border-[var(--brand-500)] transition-colors"
          >
            Open Pokemon catalog
            <span aria-hidden="true">↗</span>
          </a>
        ) : !isLoading && !error ? (
          <span className="text-xs text-[var(--text-muted)]">PSA-Exchange token not configured</span>
        ) : null}
      </header>

      {isLoading && <OpportunitiesTableSkeleton />}

      {error && (
        <div className="rounded-xl border border-[var(--surface-2)] bg-[var(--surface-1)] p-5 space-y-3">
          <p className="text-sm text-[var(--danger)]">Failed to load PSA-Exchange opportunities.</p>
          <p className="text-xs text-[var(--text-muted)] leading-relaxed max-w-xl">
            When this loads, you'll see a ranked table of Pokemon listings on PSA-Exchange — each row scored by tiered offer × velocity, with the top decile highlighted. Common causes: PSA-Exchange token missing or expired (configure in Admin → Integrations), or PSA-Exchange API is rate-limiting.
          </p>
          <Button variant="secondary" onClick={() => refetch()}>Retry</Button>
        </div>
      )}

      {data && !isLoading && !error && (
        <>
          <CardShell padding="sm">
            <Toolbar
              quickView={quickView}
              onQuickViewChange={handleQuickView}
              filters={filters}
              onFiltersChange={setFilters}
              groupDuplicates={groupDuplicates}
              onGroupDuplicatesChange={setGroupDuplicates}
            />
          </CardShell>

          <div className="text-xs text-[var(--text-muted)]">
            Showing {visibleCount} of {data.afterFilter} listings · {data.totalCatalogPokemon} total Pokemon
            {data.enrichmentErrors > 0 && ` · ${data.enrichmentErrors} enrichment errors`}
          </div>

          <CardShell padding="none">
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
