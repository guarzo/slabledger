import { Link } from 'react-router-dom';
import type { Campaign, CampaignPNL, CreateCampaignInput, Phase } from '../../../types/campaigns';
import { formatCents, formatPct, formatPriceRange } from '../../utils/formatters';
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
  activeOnly,
  onToggleCreate,
}: {
  campaigns: Campaign[];
  pnlMap: Record<string, CampaignPNL>;
  healthMap: Record<string, string>;
  showCreate: boolean;
  form: UseFormReturn<CreateCampaignInput>;
  createMutation: { isPending: boolean };
  activeOnly: boolean;
  onToggleCreate: () => void;
}) {
  return (
    <>
      {showCreate && (
        <div className="relative overflow-hidden rounded-xl mb-6">
          <div className="absolute top-0 left-0 right-0 h-1 bg-gradient-to-r from-[var(--brand-500)] via-[var(--brand-400)] to-[var(--brand-600)]" />
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
          title={activeOnly ? 'No active campaigns' : 'No campaigns yet'}
          description={activeOnly ? 'No campaigns are currently active.' : 'Create your first campaign to start tracking purchases and sales.'}
          action={activeOnly ? undefined : { label: '+ New Campaign', onClick: onToggleCreate }}
        />
      ) : (
        <div className="flex flex-col gap-1">
          {campaigns.map(c => {
            const pnl = pnlMap[c.id];
            const isClosed = c.phase === 'closed';
            const isProfit = pnl ? pnl.netProfitCents >= 0 : true;
            const profitColor = isProfit ? 'text-[var(--success)]' : 'text-[var(--danger)]';

            return (
              <Link
                key={c.id}
                to={`/campaigns/${c.id}`}
                className={`group flex items-center gap-3 px-3 py-2.5 bg-[var(--surface-1)] rounded-lg border border-[var(--surface-2)] hover:border-[var(--brand-500)]/50 hover:bg-[var(--surface-0)] transition-colors ${isClosed ? 'opacity-50' : ''}`}
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
                    {healthMap[c.id] && (
                      <>
                        <span
                          className={`inline-block w-1.5 h-1.5 rounded-full flex-shrink-0 ${
                            healthMap[c.id] === 'critical' ? 'bg-[var(--danger)]' :
                            healthMap[c.id] === 'warning' ? 'bg-[var(--warning)]' : 'bg-[var(--success)]'
                          }`}
                          aria-hidden="true"
                        />
                        <span className="sr-only">Health: {healthMap[c.id]}</span>
                      </>
                    )}
                  </div>
                  <FilterSummary c={c} />
                </div>

                {/* Right: inline stats */}
                <div className="flex items-center gap-4 flex-shrink-0 text-xs text-[var(--text-muted)]">
                  {/* P&L */}
                  {pnl && (
                    <div className="hidden sm:flex items-center gap-3">
                      <span className={`font-medium ${profitColor}`}>
                        {formatCents(pnl.netProfitCents)}
                      </span>
                      <span className={`font-medium ${profitColor}`}>
                        {formatPct(pnl.roi)}
                      </span>
                    </div>
                  )}

                  {/* Sell-through with mini bar */}
                  {pnl && (() => {
                    const st = pnl.sellThroughPct ?? 0;
                    return (
                      <div className="hidden md:flex items-center gap-2">
                        <span>{pnl.totalSold}/{pnl.totalPurchases}</span>
                        <div className="w-12 h-1.5 rounded-full bg-[var(--surface-3)] overflow-hidden" title={`Sell-through: ${formatPct(pnl.sellThroughPct)}`}>
                          <div
                            className="h-full rounded-full transition-all duration-300"
                            style={{
                              width: `${Math.min(st * 100, 100)}%`,
                              background: st >= 0.5 ? 'var(--success)' : 'var(--warning)',
                            }}
                          />
                        </div>
                      </div>
                    );
                  })()}

                  {/* Buy terms */}
                  <span className="hidden lg:inline">
                    {formatCents(c.dailySpendCapCents)}/d @ {formatPct(c.buyTermsCLPct)}
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
