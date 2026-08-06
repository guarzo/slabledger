import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import SubjectListEditor from './SubjectListEditor';

vi.mock('../../js/api', () => ({
  api: { listPSASubjects: vi.fn() },
}));

import { api } from '../../js/api';

function renderEditor(value: { id: number; name: string }[], onChange = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <SubjectListEditor label="Subjects" value={value} onChange={onChange} />
    </QueryClientProvider>,
  );
  return onChange;
}

describe('SubjectListEditor', () => {
  it('shows an empty-catalog message when no subjects have been harvested', async () => {
    vi.mocked(api.listPSASubjects).mockResolvedValue({ subjects: [], fetchedAt: '' });
    renderEditor([]);
    await waitFor(() => {
      expect(screen.getByText(/not yet harvested/i)).toBeInTheDocument();
    });
  });

  it('filters the catalog by typed text and adds a chip on selection, preserving the id', async () => {
    vi.mocked(api.listPSASubjects).mockResolvedValue({
      subjects: [{ id: 22210, name: 'Machamp' }, { id: 4807, name: 'Charizard' }],
      fetchedAt: '2026-08-01T00:00:00Z',
    });
    const onChange = renderEditor([]);
    const input = await screen.findByPlaceholderText(/add a subject/i);
    fireEvent.change(input, { target: { value: 'char' } });
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Charizard' })).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole('button', { name: 'Charizard' }));
    expect(onChange).toHaveBeenCalledWith([{ id: 4807, name: 'Charizard' }]);
  });

  it('adds an unresolved name with id 0 on Enter when there is no exact catalog match', async () => {
    vi.mocked(api.listPSASubjects).mockResolvedValue({
      subjects: [{ id: 22210, name: 'Machamp' }],
      fetchedAt: '2026-08-01T00:00:00Z',
    });
    const onChange = renderEditor([]);
    const input = await screen.findByPlaceholderText(/add a subject/i);
    fireEvent.change(input, { target: { value: 'Mewtwo' } });
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(onChange).toHaveBeenCalledWith([{ id: 0, name: 'Mewtwo' }]);
  });

  it('removes a chip', () => {
    const onChange = renderEditor([{ id: 4807, name: 'Charizard' }]);
    fireEvent.click(screen.getByRole('button', { name: /remove charizard/i }));
    expect(onChange).toHaveBeenCalledWith([]);
  });

  it('warns when the catalog is older than 7 days', async () => {
    vi.mocked(api.listPSASubjects).mockResolvedValue({
      subjects: [{ id: 1, name: 'Pikachu' }],
      fetchedAt: '2020-01-01T00:00:00Z',
    });
    renderEditor([]);
    await waitFor(() => {
      expect(screen.getByText(/over 7 days old/i)).toBeInTheDocument();
    });
  });
});
