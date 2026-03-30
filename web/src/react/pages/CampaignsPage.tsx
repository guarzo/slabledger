/**
 * Campaigns Page
 *
 * Lists all campaigns with P&L summary info and portfolio summary strip.
 */
import { useState, useMemo, useEffect } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useQueries, useQueryClient } from '@tanstack/react-query';
import { api } from '../../js/api';
import type { Campaign, CampaignPNL, CreateCampaignInput, Phase } from '../../types/campaigns';
import { queryKeys } from '../queries/queryKeys';
import PokeballLoader from '../PokeballLoader';
import { formatCents, formatPct, getErrorMessage } from '../utils/formatters';
import { useToast } from '../contexts/ToastContext';
import { useForm } from '../hooks/useForm';
import { defaultCampaignInput } from '../utils/campaignConstants';
import { Button, SectionErrorBoundary } from '../ui';
import { useCampaigns, useCreateCampaign, usePortfolioHealth } from '../queries/useCampaignQueries';
import PortfolioSummary from './campaigns/PortfolioSummary';
import CampaignsTab from './campaigns/CampaignsTab';

const phaseOrder: Record<Phase, number> = { active: 0, pending: 1, closed: 2 };

const phaseGradients: Record<Phase, string> = {
  active: 'linear-gradient(135deg, #059669, #10b981)',
  pending: 'linear-gradient(135deg, #f59e0b, #fbbf24)',
  closed: 'linear-gradient(135deg, #374151, #4b5563)',
};

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

function formatPriceRange(raw: string): string {
  if (!raw) return '';
  const parts = raw.split(/\s*[-\u2013\u2014]\s*/);
  return parts.map(p => {
    const n = p.replace(/[^0-9.]/g, '');
    return n ? `$${n}` : p;
  }).join(' to ');
}

// Parsed campaign: only fields explicitly present in the text are set.
// inclusionList + exclusionMode are always included — absent line = cleared.
type ParsedCampaign = Partial<CreateCampaignInput> & { name: string; inclusionList: string; exclusionMode: boolean };

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
    // Conditionally-emitted string filters (year, grade, price, clConfidence,
    // inclusionList) default to '' so an absent line clears the filter —
    // buildExportText only emits these when non-empty.
    const input: ParsedCampaign = {
      name: headerMatch[1].trim(),
      yearRange: '',
      gradeRange: '',
      priceRange: '',
      clConfidence: '',
      inclusionList: '',
      exclusionMode: false,
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
        case 'INCLUSION':
          input.inclusionList = val;
          input.exclusionMode = false;
          break;
        case 'EXCLUSION':
          input.inclusionList = val;
          input.exclusionMode = true;
          break;
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
    if (c.inclusionList) {
      const label = c.exclusionMode ? 'Exclusion' : 'Inclusion';
      lines.push(`${label}: ${c.inclusionList}`);
    }
    return lines.join('\n');
  }).join('\n\n');
}

export default function CampaignsPage() {
  const [showCreate, setShowCreate] = useState(false);
  const [activeOnly, setActiveOnly] = useState(false);
  const toast = useToast();
  const [searchParams, setSearchParams] = useSearchParams();

  const queryClient = useQueryClient();
  const { data: allCampaigns = [], isLoading } = useCampaigns(false);
  const createMutation = useCreateCampaign();

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

  // Pre-fill form from URL search params (e.g. from suggestions page)
  useEffect(() => {
    if (searchParams.get('create') !== '1') return;
    const name = searchParams.get('name');
    const inclusionList = searchParams.get('inclusionList');
    const gradeRange = searchParams.get('gradeRange');
    const yearRange = searchParams.get('yearRange');
    const priceRange = searchParams.get('priceRange');
    const buyTerms = searchParams.get('buyTermsCLPct');
    const spendCap = searchParams.get('dailySpendCapCents');

    const warnings: string[] = [];

    if (name) form.handleChange('name', name);
    if (inclusionList) form.handleChange('inclusionList', inclusionList);
    if (gradeRange) form.handleChange('gradeRange', gradeRange);
    if (yearRange) form.handleChange('yearRange', yearRange);
    if (priceRange) form.handleChange('priceRange', priceRange);

    if (buyTerms) {
      const val = parseFloat(buyTerms);
      if (isNaN(val) || val <= 0 || val > 1) {
        warnings.push(`Invalid buy terms "${buyTerms}" \u2014 ignored`);
      } else {
        form.handleChange('buyTermsCLPct', val);
      }
    }
    if (spendCap) {
      const val = parseInt(spendCap, 10);
      if (isNaN(val) || val <= 0) {
        warnings.push(`Invalid spend cap "${spendCap}" \u2014 ignored`);
      } else {
        form.handleChange('dailySpendCapCents', val);
      }
    }

    if (warnings.length > 0) {
      toast.warning(warnings.join('. '));
    }

    setShowCreate(true);
    setSearchParams({}, { replace: true });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const pnlQueries = useQueries({
    queries: allCampaigns.map(c => ({
      queryKey: queryKeys.campaigns.pnl(c.id),
      queryFn: () => api.getCampaignPNL(c.id),
      enabled: !!c.id,
    })),
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

  const campaigns = useMemo(() => {
    const filtered = activeOnly
      ? allCampaigns.filter(c => c.phase === 'active')
      : allCampaigns;
    return sortCampaigns(filtered);
  }, [allCampaigns, activeOnly]);

  const { data: healthData } = usePortfolioHealth();
  const healthMap = useMemo(() => {
    const map: Record<string, string> = {};
    healthData?.campaigns?.forEach(ch => { map[ch.campaignId] = ch.healthStatus; });
    return map;
  }, [healthData]);

  const activeCampaignCount = allCampaigns.filter(c => c.phase === 'active').length;

  if (isLoading) {
    return (
      <div className="flex items-center justify-center min-h-[50vh]">
        <PokeballLoader />
      </div>
    );
  }

  return (
    <div className="max-w-6xl mx-auto px-4">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-[22px] font-bold text-[var(--text)] tracking-tight">Campaigns</h1>
        <div className="flex items-center gap-3">
          <label className="flex items-center gap-2 text-sm text-[var(--text-muted)] cursor-pointer">
            <input type="checkbox" checked={activeOnly} onChange={e => setActiveOnly(e.target.checked)}
              className="rounded" />
            Active only
          </label>
          <Button
            size="icon"
            variant="secondary"
            title="Export active campaigns to clipboard"
            aria-label="Export active campaigns to clipboard"
            onClick={async () => {
              const text = buildExportText(sortCampaigns(allCampaigns));
              if (!text) { toast.error('No active campaigns to export'); return; }
              try {
                await navigator.clipboard.writeText(text);
                toast.success('Active campaigns copied to clipboard');
              } catch { toast.error('Failed to copy to clipboard'); }
            }}
          >
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5" aria-hidden="true" focusable="false">
              <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
              <path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1" />
            </svg>
          </Button>
          <Button
            size="icon"
            variant="secondary"
            title="Import campaigns from clipboard"
            aria-label="Import campaigns from clipboard"
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
                    const existing = allCampaigns.find(c => c.name.toLowerCase() === input.name.toLowerCase());
                    if (existing) {
                      // Only overlay fields that were explicitly in the import text;
                      // omitted fields (e.g. PSA Sourcing Fee) keep their current values.
                      // Strip server-owned fields so only mutable data is sent.
                      const { id: _id, createdAt: _ca, updatedAt: _ua, expectedFillRate: _efr, ...base } = existing;
                      await api.updateCampaign(existing.id, { ...base, ...input });
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
                if (errors.length > 0) toast.error(`Failed: ${errors.join('; ')}`);
              } catch {
                toast.error('Failed to read clipboard. Check browser permissions.');
              }
            }}
          >
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5" aria-hidden="true" focusable="false">
              <path d="M16 4h2a2 2 0 012 2v14a2 2 0 01-2 2H6a2 2 0 01-2-2V6a2 2 0 012-2h2" />
              <rect x="8" y="2" width="8" height="4" rx="1" ry="1" />
              <path d="M9 14l2 2 4-4" />
            </svg>
          </Button>
          <Button
            size="icon"
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
        <PortfolioSummary campaignCount={activeCampaignCount} pnlMap={pnlMap} />
      )}

      <SectionErrorBoundary sectionName="Campaigns">
        <CampaignsTab
          campaigns={campaigns}
          pnlMap={pnlMap}
          healthMap={healthMap}
          showCreate={showCreate}
          form={form}
          createMutation={createMutation}
          activeOnly={activeOnly}
          onToggleCreate={() => setShowCreate(true)}
          phaseGradients={phaseGradients}
        />
      </SectionErrorBoundary>
    </div>
  );
}
