import { Link } from 'react-router-dom';
import type { Campaign, CampaignPNL, CreateCampaignInput, Phase } from '../../../types/campaigns';
import { formatCents, formatPct } from '../../utils/formatters';
import { EmptyState, Button } from '../../ui';
import CardShell from '../../ui/CardShell';
import CampaignFormFields from '../../ui/CampaignFormFields';
import PNLBadge from './PNLBadge';
import type { UseFormReturn } from '../../hooks/useForm';

export default function CampaignsTab({
  campaigns,
  pnlMap,
  healthMap,
  showCreate,
  form,
  createMutation,
  activeOnly,
  onToggleCreate,
  phaseGradients,
}: {
  campaigns: Campaign[];
  pnlMap: Record<string, CampaignPNL>;
  healthMap: Record<string, string>;
  showCreate: boolean;
  form: UseFormReturn<CreateCampaignInput>;
  createMutation: { isPending: boolean };
  activeOnly: boolean;
  onToggleCreate: () => void;
  phaseGradients: Record<Phase, string>;
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
        <div className="grid gap-4">
          {campaigns.map(c => (
            <Link key={c.id} to={`/campaigns/${c.id}`}
              className="block p-4 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)] hover:border-[var(--brand-500)]/50 hover:-translate-y-0.5 hover:shadow-[var(--shadow-2)] transition-all">
              <div className="flex items-center justify-between">
                <div>
                  <div className="flex items-center gap-2 mb-1">
                    <h3 className="text-lg font-semibold text-[var(--text)]">{c.name}</h3>
                    <span
                      className="px-2 py-0.5 text-xs font-medium text-white rounded-full"
                      style={{ background: phaseGradients[c.phase] }}
                    >
                      {c.phase}
                    </span>
                    {healthMap[c.id] && (
                      <span
                        className={`inline-block w-2 h-2 rounded-full ${
                          healthMap[c.id] === 'critical' ? 'bg-[var(--danger)]' :
                          healthMap[c.id] === 'warning' ? 'bg-[var(--warning)]' : 'bg-[var(--success)]'
                        }`}
                        title={`Health: ${healthMap[c.id]}`}
                      />
                    )}
                  </div>
                  <div className="flex gap-4 text-sm text-[var(--text-muted)]">
                    <span>{c.sport}</span>
                    {c.yearRange && <span>{c.yearRange}</span>}
                    {c.gradeRange && <span>PSA {c.gradeRange}</span>}
                    {c.priceRange && <span>${c.priceRange}</span>}
                  </div>
                </div>
                <div className="text-right text-sm text-[var(--text-muted)]">
                  <div>Buy at {formatPct(c.buyTermsCLPct)} CL</div>
                  <div>Cap: {formatCents(c.dailySpendCapCents)}/day</div>
                </div>
              </div>
              {pnlMap[c.id] && <PNLBadge pnl={pnlMap[c.id]} />}
            </Link>
          ))}
        </div>
      )}
    </>
  );
}
