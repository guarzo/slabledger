import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import PSAPublishModal from './PSAPublishModal';
import { ToastProvider } from '../../contexts/ToastContext';
import { AuthProvider } from '../../contexts/AuthContext';
import type { Campaign, PSAPushRow } from '../../../types/campaigns';

vi.mock('../../../js/api', () => ({
  api: {
    get: vi.fn().mockRejectedValue({ status: 401 }),
    listPSACampaigns: vi.fn(),
    psaLink: vi.fn(),
    psaPropose: vi.fn(),
    psaProposeCreate: vi.fn(),
    psaPublish: vi.fn(),
  },
  isAPIError: (err: unknown): err is { status: number } =>
    typeof err === 'object' && err !== null && 'status' in err,
}));

import { api } from '../../../js/api';

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
    psaCampaignRequestId: 'PSA-123',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  } as Campaign;
}

function modalTree(campaign: Campaign, pushRow: PSAPushRow | null, qc: QueryClient, onClose = vi.fn()) {
  return (
    <QueryClientProvider client={qc}>
      <AuthProvider>
        <ToastProvider>
          <PSAPublishModal open={true} onClose={onClose} campaign={campaign} pushRow={pushRow} />
        </ToastProvider>
      </AuthProvider>
    </QueryClientProvider>
  );
}

function renderModal(campaign: Campaign, pushRow: PSAPushRow | null = null, onClose = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(modalTree(campaign, pushRow, qc, onClose));
}

describe('PSAPublishModal', () => {
  beforeEach(() => {
    vi.mocked(api.listPSACampaigns).mockReset();
    vi.mocked(api.psaLink).mockReset();
    vi.mocked(api.psaPropose).mockReset();
    vi.mocked(api.psaProposeCreate).mockReset();
    vi.mocked(api.psaPublish).mockReset();
  });

  it('renders a pending diff after checking for changes', async () => {
    vi.mocked(api.psaPropose).mockResolvedValue({
      pushId: 'push-1',
      diff: { changes: [{ field: 'bidPercentage', old: '70', new: '80' }] },
    });

    renderModal(makeCampaign());

    fireEvent.click(screen.getByRole('button', { name: /check for changes/i }));

    await waitFor(() => {
      expect(screen.getByText(/bidPercentage/)).toBeInTheDocument();
    });
    expect(screen.getByText(/70/)).toBeInTheDocument();
    expect(screen.getByText(/80/)).toBeInTheDocument();
  });

  it('clicking Publish to PSA calls api.psaPublish with the pending pushId', async () => {
    vi.mocked(api.psaPropose).mockResolvedValue({
      pushId: 'push-1',
      diff: { changes: [{ field: 'bidPercentage', old: '70', new: '80' }] },
    });
    vi.mocked(api.psaPublish).mockResolvedValue({ pushId: 'push-1', status: 'approved' });

    renderModal(makeCampaign());

    fireEvent.click(screen.getByRole('button', { name: /check for changes/i }));

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /publish to psa/i })).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole('button', { name: /publish to psa/i }));

    await waitFor(() => {
      expect(vi.mocked(api.psaPublish)).toHaveBeenCalledWith('c1', 'push-1');
    });
  });

  it('Create on PSA flow proposes a create then queues it', async () => {
    vi.mocked(api.listPSACampaigns).mockResolvedValue({ campaigns: [], fetchedAt: '' });
    vi.mocked(api.psaProposeCreate).mockResolvedValue({
      pushId: 'push-create-1',
      formData: {
        campaignName: 'Modern 10s',
        campaignType: 'CATEGORY',
        category: 'POKEMON',
        prepackagedSpecListIds: [],
        isActive: false,
        bidPercentage: 72,
        flatFee: 3,
        dailyBudget: 3000,
        dailySpecLimit: 2,
        gradeMinimum: '10',
        gradeMaximum: '10',
        yearMinimum: 2024,
        yearMaximum: 2026,
        priceMinimum: 500,
        priceMaximum: 3000,
        cardLadderConfidenceMinimum: 3,
        publisherFilterType: 'Target',
        selectedPublishers: [],
        subjectFilterType: 'Target',
        selectedSubjects: [],
        deniedSpecs: [],
      },
    });
    vi.mocked(api.psaPublish).mockResolvedValue({ pushId: 'push-create-1', status: 'approved' });

    renderModal(makeCampaign({ psaCampaignRequestId: '' }));

    fireEvent.click(screen.getByRole('button', { name: /create on psa/i }));

    await waitFor(() => {
      expect(vi.mocked(api.psaProposeCreate)).toHaveBeenCalledWith('c1');
    });
    // Wait for the preview to actually render (onSuccess committed to the DOM)
    // before asserting on its fields, to avoid intermittent flakiness.
    await waitFor(() => {
      expect(screen.getByText(/PAUSED/)).toBeInTheDocument();
    });
    expect(screen.getByText(/72%/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /approve & queue create/i }));

    await waitFor(() => {
      expect(vi.mocked(api.psaPublish)).toHaveBeenCalledWith('c1', 'push-create-1');
    });
  });
});

describe('PSAPublishModal with a queued push row', () => {
  beforeEach(() => {
    vi.mocked(api.listPSACampaigns).mockReset();
    vi.mocked(api.psaLink).mockReset();
    vi.mocked(api.psaPropose).mockReset();
    vi.mocked(api.psaProposeCreate).mockReset();
    vi.mocked(api.psaPublish).mockReset();
  });

  const pendingCreateRow: PSAPushRow = {
    campaignId: 'c1',
    pushId: 'push-queued-1',
    operation: 'create',
    status: 'pending',
    requestedBy: 'alice',
    updatedAt: '2026-07-14T12:00:00Z',
    formData: {
      campaignName: 'Modern 10s',
      campaignType: 'CATEGORY',
      category: 'POKEMON',
      prepackagedSpecListIds: [],
      isActive: false,
      bidPercentage: 72,
      flatFee: 3,
      dailyBudget: 3000,
      dailySpecLimit: 2,
      gradeMinimum: '10',
      gradeMaximum: '10',
      yearMinimum: 2024,
      yearMaximum: 2026,
      priceMinimum: 500,
      priceMaximum: 3000,
      cardLadderConfidenceMinimum: 3,
      publisherFilterType: 'Target',
      selectedPublishers: [],
      subjectFilterType: 'Target',
      selectedSubjects: [],
      deniedSpecs: [],
    },
  };

  it('renders a queued create preview and approves it without proposing', async () => {
    vi.mocked(api.listPSACampaigns).mockResolvedValue({ campaigns: [], fetchedAt: '' });
    vi.mocked(api.psaPublish).mockResolvedValue({ pushId: 'push-queued-1', status: 'approved' });

    renderModal(makeCampaign({ psaCampaignRequestId: '' }), pendingCreateRow);

    // Preview renders straight from the queued row — no propose call.
    expect(screen.getByText(/Modern 10s/)).toBeInTheDocument();
    expect(screen.getByText(/72%/)).toBeInTheDocument();
    expect(vi.mocked(api.psaProposeCreate)).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole('button', { name: /approve & queue create/i }));

    await waitFor(() => {
      expect(vi.mocked(api.psaPublish)).toHaveBeenCalledWith('c1', 'push-queued-1');
    });
  });

  it('renders a queued update diff with a publish button', () => {
    const pendingUpdateRow: PSAPushRow = {
      campaignId: 'c1',
      pushId: 'push-queued-2',
      operation: 'update',
      status: 'pending',
      updatedAt: '2026-07-14T12:00:00Z',
      diff: { changes: [{ field: 'bidPercentage', old: '70', new: '80' }] },
    };
    renderModal(makeCampaign(), pendingUpdateRow);

    expect(screen.getByText(/bidPercentage/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /publish to psa/i })).toBeInTheDocument();
    expect(vi.mocked(api.psaPropose)).not.toHaveBeenCalled();
  });

  it('shows in-flight status and hides action buttons for an approved row', () => {
    const approvedRow: PSAPushRow = {
      ...pendingCreateRow,
      status: 'approved',
      approvedBy: 'bob',
    };
    vi.mocked(api.listPSACampaigns).mockResolvedValue({ campaigns: [], fetchedAt: '' });

    renderModal(makeCampaign({ psaCampaignRequestId: '' }), approvedRow);

    expect(screen.getByText(/push in flight/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /create on psa/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /approve & queue create/i })).not.toBeInTheDocument();
  });

  it('hides the update publish button while a push is in flight', () => {
    const inFlightUpdateRow: PSAPushRow = {
      campaignId: 'c1',
      pushId: 'push-inflight-1',
      operation: 'update',
      status: 'approved',
      approvedBy: 'bob',
      updatedAt: '2026-07-14T12:00:00Z',
      diff: { changes: [{ field: 'bidPercentage', old: '70', new: '80' }] },
    };
    renderModal(makeCampaign(), inFlightUpdateRow);

    expect(screen.getByText(/push in flight/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /publish to psa/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /check for changes/i })).not.toBeInTheDocument();
  });

  it('shows the stored error for a failed row and keeps the retry path', () => {
    const failedRow: PSAPushRow = {
      campaignId: 'c1',
      pushId: 'push-failed-1',
      operation: 'update',
      status: 'failed',
      error: 'portal returned 500',
      updatedAt: '2026-07-14T12:00:00Z',
    };
    renderModal(makeCampaign(), failedRow);

    expect(screen.getByText(/portal returned 500/)).toBeInTheDocument();
    // Retry path: the normal propose button stays available.
    expect(screen.getByRole('button', { name: /check for changes/i })).toBeInTheDocument();
  });

  it('drops stale local state when the queued row is superseded by a different push', async () => {
    vi.mocked(api.psaPropose).mockResolvedValue({
      pushId: 'push-old',
      diff: { changes: [{ field: 'bidPercentage', old: '70', new: '80' }] },
    });
    vi.mocked(api.psaPublish).mockResolvedValue({ pushId: 'push-new', status: 'approved' });

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const campaign = makeCampaign();
    const view = render(modalTree(campaign, null, qc));

    // Build local state for push-old via the normal propose flow.
    fireEvent.click(screen.getByRole('button', { name: /check for changes/i }));
    await waitFor(() => {
      expect(screen.getByText(/bidPercentage/)).toBeInTheDocument();
    });

    // The shared query refetch delivers a different pending push (superseded
    // out-of-band) — the row must win over the stale local diff/pushId.
    const supersededRow: PSAPushRow = {
      campaignId: 'c1',
      pushId: 'push-new',
      operation: 'update',
      status: 'pending',
      updatedAt: '2026-07-15T12:00:00Z',
      diff: { changes: [{ field: 'flatFee', old: '3', new: '4' }] },
    };
    view.rerender(modalTree(campaign, supersededRow, qc));

    expect(screen.getByText(/flatFee/)).toBeInTheDocument();
    expect(screen.queryByText(/bidPercentage/)).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /publish to psa/i }));
    await waitFor(() => {
      expect(vi.mocked(api.psaPublish)).toHaveBeenCalledWith('c1', 'push-new');
    });
  });
});
