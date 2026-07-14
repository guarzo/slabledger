import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import PSAPublishModal from './PSAPublishModal';
import { ToastProvider } from '../../contexts/ToastContext';
import { AuthProvider } from '../../contexts/AuthContext';
import type { Campaign } from '../../../types/campaigns';

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

function renderModal(campaign: Campaign, onClose = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <AuthProvider>
        <ToastProvider>
          <PSAPublishModal open={true} onClose={onClose} campaign={campaign} />
        </ToastProvider>
      </AuthProvider>
    </QueryClientProvider>
  );
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
