import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import CampaignsTab from './CampaignsTab';
import { ToastProvider } from '../../contexts/ToastContext';
import { AuthProvider } from '../../contexts/AuthContext';
import type { Campaign, CreateCampaignInput } from '../../../types/campaigns';
import type { PSAPushRow } from '../../../types/campaigns';
import type { UseFormReturn } from '../../hooks/useForm';
import { toFormValues } from '../../utils/campaignFormValues';

vi.mock('../../../js/api', () => ({
  api: { get: vi.fn().mockRejectedValue({ status: 401 }) },
  isAPIError: (err: unknown): err is { status: number } =>
    typeof err === 'object' && err !== null && 'status' in err,
}));

function makeCampaign(overrides: Partial<Campaign> = {}): Campaign {
  return {
    id: 'c1',
    name: 'Test Campaign',
    sport: 'Pokemon',
    yearRange: '',
    gradeRange: '',
    priceRange: '',
    clConfidence: '',
    buyTermsCLPct: 0.7,
    dailySpendCapCents: 100000,
    targetLanguages: [],
    subjectFilterMode: 'Target',
    subjects: [],
    deniedSpecs: [],
    phase: 'active',
    psaSourcingFeeCents: 0,
    ebayFeePct: 0,
    expectedFillRate: 0,
    psaCampaignRequestId: '',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  } as Campaign;
}

function makePush(overrides: Partial<PSAPushRow> = {}): PSAPushRow {
  return {
    campaignId: 'c1',
    pushId: 'push-1',
    operation: 'create',
    status: 'pending',
    updatedAt: '2026-07-14T12:00:00Z',
    ...overrides,
  };
}

const fakeForm = {
  values: {},
  errors: {},
  touched: {},
  isSubmitting: false,
  handleChange: vi.fn(),
  handleBlur: vi.fn(),
  handleSubmit: vi.fn((e: React.FormEvent) => e.preventDefault()),
  reset: vi.fn(),
} as unknown as UseFormReturn<CreateCampaignInput>;

function renderTab(
  campaigns: Campaign[],
  psaPushMap: Record<string, PSAPushRow>,
  opts: {
    editingCampaign?: Campaign | null;
    editForm?: UseFormReturn<CreateCampaignInput>;
    onEdit?: (c: Campaign) => void;
  } = {},
) {
  const { editingCampaign = null, editForm: editFormArg, onEdit = vi.fn() } = opts;
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <AuthProvider>
        <ToastProvider>
          <CampaignsTab
            campaigns={campaigns}
            pnlMap={{}}
            healthMap={{}}
            psaPushMap={psaPushMap}
            showCreate={false}
            form={fakeForm}
            createMutation={{ isPending: false }}
            onToggleCreate={vi.fn()}
            editingCampaign={editingCampaign}
            editForm={editFormArg ?? fakeForm}
            updateMutation={{ isPending: false }}
            onEdit={onEdit}
            onCancelEdit={vi.fn()}
          />
        </ToastProvider>
      </AuthProvider>
    </QueryClientProvider>
  );
}

describe('CampaignsTab PSA sync indicator', () => {
  const tests: Array<{
    name: string;
    campaign?: Partial<Campaign>;
    push: PSAPushRow | undefined;
    wantLabel: string;
    wantBadge: string;
  }> = [
    {
      name: 'never linked, no push row shows Not on PSA',
      push: undefined,
      wantLabel: 'Publish to PSA for Test Campaign — currently Not on PSA',
      wantBadge: 'Not on PSA',
    },
    {
      name: 'linked, no push row shows Synced',
      campaign: { psaCampaignRequestId: 'psa-123' },
      push: undefined,
      wantLabel: 'Publish to PSA for Test Campaign — currently Synced',
      wantBadge: 'Synced',
    },
    {
      name: 'pending push marks Pending',
      push: makePush({ status: 'pending' }),
      wantLabel: 'Publish to PSA for Test Campaign — currently Pending',
      wantBadge: 'Pending',
    },
    {
      name: 'approved push marks Pushing',
      push: makePush({ status: 'approved' }),
      wantLabel: 'Publish to PSA for Test Campaign — currently Pushing',
      wantBadge: 'Pushing',
    },
    {
      name: 'pushing push marks Pushing',
      push: makePush({ status: 'pushing' }),
      wantLabel: 'Publish to PSA for Test Campaign — currently Pushing',
      wantBadge: 'Pushing',
    },
    {
      name: 'failed push marks Failed',
      push: makePush({ status: 'failed' }),
      wantLabel: 'Publish to PSA for Test Campaign — currently Failed',
      wantBadge: 'Failed',
    },
    {
      name: 'pushed (resolved) push with no link falls back to Not on PSA',
      push: makePush({ status: 'pushed' }),
      wantLabel: 'Publish to PSA for Test Campaign — currently Not on PSA',
      wantBadge: 'Not on PSA',
    },
  ];

  it.each(tests)('$name', ({ campaign, push, wantLabel, wantBadge }) => {
    renderTab([makeCampaign(campaign)], push ? { c1: push } : {});
    expect(screen.getByRole('button', { name: wantLabel })).toBeInTheDocument();
    expect(screen.getByText(wantBadge)).toBeInTheDocument();
  });
});

it('reports the campaign to edit when Edit is clicked', () => {
  const onEdit = vi.fn();
  const campaign = makeCampaign();
  renderTab([campaign], {}, { onEdit });

  fireEvent.click(screen.getByRole('button', { name: /edit test campaign/i }));

  expect(onEdit).toHaveBeenCalledWith(campaign);
});

it('offers Edit on a closed campaign', () => {
  // A closed campaign's targeting is still worth correcting before it is
  // reopened, so the button is not phase-gated.
  renderTab([makeCampaign({ phase: 'closed' })], {});
  expect(screen.getByRole('button', { name: /edit test campaign/i })).toBeInTheDocument();
});

it('renders the edit card with the seeded form values when a campaign is being edited', () => {
  const campaign = makeCampaign({ name: 'Vintage Core' });
  const editForm = {
    ...fakeForm,
    values: toFormValues(campaign),
  } as unknown as UseFormReturn<CreateCampaignInput>;

  renderTab([campaign], {}, { editingCampaign: campaign, editForm });

  expect(screen.getByText('Edit Vintage Core')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /save changes/i })).toBeInTheDocument();
});
