import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import CampaignTuningTable from './CampaignTuningTable';
import DoNowSection from './DoNowSection';
import HealthSignalsTiles from './HealthSignalsTiles';
import type { Action, Signals, TuningRow } from '../../../types/insights';

function wrap(ui: React.ReactNode) {
  return render(<MemoryRouter>{ui}</MemoryRouter>);
}

describe('DoNowSection', () => {
  it('shows empty-state line when no actions', () => {
    wrap(<DoNowSection actions={[]} />);
    expect(screen.getByText(/Nothing needs your attention/i)).toBeInTheDocument();
  });

  it('renders severity dot and link for each action', () => {
    const actions: Action[] = [{
      id: 'a1',
      severity: 'act',
      title: 'Run profit-capture on 6 certs',
      detail: '+$386.79 net',
      link: { path: '/global-inventory', query: { filter: 'spike' } },
    }];
    wrap(<DoNowSection actions={actions} />);
    expect(screen.getByText(/Run profit-capture/i)).toBeInTheDocument();
    const link = screen.getByRole('link', { name: /Open:\s*Run profit-capture/i });
    expect(link).toHaveAttribute('href', '/global-inventory?filter=spike');
  });
});

describe('HealthSignalsTiles', () => {
  it('shows em-dash for AI accept rate when nothing resolved', () => {
    const signals: Signals = {
      aiAcceptRate: { pct: 0, accepted: 0, resolved: 0 },
      liquidationRecoverableUsd: 0,
      spikeProfitUsd: 0,
      spikeCertCount: 0,
      stuckInPipelineCount: 0,
    };
    wrap(<HealthSignalsTiles signals={signals} />);
    expect(screen.getByText('—')).toBeInTheDocument();
  });
});

describe('CampaignTuningTable', () => {
  it('renders "—" in cells with no recommendation', () => {
    const rows: TuningRow[] = [{
      campaignId: 'c1',
      campaignName: 'Cards 70-74 Volume',
      cells: { buyPct: { recommendation: 'Hold', severity: 'ok' } },
      status: 'OK',
    }];
    wrap(<CampaignTuningTable rows={rows} />);
    expect(screen.getAllByText('—').length).toBeGreaterThanOrEqual(3); // characters, years, spendCap
    expect(screen.getByText('Cards 70-74 Volume')).toBeInTheDocument();
  });

  it('shows empty state when no active campaigns', () => {
    wrap(<CampaignTuningTable rows={[]} />);
    expect(screen.getByText(/No active campaigns/)).toBeInTheDocument();
  });

  it('sorts rows so Action precedes Tune precedes OK', () => {
    const rows: TuningRow[] = [
      { campaignId: 'a', campaignName: 'A-OK',     cells: {}, status: 'OK' },
      { campaignId: 'b', campaignName: 'B-Action', cells: {}, status: 'Act' },
      { campaignId: 'c', campaignName: 'C-Tune',   cells: {}, status: 'Tune' },
      { campaignId: 'd', campaignName: 'D-Kill',   cells: {}, status: 'Kill' },
    ];
    wrap(<CampaignTuningTable rows={rows} />);
    const links = screen.getAllByRole('link');
    expect(links.map(l => l.textContent)).toEqual([
      expect.stringContaining('B-Action'),
      expect.stringContaining('D-Kill'),
      expect.stringContaining('C-Tune'),
      expect.stringContaining('A-OK'),
    ]);
  });

  it('applies status-colored left-edge strip per row', () => {
    const rows: TuningRow[] = [
      { campaignId: 'a', campaignName: 'A', cells: {}, status: 'Act' },
      { campaignId: 'b', campaignName: 'B', cells: {}, status: 'OK' },
    ];
    wrap(<CampaignTuningTable rows={rows} />);
    const actRow = screen.getByRole('link', { name: /A/ });
    const okRow = screen.getByRole('link', { name: /B/ });
    expect(actRow.getAttribute('style')).toMatch(/border-left-color:\s*var\(--danger\)/);
    expect(okRow.getAttribute('style')).toMatch(/border-left-color:\s*var\(--success\)/);
  });

  it('renders cell recommendations as RecommendationBadge text', () => {
    const rows: TuningRow[] = [{
      campaignId: 'c1',
      campaignName: 'Cards',
      cells: {
        buyPct: { recommendation: 'high', severity: 'act' },
        spendCap: { recommendation: 'low', severity: 'tune' },
      },
      status: 'Act',
    }];
    wrap(<CampaignTuningTable rows={rows} />);
    expect(screen.getByText('high')).toBeInTheDocument();
    expect(screen.getByText('low')).toBeInTheDocument();
  });
});
