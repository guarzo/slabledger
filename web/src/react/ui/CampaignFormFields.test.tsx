import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import CampaignFormFields, { type CampaignFormValues } from './CampaignFormFields';

vi.mock('../../js/api', () => ({
  api: { listPSASubjects: vi.fn().mockResolvedValue({ subjects: [], fetchedAt: '' }) },
}));

function baseValues(): CampaignFormValues {
  return {
    name: 'Test',
    sport: 'Pokemon',
    yearRange: '',
    gradeRange: '',
    priceRange: '',
    clConfidence: '',
    buyTermsCLPct: 0.7,
    dailySpendCapCents: 50000,
    targetLanguages: [],
    subjectFilterMode: 'Target',
    subjects: [],
    deniedSpecs: [],
    psaSourcingFeeCents: 300,
    ebayFeePct: 0.1235,
  };
}

function renderFields(values: CampaignFormValues, onChange = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <CampaignFormFields values={values} onChange={onChange} />
    </QueryClientProvider>,
  );
  return onChange;
}

describe('CampaignFormFields targeting section', () => {
  it('checking a language adds its token to the set', () => {
    const onChange = renderFields(baseValues());
    fireEvent.click(screen.getByRole('checkbox', { name: 'Japanese' }));
    expect(onChange).toHaveBeenCalledWith('targetLanguages', ['japanese']);
  });

  it('checking a second language keeps the first — the live campaigns carry both', () => {
    const onChange = renderFields({ ...baseValues(), targetLanguages: ['english'] });
    fireEvent.click(screen.getByRole('checkbox', { name: 'Japanese' }));
    expect(onChange).toHaveBeenCalledWith('targetLanguages', ['english', 'japanese']);
  });

  it('unchecking a language removes only that token', () => {
    const onChange = renderFields({ ...baseValues(), targetLanguages: ['english', 'japanese'] });
    fireEvent.click(screen.getByRole('checkbox', { name: 'English' }));
    expect(onChange).toHaveBeenCalledWith('targetLanguages', ['japanese']);
  });

  it('reflects the current selection in the checkbox states', () => {
    renderFields({ ...baseValues(), targetLanguages: ['japanese'] });
    expect(screen.getByRole('checkbox', { name: 'Japanese' })).toBeChecked();
    expect(screen.getByRole('checkbox', { name: 'English' })).not.toBeChecked();
  });

  it('reads an empty selection as an open net, not as an unfilled field', () => {
    renderFields(baseValues());
    expect(screen.getByText(/open net/i)).toBeInTheDocument();
    expect(screen.getByText(/buys any language/i)).toBeInTheDocument();
  });

  it('describes a non-empty selection in plain words', () => {
    renderFields({ ...baseValues(), targetLanguages: ['english', 'japanese'] });
    expect(screen.getByText(/Buys English and Japanese cards only\./)).toBeInTheDocument();
  });

  it('surfaces an unknown token and preserves it through an unrelated toggle', () => {
    // Defensive: the backend closed set is {english, japanese} today. If a
    // future token arrives before this copy of the set is updated, it must be
    // visible — and toggling a known box must not drop it (see toggleLanguage).
    const onChange = renderFields({ ...baseValues(), targetLanguages: ['chinese'] });
    expect(screen.getByText(/Unrecognized language token: chinese/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('checkbox', { name: 'English' }));
    expect(onChange).toHaveBeenCalledWith('targetLanguages', ['chinese', 'english']);
  });

  it('offers an explicit removal for an unknown token, since validation blocks the save', () => {
    // normalizeTargetLanguages rejects the whole campaign on the first unknown
    // token, and toggling a known checkbox deliberately preserves unknowns —
    // so without this button the form is unsaveable with no way out. The
    // removal must be operator-initiated, never a side effect of an unrelated
    // toggle (the case above).
    const onChange = renderFields({ ...baseValues(), targetLanguages: ['chinese', 'english'] });
    fireEvent.click(screen.getByRole('button', { name: 'Remove chinese' }));
    expect(onChange).toHaveBeenCalledWith('targetLanguages', ['english']);
  });

  it('describes an unknown-only selection without asserting an empty buy scope or an open net', () => {
    // A set of only unrecognized tokens is neither empty (not open net) nor
    // enumerable in plain words (nothing recognized to name) — the summary
    // must not degenerate into "Buys  cards only." nor claim "open net".
    renderFields({ ...baseValues(), targetLanguages: ['chinese'] });
    expect(screen.getByText(/see warning below/i)).toBeInTheDocument();
    expect(screen.queryByText(/open net/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/^Buys\s+cards only\.$/)).not.toBeInTheDocument();
  });

  it('toggling the subject mode segmented control calls onChange with Exclude', () => {
    const onChange = renderFields(baseValues());
    fireEvent.click(screen.getByRole('radio', { name: 'Exclude' }));
    expect(onChange).toHaveBeenCalledWith('subjectFilterMode', 'Exclude');
  });

  it('shows portal-managed denied specs read-only when present', () => {
    renderFields({ ...baseValues(), deniedSpecs: [{ id: 999, name: 'Bad Card' }] });
    expect(screen.getByText('Bad Card')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /remove bad card/i })).not.toBeInTheDocument();
  });
});
