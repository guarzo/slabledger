import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import CampaignsTab from './CampaignsTab';
import { ToastProvider } from '../../contexts/ToastContext';
import { AuthProvider } from '../../contexts/AuthContext';
import type { Campaign, CreateCampaignInput } from '../../../types/campaigns';
import type { PSAPushRow } from '../../../types/campaigns';
import type { UseFormReturn } from '../../hooks/useForm';

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
    inclusionList: '',
    exclusionMode: false,
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

function renderTab(campaigns: Campaign[], psaPushMap: Record<string, PSAPushRow>) {
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
          />
        </ToastProvider>
      </AuthProvider>
    </QueryClientProvider>
  );
}

describe('CampaignsTab PSA push indicator', () => {
  const tests: Array<{
    name: string;
    push: PSAPushRow | undefined;
    wantLabel: string;
  }> = [
    {
      name: 'no push row keeps the plain label',
      push: undefined,
      wantLabel: 'Publish to PSA for Test Campaign',
    },
    {
      name: 'pending push marks approval pending',
      push: makePush({ status: 'pending' }),
      wantLabel: 'Publish to PSA for Test Campaign — approval pending',
    },
    {
      name: 'approved push marks in flight',
      push: makePush({ status: 'approved' }),
      wantLabel: 'Publish to PSA for Test Campaign — push in flight',
    },
    {
      name: 'pushing push marks in flight',
      push: makePush({ status: 'pushing' }),
      wantLabel: 'Publish to PSA for Test Campaign — push in flight',
    },
    {
      name: 'failed push marks failure',
      push: makePush({ status: 'failed' }),
      wantLabel: 'Publish to PSA for Test Campaign — last push failed',
    },
    {
      name: 'pushed (resolved) push keeps the plain label',
      push: makePush({ status: 'pushed' }),
      wantLabel: 'Publish to PSA for Test Campaign',
    },
  ];

  it.each(tests)('$name', ({ push, wantLabel }) => {
    renderTab([makeCampaign()], push ? { c1: push } : {});
    expect(screen.getByRole('button', { name: wantLabel })).toBeInTheDocument();
  });
});
