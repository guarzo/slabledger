import { Link } from 'react-router-dom';
import type { Campaign, CampaignPNL, CreateCampaignInput, Phase } from '../../../types/campaigns';
import { formatCents, formatDollarsWhole, formatPct, formatPriceRange } from '../../utils/formatters';
import { EmptyState, Button } from '../../ui';
import CardShell from '../../ui/CardShell';
import CampaignFormFields from '../../ui/CampaignFormFields';
import type { UseFormReturn } from '../../hooks/useForm';
import { phaseHexColors } from '../../utils/campaignConstants';

function PhaseBadge({ phase }: { phase: Phase }) {
  const color = phaseHexColors[phase];
  return (
    <span
      className="text-[10px] font-semibold uppercase px-1.5 py-0.5 rounded-full tracking-wider"
      style={{ background: `${color}20`, color, border: `1px solid ${color}40` }}
    >
      {phase}
    </span>
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
  showCreate,
  form,
  createMutation,
  phaseFilter,
  phaseFilterLabel,
  onToggleCreate,
}: {
  campaigns: Campaign[];
  pnlMap: Record<string, CampaignPNL>;
  healthMap: Record<string, string>;
  showCreate: boolean;
  form: UseFormReturn<CreateCampaignInput>;
  createMutation: { isPending: boolean };
  phaseFilter: 'all' | Phase;
  phaseFilterLabel: string;
  onToggleCreate: () => void;
}) {
  const isFiltered = phaseFilter !== 'all';
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
          title={isFiltered ? `No ${phaseFilterLabel.toLowerCase()} campaigns` : 'No campaigns yet'}
          description={isFiltered ? `No campaigns are currently ${phaseFilterLabel.toLowerCase()}.` : 'Create your first campaign to start tracking purchases and sales.'}
          action={isFiltered ? undefined : { label: '+ New Campaign', onClick: onToggleCreate }}
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
          {campaigns.map(c => {
            const pnl = pnlMap[c.id];
            const isClosed = c.phase === 'closed';
            const isProfit = pnl ? pnl.netProfitCents >= 0 : true;
            const profitColor = isProfit ? 'text-[var(--success)]' : 'text-[var(--danger)]';

            return (
              <Link
                key={c.id}
                to={`/campaigns/${c.id}`}
                className={`group flex items-center gap-3 px-3 py-2.5 bg-[var(--surface-1)] rounded-lg border border-[var(--surface-2)] hover:border-[var(--brand-500)]/50 hover:bg-[var(--surface-0)] hover:-translate-y-0.5 hover:shadow-sm focus-ring transition-[color,border-color,background-color,transform,box-shadow] ${isClosed ? 'opacity-50' : ''}`}
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
                              background: st >= 0.5 ? 'var(--success)' : st >= 0.10 ? 'var(--warning)' : 'var(--danger)',
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

                  {/* Chevron */}
                  <svg
                    xmlns="http://www.w3.org/2000/svg"
                    viewBox="0 0 20 20"
                    fill="currentColor"
                    className="w-4 h-4 text-[var(--text-muted)] group-hover:text-[var(--brand-500)] transition-colors"
                    aria-hidden="true"
                  >
                    <path
                      fillRule="evenodd"
                      d="M7.21 14.77a.75.75 0 01.02-1.06L11.168 10 7.23 6.29a.75.75 0 111.04-1.08l4.5 4.25a.75.75 0 010 1.08l-4.5 4.25a.75.75 0 01-1.06-.02z"
                      clipRule="evenodd"
                    />
                  </svg>
                </div>
              </Link>
            );
          })}
        </div>
      )}
    </>
  );
}
