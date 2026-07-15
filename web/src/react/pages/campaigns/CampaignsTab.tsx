import { Fragment, useMemo, useState } from 'react';
import type { Campaign, CampaignPNL, CreateCampaignInput, Phase, PSAPushRow } from '../../../types/campaigns';
import { formatCents, formatDollarsWhole, formatPct, formatPriceRange } from '../../utils/formatters';
import { EmptyState, Button, StatusPill, type StatusTone } from '../../ui';
import CardShell from '../../ui/CardShell';
import CampaignFormFields from '../../ui/CampaignFormFields';
import PSAPublishModal from '../campaign-detail/PSAPublishModal';
import type { UseFormReturn } from '../../hooks/useForm';
import { phaseHexColors } from '../../utils/campaignConstants';

const PHASE_TONES: Record<Phase, StatusTone> = {
  active: 'success',
  pending: 'warning',
  closed: 'neutral',
};

const PHASE_LABELS: Record<Phase, string> = {
  active: 'Active',
  pending: 'Pending',
  closed: 'Closed',
};

/** Phase order on the page. Matches the parent's sortCampaigns(). */
const PHASE_ORDER: Phase[] = ['active', 'pending', 'closed'];

/** Visual state of the PSA button's push indicator. `pushed` and absent rows
    render no indicator — the queue entry is resolved. */
type PushIndicatorState = 'pending' | 'inflight' | 'failed';

const PUSH_INDICATOR: Record<PushIndicatorState, { color: string; label: string }> = {
  pending: { color: 'var(--warning)', label: 'approval pending' },
  inflight: { color: 'var(--info)', label: 'push in flight' },
  failed: { color: 'var(--danger)', label: 'last push failed' },
};

function pushIndicatorState(push: PSAPushRow | undefined): PushIndicatorState | null {
  if (!push) return null;
  switch (push.status) {
    case 'pending': return 'pending';
    case 'approved':
    case 'pushing': return 'inflight';
    case 'failed': return 'failed';
    default: return null; // pushed = resolved
  }
}

function PhaseBadge({ phase }: { phase: Phase }) {
  return (
    <StatusPill tone={PHASE_TONES[phase]} size="xs" className="uppercase">
      {phase}
    </StatusPill>
  );
}

function FilterSummary({ c }: { c: Campaign }) {
  const parts: string[] = [c.sport];
  if (c.yearRange) parts.push(c.yearRange);
  if (c.gradeRange) parts.push(`PSA ${c.gradeRange}`);
  if (c.priceRange) parts.push(formatPriceRange(c.priceRange));
  return (
    <span className="text-xs text-[var(--text-muted)] truncate">
      {parts.join(' / ')}
    </span>
  );
}

export default function CampaignsTab({
  campaigns,
  pnlMap,
  healthMap,
  psaPushMap,
  showCreate,
  form,
  createMutation,
  onToggleCreate,
}: {
  campaigns: Campaign[];
  pnlMap: Record<string, CampaignPNL>;
  healthMap: Record<string, string>;
  psaPushMap: Record<string, PSAPushRow>;
  showCreate: boolean;
  form: UseFormReturn<CreateCampaignInput>;
  createMutation: { isPending: boolean };
  onToggleCreate: () => void;
}) {
  const [psaModalCampaignId, setPsaModalCampaignId] = useState<string | null>(null);
  const psaModalCampaign = campaigns.find(c => c.id === psaModalCampaignId) ?? null;

  // Compute the indices in the (already-sorted) campaign list where the phase
  // changes. We render a section eyebrow before the first row of each phase
  // so the operator gets a quick "10 active · 2 pending" structural read
  // without losing the existing column alignment.
  const phaseSections = useMemo(() => {
    const sections: Array<{ phase: Phase; startIdx: number; count: number }> = [];
    for (const phase of PHASE_ORDER) {
      const startIdx = campaigns.findIndex(c => c.phase === phase);
      if (startIdx === -1) continue;
      const count = campaigns.filter(c => c.phase === phase).length;
      sections.push({ phase, startIdx, count });
    }
    return sections;
  }, [campaigns]);

  return (
    <>
      {showCreate && (
        <div className="mb-6">
          <CardShell variant="elevated" padding="lg">
            <form onSubmit={form.handleSubmit}>
              <div className="mb-5">
                <h2 className="text-lg font-semibold text-[var(--text)]">Create Campaign</h2>
                <p className="text-sm text-[var(--text-muted)] mt-1">
                  Define your buying strategy — set targets, budgets, and filters.
                </p>
              </div>
              <CampaignFormFields
                values={form.values}
                onChange={(field, value) => form.handleChange(field as keyof CreateCampaignInput, value)}
                nameError={form.touched.name ? form.errors.name : undefined}
                onNameBlur={() => form.handleBlur('name')}
              />
              <div className="mt-5 flex justify-end">
                <Button type="submit" loading={form.isSubmitting || createMutation.isPending}>Create Campaign</Button>
              </div>
            </form>
          </CardShell>
        </div>
      )}

      {campaigns.length === 0 ? (
        <EmptyState
          icon="📋"
          title="No campaigns yet"
          description="Create your first campaign to start tracking purchases and sales."
          action={{ label: '+ New Campaign', onClick: onToggleCreate }}
        />
      ) : (
        <div className="flex flex-col gap-1">
          {/* Column headers — match responsive breakpoints of the row cells below.
              Aligns with each row's right cluster so columns scan vertically. */}
          <div className="hidden sm:flex items-center gap-3 px-3 py-1.5 text-2xs uppercase tracking-wider font-semibold text-[var(--text-subtle)]" aria-hidden="true">
            <div className="w-[3px] flex-shrink-0" />
            <div className="flex-1 min-w-0">Campaign</div>
            <div className="flex items-center gap-4 flex-shrink-0">
              <div className="hidden sm:flex items-center gap-3">
                <span className="tabular-nums text-right" style={{ minWidth: '5rem' }}>
                  <abbr title="Net Profit and Loss across all sales for this campaign." style={{ textDecoration: 'none', cursor: 'help' }}>P&amp;L</abbr>
                </span>
                <span className="tabular-nums text-right" style={{ minWidth: '3rem' }}>
                  <abbr title="Return on Investment: net profit divided by total spent." style={{ textDecoration: 'none', cursor: 'help' }}>ROI</abbr>
                </span>
              </div>
              <abbr className="hidden md:inline" style={{ minWidth: '5.25rem', textDecoration: 'none', cursor: 'help' }} title="Percent of cards in this campaign that have been sold.">Sell-through</abbr>
              <abbr className="hidden lg:inline" style={{ minWidth: '8.5rem', textDecoration: 'none', cursor: 'help' }} title="Daily spend cap and the percent of CL value paid on incoming buys (Buy%). Suppressed for closed campaigns.">Cap · Buy%</abbr>
              <span className="w-4" aria-hidden="true" />
            </div>
          </div>
          {campaigns.map((c, idx) => {
            const pnl = pnlMap[c.id];
            const isClosed = c.phase === 'closed';
            const isProfit = pnl ? pnl.netProfitCents >= 0 : true;
            const profitColor = isProfit ? 'text-[var(--success)]' : 'text-[var(--danger)]';
            const sectionHeader = phaseSections?.find(s => s.startIdx === idx);

            return (
              <Fragment key={c.id}>
                {sectionHeader && (
                  <div
                    className={`flex items-baseline gap-2 px-3 ${idx === 0 ? 'pt-1 pb-1' : 'pt-4 pb-1'}`}
                    aria-hidden="true"
                  >
                    <span className="text-[10px] font-semibold uppercase tracking-[0.14em] text-[var(--brand-400)]">
                      {PHASE_LABELS[sectionHeader.phase]}
                    </span>
                    <span className="text-[10px] font-medium tabular-nums text-[var(--text-subtle)]">
                      {sectionHeader.count}
                    </span>
                  </div>
                )}
                <div
                className={`group flex items-center gap-3 px-3 py-2.5 bg-[var(--surface-1)] rounded-lg border border-[var(--surface-2)] ${isClosed ? 'opacity-50' : ''}`}
              >
                {/* Phase accent bar */}
                <div
                  className="w-[3px] self-stretch rounded-full flex-shrink-0"
                  style={{ backgroundColor: phaseHexColors[c.phase] }}
                  aria-hidden="true"
                />
                <span className="sr-only">Phase: {c.phase}</span>

                {/* Left: name + filters */}
                <div className="flex flex-col min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-semibold text-[var(--text)] truncate">{c.name}</span>
                    <PhaseBadge phase={c.phase} />
                    {healthMap[c.id] && healthMap[c.id] !== 'healthy' && (
                      <>
                        <span
                          className={`inline-block w-1.5 h-1.5 rounded-full flex-shrink-0 ${
                            healthMap[c.id] === 'critical' ? 'bg-[var(--danger)]' : 'bg-[var(--warning)]'
                          }`}
                          aria-hidden="true"
                        />
                        <span className="sr-only">Health: {healthMap[c.id]}</span>
                      </>
                    )}
                  </div>
                  <FilterSummary c={c} />
                </div>

                {/* Right: inline stats. The P&L and Sell-through wrappers
                    render unconditionally so columns stay aligned with the
                    header even when pnl hasn't loaded for a given campaign. */}
                <div className="flex items-center gap-4 flex-shrink-0 text-xs text-[var(--text-muted)]">
                  {/* P&L + ROI. The visual column header is aria-hidden, so
                      each value carries its own metric label for screen
                      readers via aria-label. */}
                  <div className="hidden sm:flex items-center gap-3">
                    <span
                      className={`font-medium tabular-nums text-right ${pnl ? profitColor : ''}`}
                      style={{ minWidth: '5rem' }}
                      aria-label={pnl ? `Net profit ${formatCents(pnl.netProfitCents)}` : 'Net profit unavailable'}
                    >
                      {pnl ? formatCents(pnl.netProfitCents) : ''}
                    </span>
                    <span
                      className={`font-medium tabular-nums text-right ${pnl ? profitColor : ''}`}
                      style={{ minWidth: '3rem' }}
                      aria-label={pnl ? `ROI ${formatPct(pnl.roi)}` : 'ROI unavailable'}
                    >
                      {pnl ? formatPct(pnl.roi) : ''}
                    </span>
                  </div>

                  {/* Sell-through with mini bar (sell-through % + visible color bar) */}
                  {(() => {
                    if (!pnl) {
                      return (
                        <div
                          className="hidden md:flex items-center gap-2"
                          style={{ minWidth: '5.25rem' }}
                          aria-hidden="true"
                        />
                      );
                    }
                    const st = Math.max(0, Math.min(pnl.sellThroughPct ?? 0, 1));
                    const pctText = formatPct(st);
                    // Sell-through alone doesn't reflect campaign health — a fast-
                    // moving losing campaign would otherwise read as "all green."
                    // Tint by sell-through, then demote one tier on a loss so the
                    // bar matches the P/L signal next to it.
                    const baseTier = st >= 0.5 ? 'success' : st >= 0.10 ? 'warning' : 'danger';
                    const tier = isProfit
                      ? baseTier
                      : baseTier === 'success' ? 'warning' : 'danger';
                    const barColor = `var(--${tier})`;
                    return (
                      <div
                        className="hidden md:flex items-center gap-2"
                        style={{ minWidth: '5.25rem' }}
                        role="group"
                        aria-label={`Sell-through ${pctText}, ${pnl.totalSold} of ${pnl.totalPurchases} sold`}
                      >
                        <span className="tabular-nums">{pctText}</span>
                        <div
                          className="w-14 h-2.5 rounded-full bg-[var(--surface-3)] overflow-hidden"
                          role="progressbar"
                          aria-valuenow={Math.round(st * 100)}
                          aria-valuemin={0}
                          aria-valuemax={100}
                          aria-valuetext={`${pctText} sold`}
                          title={`${pnl.totalSold}/${pnl.totalPurchases} sold`}
                        >
                          <div
                            className="h-full rounded-full transition-[width] duration-300"
                            style={{
                              width: `${st * 100}%`,
                              background: barColor,
                            }}
                          />
                        </div>
                      </div>
                    );
                  })()}

                  {/* Cap · Buy% — suppressed on closed campaigns (not actively buying) */}
                  <span
                    className="hidden lg:inline tabular-nums text-right"
                    style={{ minWidth: '8.5rem' }}
                  >
                    {isClosed ? '' : `${formatDollarsWhole(c.dailySpendCapCents)}/d · ${formatPct(c.buyTermsCLPct)}`}
                  </span>
                  {(() => {
                    const indicator = pushIndicatorState(psaPushMap[c.id]);
                    return (
                      <Button
                        size="sm"
                        variant="ghost"
                        aria-label={`Publish to PSA for ${c.name}${indicator ? ` — ${PUSH_INDICATOR[indicator].label}` : ''}`}
                        onClick={() => setPsaModalCampaignId(c.id)}
                      >
                        PSA
                        {indicator && (
                          <span
                            className="inline-block w-1.5 h-1.5 rounded-full ml-1.5 align-middle"
                            style={{ backgroundColor: PUSH_INDICATOR[indicator].color }}
                            aria-hidden="true"
                          />
                        )}
                      </Button>
                    );
                  })()}
                </div>
              </div>
              </Fragment>
            );
          })}
        </div>
      )}

      {psaModalCampaign && (
        <PSAPublishModal
          open={!!psaModalCampaign}
          onClose={() => setPsaModalCampaignId(null)}
          campaign={psaModalCampaign}
          pushRow={psaPushMap[psaModalCampaign.id] ?? null}
        />
      )}
    </>
  );
}
