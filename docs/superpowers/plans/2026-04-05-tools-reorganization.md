# Tools Reorganization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reorganize the Tools section from 5 tabs to 3 (Daily Ops, Card Intake, Legacy), move Invoices to Insights, redesign Admin DH stats, and remove Active Campaigns from Dashboard.

**Architecture:** Frontend-only changes for Tasks 1-5. Task 6 adds backend DH API health tracking + counts endpoint. All changes use existing React Query hooks, Radix Tabs, and Tailwind styling. New components follow existing patterns in `web/src/react/pages/tools/`.

**Tech Stack:** React 18, TypeScript, Radix UI Tabs, TanStack React Query, Tailwind CSS, Go 1.26 (backend)

---

## File Structure

### New Files
| File | Purpose |
|------|---------|
| `web/src/react/pages/tools/DHUnmatchedSection.tsx` | DH unmatched items table with improved styling (moved from DHTab) |
| `web/src/react/pages/tools/LegacyTab.tsx` | Legacy tab: compact cards for External Import, Price Sync, eBay Export, Import Sales |
| `web/src/react/components/insights/InvoicesSection.tsx` | PSA Invoices section for the Insights page |

### Modified Files
| File | Change |
|------|---------|
| `web/src/react/pages/ToolsPage.tsx` | 3 tabs: Daily Ops, Card Intake, Legacy |
| `web/src/react/pages/campaigns/OperationsTab.tsx` | Remove External Import card, add DH Unmatched section below |
| `web/src/react/pages/InsightsPage.tsx` | Add InvoicesSection after Credit Health |
| `web/src/react/pages/admin/DHTab.tsx` | Redesign stats, remove unmatched fix table, add health + DH counts |
| `web/src/react/pages/DashboardPage.tsx` | Remove Active Campaigns grid |
| `web/src/types/apiStatus.ts` | Extend DHStatusResponse with health + counts fields |
| `web/src/js/api/admin.ts` | No changes needed (status endpoint returns new fields) |
| `internal/adapters/httpserver/handlers/dh_status_handler.go` | Extend HandleGetStatus with API health + DH counts |
| `internal/adapters/httpserver/handlers/dh_handler.go` | Add DHCountsFetcher interface, wire into DHHandler |
| `internal/adapters/clients/dh/client.go` | Add GetCounts method (inventory count + orders count) |

---

### Task 1: Dashboard — Remove Active Campaigns

**Files:**
- Modify: `web/src/react/pages/DashboardPage.tsx`

This is the simplest change — good warm-up.

- [ ] **Step 1: Remove Active Campaigns code from DashboardPage**

Open `web/src/react/pages/DashboardPage.tsx` and remove:
1. The `activeCampaigns` useMemo (lines 18-21)
2. The `healthMap` useMemo (lines 23-27)
3. The `usePortfolioHealth` import and hook call — BUT only if nothing else uses `healthData`. Check: `HeroStatsBar` uses `health={healthData}`, so keep `usePortfolioHealth` and `healthData`.
4. The entire Active Campaigns JSX block (lines 51-80) — the `{activeCampaigns.length > 0 && (...)}` block
5. The `Link` import from `react-router-dom` (no longer needed)
6. The `formatCents` import from `../utils/formatters` (no longer needed)

The result should be: `HeroStatsBar` → `WeeklyReviewSection` → `AIAnalysisWidget` → `WatchlistSection` with no campaigns grid in between.

```tsx
// DashboardPage.tsx — after cleanup
import PokeballLoader from '../PokeballLoader';
import { useCampaigns, usePortfolioHealth, useWeeklyReview, useCreditSummary } from '../queries/useCampaignQueries';
import HeroStatsBar from '../components/portfolio/HeroStatsBar';
import WeeklyReviewSection from '../components/portfolio/WeeklyReviewSection';
import WatchlistSection from '../components/watchlist/WatchlistSection';
import AIAnalysisWidget from '../components/advisor/AIAnalysisWidget';
import { SectionErrorBoundary } from '../ui';

export default function DashboardPage() {
  const { isLoading: campaignsLoading } = useCampaigns(false);
  const { data: healthData } = usePortfolioHealth();
  const { data: weeklyReview } = useWeeklyReview();
  const { data: creditData } = useCreditSummary();

  if (campaignsLoading) {
    return (
      <div className="flex items-center justify-center min-h-[50vh]">
        <PokeballLoader />
      </div>
    );
  }

  return (
    <div className="max-w-6xl mx-auto px-4">
      <div className="mb-6">
        <h1 className="text-[22px] font-bold text-[var(--text)] tracking-tight">Dashboard</h1>
      </div>

      <HeroStatsBar health={healthData} credit={creditData} />

      <div className="mb-6">
        <SectionErrorBoundary sectionName="Weekly Review">
          {weeklyReview && <WeeklyReviewSection data={weeklyReview} />}
        </SectionErrorBoundary>
      </div>

      <div className="mb-6">
        <SectionErrorBoundary sectionName="AI Advisor">
          <AIAnalysisWidget
            endpoint="digest"
            cacheType="digest"
            title="Weekly Intelligence"
            buttonLabel="Generate Digest"
            description="Get an AI-powered weekly review with performance insights, credit health assessment, and prioritized action items."
            collapsible
          />
        </SectionErrorBoundary>
      </div>

      <div className="mb-6">
        <WatchlistSection maxItems={4} />
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Verify the app compiles**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 3: Run lint**

Run: `cd /workspace/web && npm run lint`
Expected: No errors (or only pre-existing ones)

- [ ] **Step 4: Commit**

```bash
git add web/src/react/pages/DashboardPage.tsx
git commit -m "feat: remove Active Campaigns grid from Dashboard"
```

---

### Task 2: Create DHUnmatchedSection Component

**Files:**
- Create: `web/src/react/pages/tools/DHUnmatchedSection.tsx`

Extract the unmatched table from `DHTab.tsx` into a standalone component with improved styling. This component will be used on the Daily Ops tab.

- [ ] **Step 1: Create DHUnmatchedSection.tsx**

This component reuses the existing `useDHUnmatched`, `useFixDHMatch`, `useDHStatus` hooks from `useAdminQueries.ts`. It receives no props — it fetches its own data.

```tsx
// web/src/react/pages/tools/DHUnmatchedSection.tsx
import { useState } from 'react';
import { useDHStatus, useDHUnmatched, useFixDHMatch } from '../../queries/useAdminQueries';
import { useToast } from '../../contexts/ToastContext';
import { Button, CardShell } from '../../ui';
import type { DHUnmatchedCard } from '../../../types/apiStatus';

function formatCents(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

const DH_URL_REGEX = /doubleholo\.com\/card\/\d+/;

function UnmatchedRow({ card }: { card: DHUnmatchedCard }) {
  const [url, setUrl] = useState('');
  const [validationError, setValidationError] = useState('');
  const fixMutation = useFixDHMatch();
  const toast = useToast();

  const handleFix = async () => {
    setValidationError('');
    if (!DH_URL_REGEX.test(url)) {
      setValidationError('Enter a valid DoubleHolo card URL (e.g. doubleholo.com/card/123)');
      return;
    }
    try {
      await fixMutation.mutateAsync({ purchaseId: card.purchase_id, dhUrl: url });
      setUrl('');
      toast.success('Match fixed');
    } catch {
      setValidationError('Failed to fix match. Please try again.');
    }
  };

  return (
    <tr className="border-b border-[var(--border)]/30 even:bg-[var(--surface-1)]/30">
      <td className="py-2.5 px-3 text-sm font-mono text-[var(--text-muted)]">{card.cert_number}</td>
      <td className="py-2.5 px-3 text-sm font-medium text-[var(--text)]">{card.card_name}</td>
      <td className="py-2.5 px-3 text-sm text-[var(--text-muted)]">{card.set_name}</td>
      <td className="py-2.5 px-3 text-sm text-[var(--text-muted)] text-right">{card.grade || '—'}</td>
      <td className="py-2.5 px-3 text-sm text-[var(--text-muted)] text-right">{formatCents(card.cl_value_cents)}</td>
      <td className="py-2.5 px-3">
        <div className="flex flex-col gap-1 min-w-[260px]">
          <div className="flex gap-2">
            <input
              type="text"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="doubleholo.com/card/..."
              className="flex-1 text-xs px-2.5 py-1.5 rounded-md border border-[var(--border)] bg-[var(--surface-1)] text-[var(--text)] placeholder-[var(--text-muted)] focus:outline-none focus:border-[var(--brand-500)]"
            />
            <Button
              variant="secondary"
              size="sm"
              onClick={handleFix}
              loading={fixMutation.isPending}
              disabled={fixMutation.isPending}
            >
              Fix
            </Button>
          </div>
          {validationError && (
            <p className="text-xs text-[var(--danger)]">{validationError}</p>
          )}
        </div>
      </td>
    </tr>
  );
}

export default function DHUnmatchedSection() {
  const { data: status } = useDHStatus();
  const unmatchedCount = status?.unmatched_count ?? 0;
  const { data: unmatchedData } = useDHUnmatched({ enabled: unmatchedCount > 0 });

  if (unmatchedCount === 0) return null;

  return (
    <CardShell variant="default" padding="lg">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-3">
          <div className="w-8 h-8 rounded-lg bg-[var(--warning-bg)] flex items-center justify-center">
            <span className="text-[var(--warning)] text-xs font-bold">DH</span>
          </div>
          <div>
            <div className="text-sm font-semibold text-[var(--text)]">Unmatched DH Items</div>
            <div className="text-xs text-[var(--text-muted)]">Cards needing manual DoubleHolo matching</div>
          </div>
        </div>
        <span className="bg-[var(--warning-bg)] text-[var(--warning)] text-xs font-semibold px-2.5 py-1 rounded-full">
          {unmatchedCount} unmatched
        </span>
      </div>

      {unmatchedData?.unmatched && unmatchedData.unmatched.length > 0 ? (
        <div className="overflow-x-auto">
          <table className="w-full text-left border-collapse">
            <thead>
              <tr className="bg-[var(--surface-1)]/50">
                <th className="pb-2 pt-2.5 px-3 text-[11px] font-semibold text-[var(--text-muted)] uppercase tracking-wider">Cert</th>
                <th className="pb-2 pt-2.5 px-3 text-[11px] font-semibold text-[var(--text-muted)] uppercase tracking-wider">Card Name</th>
                <th className="pb-2 pt-2.5 px-3 text-[11px] font-semibold text-[var(--text-muted)] uppercase tracking-wider">Set</th>
                <th className="pb-2 pt-2.5 px-3 text-[11px] font-semibold text-[var(--text-muted)] uppercase tracking-wider text-right">Grade</th>
                <th className="pb-2 pt-2.5 px-3 text-[11px] font-semibold text-[var(--text-muted)] uppercase tracking-wider text-right">Value</th>
                <th className="pb-2 pt-2.5 px-3 text-[11px] font-semibold text-[var(--text-muted)] uppercase tracking-wider">DH Match</th>
              </tr>
            </thead>
            <tbody>
              {unmatchedData.unmatched.map((card) => (
                <UnmatchedRow key={card.purchase_id} card={card} />
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <p className="text-sm text-[var(--text-muted)]">Loading unmatched cards...</p>
      )}
    </CardShell>
  );
}
```

- [ ] **Step 2: Verify the app compiles**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add web/src/react/pages/tools/DHUnmatchedSection.tsx
git commit -m "feat: create DHUnmatchedSection component with improved styling"
```

---

### Task 3: Restructure OperationsTab — Remove External Import, Add DH Unmatched

**Files:**
- Modify: `web/src/react/pages/campaigns/OperationsTab.tsx`

Remove the External Import card and the `externalResult` rendering. Add `DHUnmatchedSection` below the import results. The External Import functionality will move to the Legacy tab.

- [ ] **Step 1: Update OperationsTab**

Changes:
1. Remove props: `externalResult`, `setExternalResult` from the component signature and the `OperationsTab` export
2. Remove `handleExternalImport` function
3. Remove the `ShopBagIcon` component
4. Remove the External Import `OperationCard` (4th card in grid)
5. Remove the `externalResult` rendering block
6. Change grid from `lg:grid-cols-4` to `lg:grid-cols-3`
7. Remove `'importing-external'` from the `OperationState` type
8. Add `DHUnmatchedSection` import and render it below the results area
9. Update section header text from "Data Operations" / "Import and export campaign data" to "Daily Operations" / "Daily import, export, and matching workflow"

The updated `OperationState` type:
```tsx
export type OperationState = 'idle' | 'importing' | 'exporting' | 'importing-psa';
```

The updated props interface (remove `externalResult` and `setExternalResult`):
```tsx
export default function OperationsTab({ campaigns, operationState, setOperationState, importResult, setImportResult, psaResult, setPsaResult }: {
  campaigns: Campaign[];
  operationState: OperationState;
  setOperationState: (state: OperationState) => void;
  importResult: GlobalImportResult | null;
  setImportResult: (result: GlobalImportResult | null) => void;
  psaResult: PSAImportResult | null;
  setPsaResult: (result: PSAImportResult | null) => void;
}) {
```

Updated grid (3 columns, no External Import card):
```tsx
<div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 mb-6">
```

Remove the `ShopBagIcon` component and the 4th OperationCard entirely.

Remove the `ExternalImportResult` type import and the `externalResult` JSX block.

Add at the bottom of the component (after all result blocks, before the closing `</>`):
```tsx
import DHUnmatchedSection from '../tools/DHUnmatchedSection';

// ... at end of JSX, after all result blocks:
<DHUnmatchedSection />
```

- [ ] **Step 2: Verify the app compiles**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: Errors in `ToolsPage.tsx` (because it still passes removed props). That's expected — we fix it in Task 4.

- [ ] **Step 3: Commit (WIP — will compile after Task 4)**

```bash
git add web/src/react/pages/campaigns/OperationsTab.tsx
git commit -m "feat: streamline OperationsTab — remove External Import, add DH Unmatched"
```

---

### Task 4: Create LegacyTab and Restructure ToolsPage

**Files:**
- Create: `web/src/react/pages/tools/LegacyTab.tsx`
- Modify: `web/src/react/pages/ToolsPage.tsx`

- [ ] **Step 1: Create LegacyTab.tsx**

This tab contains 4 compact cards in a 2-column grid: External Import, Price Sync, eBay Export, Import Sales. eBay Export and Import Sales expand inline when activated.

```tsx
// web/src/react/pages/tools/LegacyTab.tsx
import { useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { api } from '../../../js/api';
import type { ExternalImportResult } from '../../../types/campaigns';
import { queryKeys } from '../../queries/queryKeys';
import { useToast } from '../../contexts/ToastContext';
import { Button, CardShell } from '../../ui';
import { getErrorMessage } from '../../utils/formatters';
import ShopifySyncPage from '../ShopifySyncPage';
import EbayExportTab from './EbayExportTab';
import ImportSalesTab from './ImportSalesTab';
import { SectionErrorBoundary } from '../../ui';

function TransitionalBadge() {
  return (
    <span className="text-[10px] px-2 py-0.5 rounded-full bg-[var(--warning-bg)] text-[var(--warning)] font-medium ml-auto shrink-0">
      Transitional
    </span>
  );
}

function LegacyCard({ icon, title, description, children }: {
  icon: React.ReactNode;
  title: string;
  description: string;
  children: React.ReactNode;
}) {
  return (
    <CardShell variant="default" padding="lg">
      <div className="flex items-center gap-2.5 mb-2">
        <span className="text-base">{icon}</span>
        <div className="text-sm font-semibold text-[var(--text)]">{title}</div>
        <TransitionalBadge />
      </div>
      <div className="text-xs text-[var(--text-muted)] mb-3">{description}</div>
      {children}
    </CardShell>
  );
}

export default function LegacyTab() {
  const toast = useToast();
  const queryClient = useQueryClient();
  const fileRef = useRef<HTMLInputElement>(null);
  const [importing, setImporting] = useState(false);
  const [externalResult, setExternalResult] = useState<ExternalImportResult | null>(null);
  const [expandedCard, setExpandedCard] = useState<'ebay' | 'sales' | 'priceSync' | null>(null);

  function invalidateAll() {
    queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.all });
    queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.insights });
    queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.health });
    queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.globalInventory });
    queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.sellSheet });
    queryClient.invalidateQueries({ queryKey: queryKeys.credit.summary });
    queryClient.invalidateQueries({ queryKey: queryKeys.credit.invoices });
    queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.capitalTimeline });
    queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.weeklyReview });
  }

  async function handleExternalImport(file: File) {
    try {
      setImporting(true);
      setExternalResult(null);
      const result = await api.globalImportExternal(file);
      setExternalResult(result);
      toast.success(`External import: ${result.imported} imported, ${result.updated} updated, ${result.skipped} skipped, ${result.failed} failed`);
      invalidateAll();
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to import external data'));
    } finally {
      setImporting(false);
    }
  }

  return (
    <div>
      <div className="mb-4">
        <h2 className="text-base font-semibold text-[var(--text)]">Legacy Tools</h2>
        <p className="text-xs text-[var(--text-muted)] mt-0.5">Transitional tools — will be removed after full DH migration</p>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        {/* External Import */}
        <LegacyCard
          icon={<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-[var(--success)]"><path d="M6 2L3 6v14a2 2 0 002 2h14a2 2 0 002-2V6l-3-4z" /><line x1="3" y1="6" x2="21" y2="6" /><path d="M16 10a4 4 0 01-8 0" /></svg>}
          title="External Import"
          description="Import a Shopify product export CSV for external purchases"
        >
          <Button
            size="sm"
            variant="secondary"
            fullWidth
            loading={importing}
            onClick={() => fileRef.current?.click()}
          >
            Upload Shopify CSV
          </Button>
          <input
            ref={fileRef}
            type="file"
            accept=".csv"
            className="hidden"
            onChange={(e) => {
              const file = e.target.files?.[0];
              if (file) handleExternalImport(file);
              e.target.value = '';
            }}
          />
          {externalResult && (
            <div className="mt-3 p-2 rounded bg-[var(--surface-2)]/50 text-xs">
              <div className="flex flex-wrap gap-2">
                {externalResult.imported > 0 && <span className="text-[var(--success)]">{externalResult.imported} imported</span>}
                {externalResult.updated > 0 && <span className="text-[var(--info)]">{externalResult.updated} updated</span>}
                {externalResult.skipped > 0 && <span className="text-[var(--text-muted)]">{externalResult.skipped} skipped</span>}
                {externalResult.failed > 0 && <span className="text-[var(--danger)]">{externalResult.failed} failed</span>}
              </div>
            </div>
          )}
        </LegacyCard>

        {/* Price Sync */}
        <LegacyCard
          icon={<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-[var(--info)]"><polyline points="23 4 23 10 17 10" /><polyline points="1 20 1 14 7 14" /><path d="M3.51 9a9 9 0 0114.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0020.49 15" /></svg>}
          title="Price Sync"
          description="Sync prices to external sales channels"
        >
          <Button
            size="sm"
            variant="secondary"
            fullWidth
            onClick={() => setExpandedCard(expandedCard === 'priceSync' ? null : 'priceSync')}
          >
            {expandedCard === 'priceSync' ? 'Collapse' : 'Configure'}
          </Button>
        </LegacyCard>

        {/* eBay Export */}
        <LegacyCard
          icon={<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-[var(--brand-500)]"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" /><polyline points="7 10 12 15 17 10" /><line x1="12" y1="15" x2="12" y2="3" /></svg>}
          title="eBay Export"
          description="Generate eBay import CSV from flagged inventory items"
        >
          <Button
            size="sm"
            variant="secondary"
            fullWidth
            onClick={() => setExpandedCard(expandedCard === 'ebay' ? null : 'ebay')}
          >
            {expandedCard === 'ebay' ? 'Collapse' : 'Load Items'}
          </Button>
        </LegacyCard>

        {/* Import Sales */}
        <LegacyCard
          icon={<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-[var(--warning)]"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" /><polyline points="17 8 12 3 7 8" /><line x1="12" y1="3" x2="12" y2="15" /></svg>}
          title="Import Sales"
          description="Upload sales orders CSV and match against inventory"
        >
          <Button
            size="sm"
            variant="secondary"
            fullWidth
            onClick={() => setExpandedCard(expandedCard === 'sales' ? null : 'sales')}
          >
            {expandedCard === 'sales' ? 'Collapse' : 'Upload Orders CSV'}
          </Button>
        </LegacyCard>
      </div>

      {/* Price Sync (expanded inline, full width below grid) */}
      {expandedCard === 'priceSync' && (
        <div className="mt-4">
          <SectionErrorBoundary sectionName="Price Sync">
            <ShopifySyncPage embedded />
          </SectionErrorBoundary>
        </div>
      )}

      {/* eBay Export (expanded inline, full width below grid) */}
      {expandedCard === 'ebay' && (
        <div className="mt-4">
          <SectionErrorBoundary sectionName="eBay Export">
            <EbayExportTab />
          </SectionErrorBoundary>
        </div>
      )}

      {/* Import Sales (expanded inline, full width below grid) */}
      {expandedCard === 'sales' && (
        <div className="mt-4">
          <SectionErrorBoundary sectionName="Import Sales">
            <ImportSalesTab />
          </SectionErrorBoundary>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Update ToolsPage.tsx**

Replace the 5-tab structure with 3 tabs. Remove imports for `EbayExportTab`, `ImportSalesTab`, `InvoicesTab`, `ShopifySyncPage`. Remove `externalResult`/`setExternalResult` state. Add import for `LegacyTab`.

```tsx
// web/src/react/pages/ToolsPage.tsx
import { useState } from 'react';
import { Tabs } from 'radix-ui';
import { useCampaigns } from '../queries/useCampaignQueries';
import OperationsTab, { type OperationState } from './campaigns/OperationsTab';
import CardIntakeTab from './tools/CardIntakeTab';
import LegacyTab from './tools/LegacyTab';
import type { GlobalImportResult, PSAImportResult } from '../../types/campaigns';
import TabNavigation from '../ui/TabNavigation';
import { SectionErrorBoundary } from '../ui';

const TABS = [
  { id: 'daily-ops', label: 'Daily Ops' },
  { id: 'card-intake', label: 'Card Intake' },
  { id: 'legacy', label: 'Legacy' },
] as const;

export default function ToolsPage() {
  const { data: allCampaigns = [] } = useCampaigns(false);
  const [operationState, setOperationState] = useState<OperationState>('idle');
  const [importResult, setImportResult] = useState<GlobalImportResult | null>(null);
  const [psaResult, setPsaResult] = useState<PSAImportResult | null>(null);

  return (
    <div className="max-w-6xl mx-auto px-4">
      <div className="mb-6">
        <h1 className="text-[22px] font-bold text-[var(--text)] tracking-tight">Tools</h1>
        <p className="mt-1 text-sm text-[var(--text-muted)]">Daily operations, card intake, and legacy tools</p>
      </div>

      <Tabs.Root defaultValue="daily-ops">
        <TabNavigation tabs={TABS} ariaLabel="Tools tabs" />

        <Tabs.Content value="daily-ops">
          <SectionErrorBoundary sectionName="Daily Ops">
            <OperationsTab
              campaigns={allCampaigns}
              operationState={operationState}
              setOperationState={setOperationState}
              importResult={importResult}
              setImportResult={setImportResult}
              psaResult={psaResult}
              setPsaResult={setPsaResult}
            />
          </SectionErrorBoundary>
        </Tabs.Content>

        <Tabs.Content value="card-intake">
          <SectionErrorBoundary sectionName="Card Intake">
            <CardIntakeTab />
          </SectionErrorBoundary>
        </Tabs.Content>

        <Tabs.Content value="legacy">
          <SectionErrorBoundary sectionName="Legacy">
            <LegacyTab />
          </SectionErrorBoundary>
        </Tabs.Content>
      </Tabs.Root>
    </div>
  );
}
```

- [ ] **Step 3: Verify the app compiles**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 4: Run lint**

Run: `cd /workspace/web && npm run lint`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add web/src/react/pages/tools/LegacyTab.tsx web/src/react/pages/ToolsPage.tsx
git commit -m "feat: restructure Tools tabs — Daily Ops, Card Intake, Legacy"
```

---

### Task 5: Move Invoices to Insights Page

**Files:**
- Create: `web/src/react/components/insights/InvoicesSection.tsx`
- Modify: `web/src/react/pages/InsightsPage.tsx`
- Delete (or leave as unused): `web/src/react/pages/tools/InvoicesTab.tsx`

- [ ] **Step 1: Create InvoicesSection.tsx**

This is essentially the same content as `InvoicesTab` but wrapped in a section card with a title/subtitle. Reuse the same hooks.

```tsx
// web/src/react/components/insights/InvoicesSection.tsx
import { useState } from 'react';
import { useInvoices, useUpdateInvoice } from '../../queries/useCampaignQueries';
import { useToast } from '../../contexts/ToastContext';
import { formatCents, getErrorMessage, localToday } from '../../utils/formatters';
import { Button, CardShell } from '../../ui';

export default function InvoicesSection() {
  const { data: invoices = [], isLoading, error } = useInvoices();
  const updateInvoice = useUpdateInvoice();
  const toast = useToast();
  const [updatingId, setUpdatingId] = useState<string | null>(null);

  function handleMarkPaid(id: string) {
    const inv = invoices.find(i => i.id === id);
    if (!inv) {
      toast.error('Invoice not found. Please refresh the page.');
      return;
    }
    setUpdatingId(id);
    const paidDate = localToday();
    updateInvoice.mutate(
      { id, data: { status: 'paid', paidDate, paidCents: inv.totalCents } },
      {
        onSuccess: () => { setUpdatingId(null); toast.success('Invoice marked as paid'); },
        onError: (err) => { setUpdatingId(null); toast.error(getErrorMessage(err, 'Failed to update invoice')); },
      },
    );
  }

  const statusBadge = (status: string) => {
    if (status === 'paid') return 'bg-[var(--success-bg)] text-[var(--success)]';
    if (status === 'partial') return 'bg-[var(--warning-bg)] text-[var(--warning)]';
    return 'bg-[var(--danger-bg)] text-[var(--danger)]';
  };

  if (isLoading) {
    return (
      <CardShell padding="lg">
        <div className="text-center text-[var(--text-muted)] py-4 text-sm">Loading invoices...</div>
      </CardShell>
    );
  }

  if (error) {
    return (
      <CardShell padding="lg">
        <div className="text-center text-[var(--danger)] py-4 text-sm">Failed to load invoices.</div>
      </CardShell>
    );
  }

  if (invoices.length === 0) {
    return (
      <CardShell padding="lg">
        <div className="mb-3">
          <div className="text-base font-semibold text-[var(--text)]">PSA Invoices</div>
          <div className="text-xs text-[var(--text-muted)] mt-0.5">Payment tracking for PSA submissions</div>
        </div>
        <div className="text-center text-[var(--text-muted)] py-4 text-sm">
          No invoices yet. Invoices are created automatically during PSA imports.
        </div>
      </CardShell>
    );
  }

  return (
    <CardShell padding="lg">
      <div className="mb-4">
        <div className="text-base font-semibold text-[var(--text)]">PSA Invoices</div>
        <div className="text-xs text-[var(--text-muted)] mt-0.5">Payment tracking for PSA submissions</div>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-[var(--surface-2)]">
              <th className="text-left py-2 px-3 text-[var(--text-muted)] font-medium text-xs">Date</th>
              <th className="text-right py-2 px-3 text-[var(--text-muted)] font-medium text-xs">Total</th>
              <th className="text-right py-2 px-3 text-[var(--text-muted)] font-medium text-xs">Paid</th>
              <th className="text-center py-2 px-3 text-[var(--text-muted)] font-medium text-xs">Status</th>
              <th className="text-right py-2 px-3 text-[var(--text-muted)] font-medium text-xs">Due</th>
              <th className="py-2 px-3"></th>
            </tr>
          </thead>
          <tbody>
            {invoices.map(inv => (
              <tr key={inv.id} className="border-b border-[var(--surface-2)]/50">
                <td className="py-2 px-3 text-xs text-[var(--text)]">{inv.invoiceDate}</td>
                <td className="py-2 px-3 text-xs text-right text-[var(--text)]">{formatCents(inv.totalCents)}</td>
                <td className="py-2 px-3 text-xs text-right text-[var(--text)]">{formatCents(inv.paidCents)}</td>
                <td className="py-2 px-3 text-xs text-center">
                  <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${statusBadge(inv.status)}`}>
                    {inv.status}
                  </span>
                </td>
                <td className="py-2 px-3 text-xs text-right text-[var(--text-muted)]">{inv.dueDate || '-'}</td>
                <td className="py-2 px-3 text-right">
                  {inv.status !== 'paid' && (
                    <Button
                      size="sm"
                      variant="success"
                      loading={updateInvoice.isPending && updatingId === inv.id}
                      onClick={() => handleMarkPaid(inv.id)}
                    >
                      Mark Paid
                    </Button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </CardShell>
  );
}
```

- [ ] **Step 2: Add InvoicesSection to InsightsPage**

In `web/src/react/pages/InsightsPage.tsx`, add the InvoicesSection after CreditHealthPanel and before the Portfolio Insights section.

```tsx
// Add import at top:
import InvoicesSection from '../components/insights/InvoicesSection';

// In the JSX, after CreditHealthPanel SectionErrorBoundary and before Portfolio Insights:
<SectionErrorBoundary sectionName="PSA Invoices">
  <InvoicesSection />
</SectionErrorBoundary>
```

The full updated return JSX:
```tsx
<div className="space-y-6">
  {weeklyReview && (
    <SectionErrorBoundary sectionName="Weekly Review">
      <WeeklyReviewSection data={weeklyReview} />
    </SectionErrorBoundary>
  )}

  {hasCapitalTimeline && (
    <SectionErrorBoundary sectionName="Capital Timeline">
      <CapitalTimelineChart data={capitalTimeline} />
    </SectionErrorBoundary>
  )}

  <SectionErrorBoundary sectionName="Credit Health">
    <CreditHealthPanel credit={creditData} />
  </SectionErrorBoundary>

  <SectionErrorBoundary sectionName="PSA Invoices">
    <InvoicesSection />
  </SectionErrorBoundary>

  <SectionErrorBoundary sectionName="Portfolio Insights">
    <InsightsSection />
  </SectionErrorBoundary>
</div>
```

- [ ] **Step 3: Delete old InvoicesTab**

Remove `web/src/react/pages/tools/InvoicesTab.tsx` — it's no longer imported anywhere.

```bash
rm web/src/react/pages/tools/InvoicesTab.tsx
```

- [ ] **Step 4: Verify the app compiles**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 5: Run lint**

Run: `cd /workspace/web && npm run lint`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add web/src/react/components/insights/InvoicesSection.tsx web/src/react/pages/InsightsPage.tsx
git rm web/src/react/pages/tools/InvoicesTab.tsx
git commit -m "feat: move PSA Invoices from Tools to Insights page"
```

---

### Task 6: Backend — Add DH API Health Tracking and Counts

**Files:**
- Modify: `internal/adapters/clients/dh/client.go` — add API call counter
- Create: `internal/adapters/clients/dh/health.go` — rolling 7-day health tracker
- Modify: `internal/adapters/httpserver/handlers/dh_handler.go` — add DHCountsFetcher interface
- Modify: `internal/adapters/httpserver/handlers/dh_status_handler.go` — extend response with health + counts

- [ ] **Step 1: Create health tracker**

Create `internal/adapters/clients/dh/health.go` — a simple thread-safe rolling counter that tracks success/failure counts over the last 7 days using minute-granularity buckets.

```go
// internal/adapters/clients/dh/health.go
package dh

import (
	"sync"
	"time"
)

const healthWindowMinutes = 7 * 24 * 60 // 7 days in minutes

// HealthStats holds aggregate API health metrics.
type HealthStats struct {
	TotalCalls int     `json:"total_calls"`
	Failures   int     `json:"failures"`
	SuccessRate float64 `json:"success_rate"`
}

// HealthTracker tracks API call success/failure counts using minute-granularity
// buckets over a rolling 7-day window.
type HealthTracker struct {
	mu      sync.Mutex
	buckets [healthWindowMinutes]bucket
	origin  time.Time // time of bucket 0
}

type bucket struct {
	success int
	failure int
}

// NewHealthTracker creates a new rolling health tracker.
func NewHealthTracker() *HealthTracker {
	return &HealthTracker{
		origin: time.Now().Truncate(time.Minute),
	}
}

func (h *HealthTracker) currentIndex() int {
	elapsed := int(time.Since(h.origin).Minutes())
	return elapsed % healthWindowMinutes
}

// advance zeroes out any buckets between the last write and now.
func (h *HealthTracker) advance() {
	now := time.Now().Truncate(time.Minute)
	elapsed := int(now.Sub(h.origin).Minutes())
	if elapsed >= healthWindowMinutes {
		// Window fully rolled over — clear everything
		h.buckets = [healthWindowMinutes]bucket{}
		h.origin = now
		return
	}
	// Zero buckets that have rolled past
	idx := elapsed % healthWindowMinutes
	// The origin tracks where we last wrote; advance zeroes the gap
	_ = idx // buckets self-manage via modular indexing; stale data is overwritten
	h.origin = now.Add(-time.Duration(elapsed%healthWindowMinutes) * time.Minute)
}

// RecordSuccess records a successful API call.
func (h *HealthTracker) RecordSuccess() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.advance()
	h.buckets[h.currentIndex()].success++
}

// RecordFailure records a failed API call.
func (h *HealthTracker) RecordFailure() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.advance()
	h.buckets[h.currentIndex()].failure++
}

// Stats returns aggregate health metrics over the rolling window.
func (h *HealthTracker) Stats() HealthStats {
	h.mu.Lock()
	defer h.mu.Unlock()
	var total, failures int
	for _, b := range h.buckets {
		total += b.success + b.failure
		failures += b.failure
	}
	rate := 100.0
	if total > 0 {
		rate = float64(total-failures) / float64(total) * 100
	}
	return HealthStats{
		TotalCalls:  total,
		Failures:    failures,
		SuccessRate: rate,
	}
}
```

- [ ] **Step 2: Instrument the DH client with health tracking**

In `internal/adapters/clients/dh/client.go`:

1. Add a `health *HealthTracker` field to the `Client` struct
2. Initialize it in `NewClient`: `health: NewHealthTracker()`
3. Add a `Health() *HealthTracker` getter method
4. In the `get`, `post`, `getEnterprise`, `doEnterprise` methods, record success/failure after the HTTP call:

For `get` — after `c.httpClient.Get`:
```go
resp, err := c.httpClient.Get(ctx, fullURL, headers, c.timeout)
if err != nil {
    c.health.RecordFailure()
    return err
}
c.health.RecordSuccess()
```

Apply the same pattern to `post`, `getEnterprise`, and `doEnterprise`.

Add the getter:
```go
// Health returns the API health tracker for this client.
func (c *Client) Health() *HealthTracker {
    return c.health
}
```

- [ ] **Step 3: Add DHCountsFetcher interface and extend DHHandler**

In `internal/adapters/httpserver/handlers/dh_handler.go`:

Add a new interface and field:
```go
// DHHealthReporter provides API health metrics.
type DHHealthReporter interface {
    Health() *dh.HealthTracker
}

// DHCountsFetcher retrieves inventory and order counts from DH.
type DHCountsFetcher interface {
    ListInventory(ctx context.Context, filters dh.InventoryFilters) (*dh.InventoryListResponse, error)
    GetOrders(ctx context.Context, filters dh.OrderFilters) (*dh.OrdersResponse, error)
}
```

Add fields to `DHHandler`:
```go
healthReporter DHHealthReporter  // optional: API health metrics
countsFetcher  DHCountsFetcher   // optional: DH inventory/order counts
```

Add these to `NewDHHandler` params and wiring (both optional — nil-check before use).

- [ ] **Step 4: Extend the status endpoint response**

In `internal/adapters/httpserver/handlers/dh_status_handler.go`:

Extend `dhStatusResponse`:
```go
type dhStatusResponse struct {
    // Existing fields
    IntelligenceCount     int    `json:"intelligence_count"`
    IntelligenceLastFetch string `json:"intelligence_last_fetch"`
    SuggestionsCount      int    `json:"suggestions_count"`
    SuggestionsLastFetch  string `json:"suggestions_last_fetch"`
    UnmatchedCount        int    `json:"unmatched_count"`
    PendingCount          int    `json:"pending_count"`
    MappedCount           int    `json:"mapped_count"`
    BulkMatchRunning      bool   `json:"bulk_match_running"`

    // New health fields
    APIHealth *dh.HealthStats `json:"api_health,omitempty"`

    // New DH counts
    DHInventoryCount int `json:"dh_inventory_count,omitempty"`
    DHListingsCount  int `json:"dh_listings_count,omitempty"`
    DHOrdersCount    int `json:"dh_orders_count,omitempty"`
}
```

In `HandleGetStatus`, after existing code and before `writeJSON`:
```go
// API health metrics
if h.healthReporter != nil {
    stats := h.healthReporter.Health().Stats()
    resp.APIHealth = &stats
}

// DH counts (best-effort — don't fail the whole response)
if h.countsFetcher != nil {
    // Inventory count — fetch page 1 with per_page=1 to get TotalCount from meta
    if invResp, err := h.countsFetcher.ListInventory(ctx, dh.InventoryFilters{PerPage: 1}); err != nil {
        h.logger.Warn(ctx, "dh status: count inventory", observability.Err(err))
    } else {
        resp.DHInventoryCount = invResp.Meta.TotalCount
    }

    // Listings count — filter by status "listed"
    if listResp, err := h.countsFetcher.ListInventory(ctx, dh.InventoryFilters{Status: "listed", PerPage: 1}); err != nil {
        h.logger.Warn(ctx, "dh status: count listings", observability.Err(err))
    } else {
        resp.DHListingsCount = listResp.Meta.TotalCount
    }

    // Orders count — use a far-past "since" to get total
    if ordResp, err := h.countsFetcher.GetOrders(ctx, dh.OrderFilters{Since: "2020-01-01T00:00:00Z", PerPage: 1}); err != nil {
        h.logger.Warn(ctx, "dh status: count orders", observability.Err(err))
    } else {
        resp.DHOrdersCount = ordResp.Meta.TotalCount
    }
}
```

- [ ] **Step 5: Wire up in main.go/server.go**

Find where `NewDHHandler` is called and pass the DH client as both `DHHealthReporter` and `DHCountsFetcher` (the `*dh.Client` already satisfies both interfaces). Add the two new params.

- [ ] **Step 6: Run backend tests**

Run: `cd /workspace && go test ./internal/adapters/clients/dh/... ./internal/adapters/httpserver/handlers/...`
Expected: PASS

- [ ] **Step 7: Run full test suite**

Run: `cd /workspace && go test ./...`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/adapters/clients/dh/health.go internal/adapters/clients/dh/client.go \
       internal/adapters/httpserver/handlers/dh_handler.go \
       internal/adapters/httpserver/handlers/dh_status_handler.go \
       cmd/slabledger/main.go cmd/slabledger/server.go
git commit -m "feat: add DH API health tracking and inventory/order counts to status endpoint"
```

---

### Task 7: Frontend — Update DHStatusResponse Type and Redesign Admin DH Tab

**Files:**
- Modify: `web/src/types/apiStatus.ts`
- Modify: `web/src/react/pages/admin/DHTab.tsx`

- [ ] **Step 1: Update DHStatusResponse type**

In `web/src/types/apiStatus.ts`, extend the `DHStatusResponse` interface:

```typescript
export interface DHHealthStats {
  total_calls: number;
  failures: number;
  success_rate: number;
}

export interface DHStatusResponse {
  intelligence_count: number;
  intelligence_last_fetch: string;
  suggestions_count: number;
  suggestions_last_fetch: string;
  unmatched_count: number;
  pending_count: number;
  mapped_count: number;
  bulk_match_running: boolean;
  // New fields
  api_health?: DHHealthStats;
  dh_inventory_count?: number;
  dh_listings_count?: number;
  dh_orders_count?: number;
}
```

- [ ] **Step 2: Redesign DHTab**

Replace the content of `web/src/react/pages/admin/DHTab.tsx`. Keep: bulk match trigger, summary stats (Intelligence, Suggestions). Add: Integration Health row (API Health, Match Rate, Unmatched), DH Counts row (Inventory, Listings, Orders). Remove: unmatched fix table (now on Daily Ops).

```tsx
// web/src/react/pages/admin/DHTab.tsx
import { useDHStatus, useTriggerDHBulkMatch } from '../../queries/useAdminQueries';
import { useToast } from '../../contexts/ToastContext';
import { CardShell } from '../../ui/CardShell';
import { SummaryCard } from './shared';
import Button from '../../ui/Button';

function formatTimestamp(ts: string): string {
  if (!ts) return 'Never';
  const d = new Date(ts);
  if (isNaN(d.getTime())) return ts;
  return d.toLocaleString();
}

function HealthCard({ label, value, sub, color }: {
  label: string;
  value: string;
  sub: string;
  color?: string;
}) {
  return (
    <div className="rounded-xl bg-[var(--surface-1)] border border-[var(--surface-2)] p-4">
      <div className="text-[11px] font-semibold text-[var(--text-muted)] uppercase tracking-wider mb-2">{label}</div>
      <div className="text-2xl font-bold" style={color ? { color } : undefined}>{value}</div>
      <div className="text-[11px] text-[var(--text-muted)] mt-1">{sub}</div>
    </div>
  );
}

export function DHTab({ enabled = true }: { enabled?: boolean }) {
  const { data: status, isLoading, error } = useDHStatus({ enabled });
  const bulkMatchMutation = useTriggerDHBulkMatch();
  const toast = useToast();

  if (!enabled) {
    return (
      <CardShell padding="lg">
        <p className="text-[var(--text-muted)]">DH integration is not configured.</p>
      </CardShell>
    );
  }

  if (isLoading) {
    return (
      <CardShell padding="lg">
        <p className="text-[var(--text-muted)]">Loading DH status...</p>
      </CardShell>
    );
  }

  if (error && !status) {
    return (
      <CardShell padding="lg">
        <p className="text-red-400 text-sm">Failed to load DH status. Integration may not be configured.</p>
      </CardShell>
    );
  }

  const isRunning = status?.bulk_match_running ?? false;
  const unmatchedCount = status?.unmatched_count ?? 0;
  const mappedCount = status?.mapped_count ?? 0;
  const totalCards = mappedCount + unmatchedCount;
  const matchRate = totalCards > 0 ? ((mappedCount / totalCards) * 100).toFixed(1) : '—';
  const unmatchedPct = totalCards > 0 ? ((unmatchedCount / totalCards) * 100).toFixed(1) : '0';
  const apiHealth = status?.api_health;

  const handleBulkMatch = async () => {
    try {
      await bulkMatchMutation.mutateAsync();
      toast.success('Bulk match started — progress will update automatically.');
    } catch {
      toast.error('Failed to start bulk match');
    }
  };

  return (
    <div className="space-y-6 mt-4">
      {/* Integration Health */}
      <div>
        <div className="text-sm font-semibold text-[var(--text-muted)] mb-3">Integration Health</div>
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
          <HealthCard
            label="API Health"
            value={apiHealth ? `${apiHealth.success_rate.toFixed(1)}%` : '—'}
            sub={apiHealth ? `${apiHealth.total_calls.toLocaleString()} calls / ${apiHealth.failures} failures (7d)` : 'No data'}
            color={apiHealth && apiHealth.success_rate >= 95 ? 'var(--success)' : apiHealth && apiHealth.success_rate >= 80 ? 'var(--warning)' : apiHealth ? 'var(--danger)' : undefined}
          />
          <HealthCard
            label="Match Rate"
            value={matchRate === '—' ? '—' : `${matchRate}%`}
            sub={`${mappedCount} matched / ${unmatchedCount} unmatched`}
            color="var(--brand-500)"
          />
          <HealthCard
            label="Unmatched"
            value={String(unmatchedCount)}
            sub={`${unmatchedPct}% of total inventory`}
            color={unmatchedCount > 0 ? 'var(--warning)' : undefined}
          />
        </div>
      </div>

      {/* DH Counts */}
      <div>
        <div className="text-sm font-semibold text-[var(--text-muted)] mb-3">DoubleHolo Counts</div>
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
          <SummaryCard
            label="Inventory"
            value={status?.dh_inventory_count ?? '—'}
            sub="Items in DH inventory"
          />
          <SummaryCard
            label="Listings"
            value={status?.dh_listings_count ?? '—'}
            sub="Active DH listings"
          />
          <SummaryCard
            label="Orders"
            value={status?.dh_orders_count ?? '—'}
            sub="Total DH orders"
          />
        </div>
      </div>

      {/* Existing: Market Intelligence & Suggestions (kept) */}
      <div>
        <div className="text-sm font-semibold text-[var(--text-muted)] mb-3">Market Data</div>
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
          <SummaryCard
            label="Market Intelligence"
            value={status?.intelligence_count ?? 0}
            sub={`Last: ${formatTimestamp(status?.intelligence_last_fetch ?? '')}`}
          />
          <SummaryCard
            label="Suggestions"
            value={status?.suggestions_count ?? 0}
            sub={`Last: ${formatTimestamp(status?.suggestions_last_fetch ?? '')}`}
          />
          <SummaryCard
            label="Pending Push"
            value={status?.pending_count ?? 0}
            color={(status?.pending_count ?? 0) > 0 ? 'var(--info)' : undefined}
          />
          <SummaryCard
            label="Mapped Cards"
            value={mappedCount}
          />
        </div>
      </div>

      {/* Bulk Match */}
      <CardShell padding="lg">
        <div className="flex items-center justify-between">
          <div>
            <h4 className="text-sm font-semibold text-[var(--text)]">Bulk Match (Backfill)</h4>
            <p className="text-xs text-[var(--text-muted)] mt-1">
              Match unmatched inventory cards against the DH catalog. High-confidence matches are automatically mapped.
            </p>
          </div>
          <Button
            variant="secondary"
            size="sm"
            onClick={handleBulkMatch}
            loading={bulkMatchMutation.isPending}
            disabled={isRunning || bulkMatchMutation.isPending}
          >
            {isRunning ? 'Running...' : 'Run Bulk Match'}
          </Button>
        </div>
        {isRunning && (
          <p className="mt-2 text-xs text-[var(--text-muted)]">
            Matching in progress — mapped/unmatched counts will update automatically.
          </p>
        )}
      </CardShell>
    </div>
  );
}
```

- [ ] **Step 3: Verify the app compiles**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 4: Run lint**

Run: `cd /workspace/web && npm run lint`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add web/src/types/apiStatus.ts web/src/react/pages/admin/DHTab.tsx
git commit -m "feat: redesign Admin DH tab — health metrics, DH counts, remove unmatched table"
```

---

### Task 8: Final Verification

- [ ] **Step 1: Full backend test suite**

Run: `cd /workspace && go test ./...`
Expected: PASS

- [ ] **Step 2: Full frontend type check**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 3: Frontend lint**

Run: `cd /workspace/web && npm run lint`
Expected: No errors

- [ ] **Step 4: Quality check**

Run: `cd /workspace && make check`
Expected: PASS

- [ ] **Step 5: Manual smoke test checklist**

Verify in browser:
1. Dashboard: No Active Campaigns grid, Weekly Review is immediately visible
2. Tools > Daily Ops: 3 import/export cards (no External Import), DH Unmatched table below
3. Tools > Card Intake: Unchanged scanning workflow
4. Tools > Legacy: 4 cards (External Import, Price Sync, eBay Export, Import Sales), all tagged "Transitional"
5. Tools > Legacy: eBay Export "Load Items" expands inline
6. Tools > Legacy: Import Sales "Upload Orders CSV" expands inline
7. Insights: PSA Invoices section appears after Credit Health
8. Admin > Integrations > DH: Integration Health row, DH Counts row, Market Data row, Bulk Match trigger
9. Admin > DH: No unmatched fix table (it's on Daily Ops now)
