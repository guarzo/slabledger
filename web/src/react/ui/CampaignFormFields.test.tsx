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
    targetLanguage: '',
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
  it('changing the language select calls onChange with the token', () => {
    const onChange = renderFields(baseValues());
    fireEvent.change(screen.getByLabelText(/language/i), { target: { value: 'japanese' } });
    expect(onChange).toHaveBeenCalledWith('targetLanguage', 'japanese');
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
