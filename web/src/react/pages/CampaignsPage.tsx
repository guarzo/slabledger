/**
 * Campaigns Page
 *
 * Lists all campaigns with P&L summary info and portfolio summary strip.
 */
import { useState, useMemo } from 'react';
import { useQueries, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../js/api';
import { reportError } from '../../js/errors';
import type { Campaign, CampaignPNL, CreateCampaignInput, Phase, PSAPushRow } from '../../types/campaigns';
import { queryKeys } from '../queries/queryKeys';
import PokeballLoader from '../PokeballLoader';
import { formatCents, formatPct, formatPriceRange, getErrorMessage } from '../utils/formatters';
import { useToast } from '../contexts/ToastContext';
import { useForm } from '../hooks/useForm';
import { defaultCampaignInput } from '../utils/campaignConstants';
import { Button, SectionErrorBoundary } from '../ui';
import { useCampaigns, useCreateCampaign, useUpdateCampaign, usePortfolioHealth, campaignPNLQueryOptions } from '../queries/useCampaignQueries';
import CampaignsPortfolioHero from './campaigns/CampaignsPortfolioHero';
import CampaignsTab from './campaigns/CampaignsTab';
import InvoicesSection from '../components/insights/InvoicesSection';
import { toFormValues, type EditCampaignFormValues } from '../utils/campaignFormValues';

const phaseOrder: Record<Phase, number> = { active: 0, pending: 1, closed: 2 };

function sortCampaigns(campaigns: Campaign[]): Campaign[] {
  return [...campaigns].sort((a, b) => {
    const orderA = phaseOrder[a.phase] ?? 2;
    const orderB = phaseOrder[b.phase] ?? 2;
    if (orderA !== orderB) return orderA - orderB;
    return new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime();
  });
}

function validateCampaignForm(values: CreateCampaignInput) {
  const errors: Partial<Record<keyof CreateCampaignInput, string>> = {};
  if (!values.name || values.name.trim() === '') {
    errors.name = 'Name is required';
  }
  return errors;
}


// Parsed campaign: only fields explicitly present in the text are set.
//
// Targeting (targetLanguages, subjectFilterMode, subjects, deniedSpecs) is
// deliberately NOT part of this bulk paste format. Subjects and denied specs
// carry portal-issued ids that must never be re-derived from a name (see the
// design doc's "ids are copied verbatim, never re-resolved" rule) — a text
// round-trip through paste would reset those ids to 0 and corrupt targeting on
// the next push for every campaign already linked to the portal. Targeting is
// set at campaign creation, edited through the per-row Edit form (which carries
// SubjectRef ids through untouched), or replaced by the harvester's baseline
// pull. This paste format stays scoped to scalar economics/range fields, which
// round-trip safely.
type ParsedCampaign = Partial<CreateCampaignInput> & { name: string };

function parseExportText(text: string): ParsedCampaign[] {
  // Split at campaign boundaries (before "Campaign N — ..." lines) instead of
  // blank lines, so the parser handles both compact and spaced-out clipboard formats.
  // Allow optional leading whitespace (^\s*) so indented clipboard text still splits.
  const blocks = text.trim().split(/(?=^\s*Campaign\s+\d+\s*[-\u2013\u2014])/m);
  const campaigns: ParsedCampaign[] = [];

  for (const block of blocks) {
    const lines = block.split('\n').map(l => l.trim()).filter(Boolean);
    if (lines.length === 0) continue;

    // First line must match "Campaign N — Name" (allow leading whitespace)
    const headerMatch = lines[0].match(/^\s*Campaign\s+\d+\s*[-\u2013\u2014]\s*(.+)$/);
    if (!headerMatch) continue;

    // Only set fields that actually appear in the text. When updating an
    // existing campaign, omitted fields (e.g. PSA Sourcing Fee, eBay Fee)
    // keep their current values instead of being reset to defaults.
    // Conditionally-emitted string filters (year, grade, price, clConfidence)
    // default to '' so an absent line clears the filter — buildExportText
    // only emits these when non-empty.
    const input: ParsedCampaign = {
      name: headerMatch[1].trim(),
      yearRange: '',
      gradeRange: '',
      priceRange: '',
      clConfidence: '',
    };

    for (let i = 1; i < lines.length; i++) {
      const line = lines[i];
      const colonIdx = line.indexOf(':');
      if (colonIdx === -1) continue;

      const key = line.slice(0, colonIdx).trim().toUpperCase();
      const val = line.slice(colonIdx + 1).trim();

      switch (key) {
        case 'SPORT':
          input.sport = val;
          break;
        case 'YEAR':
          input.yearRange = val;
          break;
        case 'PSA GRADE':
          // Normalize single value "7" → "7-7" for backend range validation
          input.gradeRange = /^\d+$/.test(val.trim()) ? `${val.trim()}-${val.trim()}` : val;
          break;
        case 'PRICE': {
          // Reverse formatPriceRange: "$10 to $50" → "10-50", strip commas
          const raw = val.replace(/[$,]/g, '').replace(/\s+to\s+/gi, '-');
          input.priceRange = raw;
          break;
        }
        case 'CL CONFIDENCE':
          input.clConfidence = val;
          break;
        case 'BUY TERMS': {
          // "78.0%" → 0.78
          const pct = parseFloat(val.replace('%', ''));
          if (!isNaN(pct)) input.buyTermsCLPct = pct / 100;
          break;
        }
        case 'DAILY SPEND': {
          // "$500.00" → 50000 cents
          const dollars = parseFloat(val.replace(/[$,]/g, ''));
          if (!isNaN(dollars)) input.dailySpendCapCents = Math.round(dollars * 100);
          break;
        }
        case 'PSA SOURCING FEE': {
          const dollars = parseFloat(val.replace(/[$,]/g, ''));
          if (!isNaN(dollars)) input.psaSourcingFeeCents = Math.round(dollars * 100);
          break;
        }
        case 'EBAY FEE': {
          const pct = parseFloat(val.replace('%', ''));
          if (!isNaN(pct)) input.ebayFeePct = pct / 100;
          break;
        }
        // 'INCLUSION'/'EXCLUSION' lines from the pre-spec-list format are no
        // longer recognized — see the comment on ParsedCampaign above.
      }
    }

    if (input.name) campaigns.push(input);
  }

  return campaigns;
}

function buildExportText(campaigns: Campaign[]): string {
  const active = campaigns.filter(c => c.phase === 'active');
  if (active.length === 0) return '';

  return active.map((c, i) => {
    const lines: string[] = [];
    lines.push(`Campaign ${i + 1} \u2014 ${c.name}`);
    lines.push(`SPORT: ${c.sport}`);
    if (c.yearRange) lines.push(`YEAR: ${c.yearRange}`);
    if (c.gradeRange) lines.push(`PSA GRADE: ${c.gradeRange}`);
    if (c.priceRange) lines.push(`Price: ${formatPriceRange(c.priceRange)}`);
    if (c.clConfidence) lines.push(`CL CONFIDENCE: ${c.clConfidence}`);
    lines.push(`BUY TERMS: ${formatPct(c.buyTermsCLPct)}`);
    lines.push(`Daily Spend: ${formatCents(c.dailySpendCapCents)}`);
    lines.push(`PSA Sourcing Fee: ${formatCents(c.psaSourcingFeeCents)}`);
    lines.push(`eBay Fee: ${formatPct(c.ebayFeePct)}`);
    return lines.join('\n');
  }).join('\n\n');
}

export default function CampaignsPage() {
  const [showCreate, setShowCreate] = useState(false);
  // The campaign under edit, plus the updatedAt captured when the form opened.
  // That timestamp is the optimistic-concurrency token: the psa-harvest
  // baseline pull writes the same targeting fields from a separate process,
  // and a stale form would put -1 placeholders back over reconciled portal ids.
  const [editing, setEditing] = useState<{ id: string; updatedAt: string } | null>(null);
  const toast = useToast();

  const queryClient = useQueryClient();
  const { data: allCampaigns = [], isLoading } = useCampaigns(false);
  const createMutation = useCreateCampaign();
  const updateMutation = useUpdateCampaign();

  const form = useForm<CreateCampaignInput>({
    initialValues: { ...defaultCampaignInput },
    validate: validateCampaignForm,
    onSubmit: async (values) => {
      try {
        await createMutation.mutateAsync(values);
        setShowCreate(false);
        form.reset();
        toast.success('Campaign created');
      } catch (err) {
        toast.error(getErrorMessage(err, 'Failed to create campaign'));
      }
    },
  });

  const editForm = useForm<EditCampaignFormValues>({
    // expectedFillRate is only meaningful once handleEdit seeds real data via
    // toFormValues; this placeholder is never shown (the edit form only
    // mounts once `editing` is set, immediately after reset()).
    initialValues: { ...defaultCampaignInput, expectedFillRate: 0 },
    validate: validateCampaignForm,
    onSubmit: async (values) => {
      if (!editing) return;

      // Re-read over the network, not from the React Query cache: useCampaigns
      // holds data fresh for CAMPAIGN_STALE_TIME (30s), and the racing writer
      // is the psa-harvest process, whose write the cache cannot observe.
      let fresh: Campaign;
      try {
        fresh = await api.getCampaign(editing.id);
      } catch (err) {
        // Fail closed. Saving anyway would reintroduce exactly the race this
        // check exists to close. The message is fixed rather than routed
        // through getErrorMessage: that helper surfaces the raw Error.message
        // when present (e.g. "network down"), which would bury the actionable
        // "nothing was saved" guidance the operator needs here.
        reportError('Campaign edit staleness check', err);
        toast.error('Could not confirm the campaign is unchanged — nothing was saved');
        return;
      }

      if (fresh.updatedAt !== editing.updatedAt) {
        // The toast's "start from current data" advice is only true if the
        // cache is actually refreshed — otherwise re-opening Edit re-reads
        // the same stale row from the query cache and fails this check again.
        await queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.all });
        toast.error(
          'This campaign changed since you opened the form — most likely the harvester baseline pull. ' +
          'Nothing was saved. Close and re-open Edit to start from current data.',
        );
        return;
      }

      try {
        // Full-row PUT: HandleUpdateCampaign decodes a whole inventory.Campaign
        // and the UPDATE sets every column, so an omitted field is written as
        // its zero value. Spreading `fresh` first is what keeps
        // psaCampaignRequestId and expectedFillRate intact. The bulk-paste path
        // below now spreads a freshly-read row the same way, behind the same
        // updatedAt guard.
        await updateMutation.mutateAsync({ id: editing.id, data: { ...fresh, ...values } });
        setEditing(null);
        toast.success(
          fresh.psaCampaignRequestId
            ? 'Campaign updated — open PSA on the row to publish the change'
            : 'Campaign updated',
        );
      } catch (err) {
        toast.error(getErrorMessage(err, 'Failed to update campaign'));
      }
    },
  });

  function handleEdit(c: Campaign) {
    setShowCreate(false);
    setEditing({ id: c.id, updatedAt: c.updatedAt });
    editForm.reset(toFormValues(c));
  }

  function handleCancelEdit() {
    setEditing(null);
  }

  const editingCampaign = editing ? allCampaigns.find(c => c.id === editing.id) ?? null : null;

  const pnlQueries = useQueries({
    queries: allCampaigns.map(c => campaignPNLQueryOptions(c.id)),
  });

  const pnlMap = useMemo(() => {
    const map: Record<string, CampaignPNL> = {};
    pnlQueries.forEach((q, i) => {
      if (q.data && allCampaigns[i]) {
        map[allCampaigns[i].id] = q.data as CampaignPNL;
      }
    });
    return map;
  }, [pnlQueries, allCampaigns]);

  const campaigns = useMemo(() => sortCampaigns(allCampaigns), [allCampaigns]);

  const activeCampaignCount = useMemo(
    () => allCampaigns.filter(c => c.phase === 'active').length,
    [allCampaigns],
  );

  const { data: healthData } = usePortfolioHealth();
  const healthMap = useMemo(() => {
    const map: Record<string, string> = {};
    healthData?.campaigns?.forEach(ch => { map[ch.campaignId] = ch.healthStatus; });
    return map;
  }, [healthData]);

  const { data: psaPushesData } = useQuery({
    queryKey: queryKeys.psaPushes.list,
    queryFn: () => api.listPSAPushes(),
  });
  const psaPushMap = useMemo(() => {
    const map: Record<string, PSAPushRow> = {};
    psaPushesData?.pushes?.forEach(p => { map[p.campaignId] = p; });
    return map;
  }, [psaPushesData]);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center min-h-[50vh]">
        <PokeballLoader />
      </div>
    );
  }

  return (
    <div className="max-w-6xl mx-auto px-4">
      <div className="flex items-center justify-between mb-4">
        <h1 className="page-title">Campaigns</h1>
        <div className="flex items-center gap-3">
          <Button
            size="sm"
            variant="secondary"
            title="Open PSA Buyer Campaign Manager"
            aria-label="Open PSA Buyer Campaign Manager"
            icon={
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4" aria-hidden="true" focusable="false">
                <path d="M18 13v6a2 2 0 01-2 2H5a2 2 0 01-2-2V8a2 2 0 012-2h6" />
                <polyline points="15 3 21 3 21 9" />
                <line x1="10" y1="14" x2="21" y2="3" />
              </svg>
            }
            onClick={() => window.open('https://www.psacard.com/buyercampaignmanager/', '_blank', 'noopener,noreferrer')}
          >
            PSA
          </Button>
          <Button
            size="sm"
            variant="secondary"
            title="Export active campaigns to clipboard"
            aria-label="Export active campaigns to clipboard"
            icon={
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4" aria-hidden="true" focusable="false">
                <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
                <path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1" />
              </svg>
            }
            onClick={async () => {
              const text = buildExportText(sortCampaigns(allCampaigns));
              if (!text) { toast.error('No active campaigns to export'); return; }
              try {
                await navigator.clipboard.writeText(text);
                toast.success('Active campaigns copied to clipboard');
              } catch { toast.error('Failed to copy to clipboard'); }
            }}
          >
            Copy
          </Button>
          <Button
            size="sm"
            variant="secondary"
            title="Import campaigns from clipboard"
            aria-label="Import campaigns from clipboard"
            icon={
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4" aria-hidden="true" focusable="false">
                <path d="M16 4h2a2 2 0 012 2v14a2 2 0 01-2 2H6a2 2 0 01-2-2V6a2 2 0 012-2h2" />
                <rect x="8" y="2" width="8" height="4" rx="1" ry="1" />
                <path d="M9 14l2 2 4-4" />
              </svg>
            }
            onClick={async () => {
              try {
                const text = await navigator.clipboard.readText();
                const parsed = parseExportText(text);
                if (parsed.length === 0) {
                  toast.error('No campaigns found in clipboard. Copy the export format first.');
                  return;
                }
                let created = 0;
                let updated = 0;
                const errors: string[] = [];
                // Process sequentially — concurrent mutateAsync calls on the
                // same mutation overwrite internal state, losing all but one.
                for (const input of parsed) {
                  try {
                    const cached = allCampaigns.find(c => c.name.toLowerCase() === input.name.toLowerCase());
                    if (cached) {
                      // Re-read over the network before the full-row PUT, the same
                      // guard the edit form uses: allCampaigns is React Query data
                      // held fresh for CAMPAIGN_STALE_TIME, and the racing writer is
                      // the psa-harvest baseline pull, whose write the cache cannot
                      // observe. Without this, a paste spreads the cached row and
                      // writes its stale targeting ids back over a newer baseline.
                      let fresh: Campaign;
                      try {
                        fresh = await api.getCampaign(cached.id);
                      } catch (err) {
                        // Fail closed, but per campaign: skip this one and keep
                        // importing the rest of the block.
                        reportError('Campaign paste staleness check', err);
                        errors.push(`${input.name}: could not confirm the campaign is unchanged — not updated`);
                        continue;
                      }
                      if (fresh.updatedAt !== cached.updatedAt) {
                        // No invalidate here — the unconditional one after this loop
                        // already refreshes the list, so a re-paste sees current data.
                        errors.push(`${input.name}: changed since the campaign list loaded (most likely the harvester baseline pull) — not updated`);
                        continue;
                      }
                      // Only overlay fields that were explicitly in the import text;
                      // omitted fields (e.g. PSA Sourcing Fee) keep their current
                      // values. Everything else rides along verbatim because this is
                      // a full-row PUT: HandleUpdateCampaign decodes a whole
                      // inventory.Campaign and the UPDATE sets every column, so an
                      // omitted field is written as its zero value. That is why
                      // expectedFillRate is no longer stripped — it is operator-owned
                      // (campaign_store.go binds c.ExpectedFillRate verbatim and
                      // nothing derives it), so stripping it zeroed the column on
                      // every paste update. id/createdAt/updatedAt are the genuinely
                      // server-owned ones: id comes from the URL, created_at is not in
                      // the UPDATE, and the service stamps UpdatedAt itself.
                      const { id: _id, createdAt: _ca, updatedAt: _ua, ...base } = fresh;
                      await api.updateCampaign(cached.id, { ...base, ...input });
                      updated++;
                    } else {
                      // New campaigns get defaults for any fields not in the text.
                      await api.createCampaign({ ...defaultCampaignInput, ...input, phase: 'active' });
                      created++;
                    }
                  } catch (err) {
                    errors.push(`${input.name}: ${getErrorMessage(err, 'failed')}`);
                  }
                }
                await queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.all });
                const parts: string[] = [];
                if (created > 0) parts.push(`${created} created`);
                if (updated > 0) parts.push(`${updated} updated`);
                if (parts.length > 0) toast.success(`Imported: ${parts.join(', ')}`);
                if (errors.length > 0) {
                  // Full details to the central error handler for the operator;
                  // concise summary in toast.
                  reportError('Campaign paste import errors', errors.join('\n'));
                  const preview = errors.slice(0, 3).join('\n');
                  const more = errors.length > 3 ? `\n…and ${errors.length - 3} more (see console)` : '';
                  toast.error(`${errors.length} import error${errors.length !== 1 ? 's' : ''}:\n${preview}${more}`);
                }
              } catch {
                toast.error('Failed to read clipboard. Check browser permissions.');
              }
            }}
          >
            Paste
          </Button>
          <Button
            size="sm"
            title={showCreate ? 'Cancel' : 'New campaign'}
            variant={showCreate ? 'danger' : 'primary'}
            onClick={() => {
              setShowCreate(!showCreate);
            }}
          >
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"
              className={`w-5 h-5 transition-transform duration-200 ${showCreate ? 'rotate-45' : ''}`} aria-hidden="true" focusable="false">
              <line x1="12" y1="5" x2="12" y2="19" />
              <line x1="5" y1="12" x2="19" y2="12" />
            </svg>
          </Button>
        </div>
      </div>

      {allCampaigns.length > 0 && (
        <CampaignsPortfolioHero campaignCount={activeCampaignCount} pnlMap={pnlMap} />
      )}

      <SectionErrorBoundary sectionName="Campaigns">
        <CampaignsTab
          campaigns={campaigns}
          pnlMap={pnlMap}
          healthMap={healthMap}
          psaPushMap={psaPushMap}
          showCreate={showCreate}
          form={form}
          createMutation={createMutation}
          onToggleCreate={() => setShowCreate(true)}
          editingCampaign={editingCampaign}
          editForm={editForm}
          updateMutation={updateMutation}
          onEdit={handleEdit}
          onCancelEdit={handleCancelEdit}
        />
      </SectionErrorBoundary>

      <SectionErrorBoundary sectionName="Invoices">
        <InvoicesSection />
      </SectionErrorBoundary>
    </div>
  );
}
