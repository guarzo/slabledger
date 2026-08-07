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

  it('shows the empty-catalog message for the real never-harvested payload shape', async () => {
    // PSAPortalCatalogStore.Subjects on a never-harvested table returns a nil
    // slice (subjects: null over JSON) and a zero time.Time (fetchedAt:
    // "0001-01-01T00:00:00Z"), not an error and not `subjects: []` — the
    // fixture above is a convenient stand-in but not what the server actually
    // sends. This case pins the real wire shape and, in the same assertion,
    // confirms that shape does NOT trigger the "over 7 days old" warning
    // (Date.now() - zero-time is enormous, so the empty-catalog gate must
    // suppress it).
    vi.mocked(api.listPSASubjects).mockResolvedValue({
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- simulating the real null the server sends, which the non-nullable TS type disallows
      subjects: null as any,
      fetchedAt: '0001-01-01T00:00:00Z',
    });
    renderEditor([]);
    await waitFor(() => {
      expect(screen.getByText(/not yet harvested/i)).toBeInTheDocument();
    });
    expect(screen.queryByText(/over 7 days old/i)).not.toBeInTheDocument();
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
    // The typed text must actually filter — Machamp doesn't match "char" and
    // must not appear in the dropdown alongside Charizard.
    expect(screen.queryByRole('button', { name: 'Machamp' })).not.toBeInTheDocument();
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

  it('suppresses an already-selected catalog subject from the dropdown', async () => {
    vi.mocked(api.listPSASubjects).mockResolvedValue({
      subjects: [{ id: 4807, name: 'Charizard' }, { id: 22210, name: 'Charmander' }],
      fetchedAt: '2026-08-01T00:00:00Z',
    });
    renderEditor([{ id: 4807, name: 'Charizard' }]);
    const input = await screen.findByPlaceholderText(/add a subject/i);
    fireEvent.change(input, { target: { value: 'char' } });
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Charmander' })).toBeInTheDocument();
    });
    // The chip's own "Remove Charizard" button is still present; what must be
    // absent is a bare "Charizard" dropdown option offering to add it twice.
    expect(screen.queryByRole('button', { name: 'Charizard' })).not.toBeInTheDocument();
  });

  it('suppresses a catalog subject whose name an operator-typed entry already holds', async () => {
    // The two-sided check: an operator-typed subject carries id 0, so an
    // id-only comparison would still offer the catalog's id-4807 "Charizard"
    // and produce two entries with the same name on one campaign.
    vi.mocked(api.listPSASubjects).mockResolvedValue({
      subjects: [{ id: 4807, name: 'Charizard' }, { id: 22210, name: 'Charmander' }],
      fetchedAt: '2026-08-01T00:00:00Z',
    });
    renderEditor([{ id: 0, name: 'charizard' }]);
    const input = await screen.findByPlaceholderText(/add a subject/i);
    fireEvent.change(input, { target: { value: 'char' } });
    // Charmander proves the catalog loaded and the dropdown opened — without
    // it, the Charizard assertion below would pass vacuously against a
    // dropdown that simply hadn't rendered yet.
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Charmander' })).toBeInTheDocument();
    });
    expect(screen.queryByRole('button', { name: 'Charizard' })).not.toBeInTheDocument();
  });

  it('does not duplicate on Enter when the typed name is already selected', async () => {
    // Enter-to-add bypasses the dropdown entirely, so addSubject needs the
    // same guard `matches` has — case-insensitively.
    vi.mocked(api.listPSASubjects).mockResolvedValue({
      subjects: [{ id: 4807, name: 'Charizard' }],
      fetchedAt: '2026-08-01T00:00:00Z',
    });
    const onChange = renderEditor([{ id: 4807, name: 'Charizard' }]);
    const input = await screen.findByPlaceholderText(/add a subject/i);
    fireEvent.change(input, { target: { value: 'charizard' } });
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(onChange).not.toHaveBeenCalled();
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

  it('shows a loading message, not the empty-catalog message, while the query is in flight', () => {
    // A never-resolving promise keeps useQuery in its loading state — distinct
    // from "not yet harvested", which is only correct once the catalog has
    // actually loaded and turned out empty.
    vi.mocked(api.listPSASubjects).mockReturnValue(new Promise(() => {}));
    renderEditor([]);
    expect(screen.getByText(/loading subject catalog/i)).toBeInTheDocument();
    expect(screen.queryByText(/not yet harvested/i)).not.toBeInTheDocument();
  });

  it('shows an error message, not the empty-catalog message, when the catalog request fails', async () => {
    // e.g. the 503 campaigns_psa.go returns when PSA sync is disabled — a
    // real failure, not "harvester hasn't run yet".
    vi.mocked(api.listPSASubjects).mockRejectedValue(new Error('PSA sync disabled'));
    renderEditor([]);
    await waitFor(() => {
      expect(screen.getByText(/could not load the subject catalog/i)).toBeInTheDocument();
    });
    expect(screen.queryByText(/not yet harvested/i)).not.toBeInTheDocument();
  });

  it('flags legacy unreconciled subjects (id -1) without disturbing operator-typed ones (id 0)', async () => {
    // -1 is inventory.LegacyUnreconciledSubjectID, backfilled by migration
    // 000024 onto pre-axis subjects. Push translation refuses a campaign that
    // still carries one, so the reason has to be visible here. id 0 is the
    // unrelated, fully supported "resolve this name at push time" marker.
    vi.mocked(api.listPSASubjects).mockResolvedValue({
      subjects: [{ id: 22210, name: 'Machamp' }],
      fetchedAt: '2026-08-01T00:00:00Z',
    });
    const onChange = renderEditor([
      { id: -1, name: 'Blastoise' },
      { id: 0, name: 'Mewtwo' },
      { id: 4807, name: 'Charizard' },
    ]);

    await waitFor(() => {
      expect(screen.getByText(/baseline pull/i)).toBeInTheDocument();
    });
    expect(screen.getByTitle('legacy subject — no portal id yet; run the harvester baseline pull')).toHaveTextContent('Blastoise');
    expect(screen.getByTitle('id: 0')).toHaveTextContent('Mewtwo');
    expect(screen.getByTitle('id: 4807')).toHaveTextContent('Charizard');

    // Legacy chips stay removable — removing one is a deliberate operator edit.
    fireEvent.click(screen.getByRole('button', { name: /remove blastoise/i }));
    expect(onChange).toHaveBeenCalledWith([{ id: 0, name: 'Mewtwo' }, { id: 4807, name: 'Charizard' }]);
  });
});
