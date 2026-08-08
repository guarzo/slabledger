import { describe, it, expect, vi } from 'vitest';
import { useState } from 'react';
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

// `renderEditor` pins `value`, so a removal never actually leaves the rendered
// list. That is fine for display assertions but it masks the removed-legacy
// block: the duplicate guard in `addSubject` would refuse the re-add on its own
// and the test would pass with the block deleted. These tests need the removal
// to be real, so this harness feeds `onChange` back in as state.
function renderControlledEditor(initial: { id: number; name: string }[]) {
  const onChange = vi.fn();
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  function Harness() {
    const [value, setValue] = useState(initial);
    return (
      <SubjectListEditor
        label="Subjects"
        value={value}
        onChange={next => { onChange(next); setValue(next); }}
      />
    );
  }
  render(
    <QueryClientProvider client={qc}>
      <Harness />
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

    // Legacy chips stay removable — removing one is a deliberate operator edit,
    // now behind ConfirmDialog (see the dedicated tests below).
    fireEvent.click(screen.getByRole('button', { name: /remove blastoise/i }));
    fireEvent.click(await screen.findByRole('button', { name: 'Remove' }));
    expect(onChange).toHaveBeenCalledWith([{ id: 0, name: 'Mewtwo' }, { id: 4807, name: 'Charizard' }]);
  });

  it('asks for confirmation before removing a legacy (-1) chip, and keeps it when declined', async () => {
    vi.mocked(api.listPSASubjects).mockResolvedValue({
      subjects: [{ id: 22210, name: 'Machamp' }],
      fetchedAt: '2026-08-01T00:00:00Z',
    });
    const onChange = renderEditor([{ id: -1, name: 'Machamp' }]);

    fireEvent.click(screen.getByRole('button', { name: /remove machamp/i }));

    // role=alertdialog, not a native prompt — that's the point of the migration.
    expect(await screen.findByRole('alertdialog')).toHaveTextContent(/Remove .Machamp./);
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(onChange).not.toHaveBeenCalled();
    // The chip survives a decline...
    expect(screen.getByTitle(/legacy subject/i)).toHaveTextContent('Machamp');
  });

  it('leaves the name freely addable after a declined removal', async () => {
    // `removedLegacyNames` must be written only on confirm. If a refactor hoists
    // that update above the early return, a declined dialog would silently burn
    // the name for the rest of the session.
    vi.mocked(api.listPSASubjects).mockResolvedValue({
      subjects: [{ id: 22210, name: 'Machamp' }, { id: 4807, name: 'Charizard' }],
      fetchedAt: '2026-08-01T00:00:00Z',
    });
    const onChange = renderControlledEditor([{ id: -1, name: 'Machamp' }]);

    fireEvent.click(screen.getByRole('button', { name: /remove machamp/i }));
    fireEvent.click(await screen.findByRole('button', { name: 'Cancel' }));
    onChange.mockClear();

    const input = await screen.findByPlaceholderText(/add a subject/i);
    fireEvent.change(input, { target: { value: 'Gyarados' } });
    fireEvent.keyDown(input, { key: 'Enter' });

    expect(onChange).toHaveBeenCalledWith([{ id: -1, name: 'Machamp' }, { id: 0, name: 'Gyarados' }]);
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('blocks re-adding a confirmed-removed legacy name, in the dropdown and on Enter', async () => {
    // Remove-then-retype is the hand repair the design forbids: SubjectID
    // matches names with strings.EqualFold (resolver.go:146), so retyping
    // "Machamp" would resolve to the unrelated portal subject 22210 and push
    // wrong targeting — the one failure mode that is silent rather than a 400.
    vi.mocked(api.listPSASubjects).mockResolvedValue({
      subjects: [{ id: 22210, name: 'Machamp' }, { id: 4807, name: 'Charizard' }],
      fetchedAt: '2026-08-01T00:00:00Z',
    });
    const onChange = renderControlledEditor([{ id: -1, name: 'Machamp' }]);

    fireEvent.click(screen.getByRole('button', { name: /remove machamp/i }));
    fireEvent.click(await screen.findByRole('button', { name: 'Remove' }));
    expect(onChange).toHaveBeenCalledWith([]);
    onChange.mockClear();

    const input = await screen.findByPlaceholderText(/add a subject/i);
    fireEvent.change(input, { target: { value: 'cha' } });
    // Charizard proves the catalog loaded and the dropdown opened, so the
    // Machamp assertion below cannot pass vacuously.
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Charizard' })).toBeInTheDocument();
    });

    // Deliberately a case variant, not the string that was removed —
    // case-insensitivity is the property the whole guard rests on, on both
    // the dropdown-filter and Enter paths.
    fireEvent.change(input, { target: { value: 'mAcHaMp' } });
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: 'Machamp' })).not.toBeInTheDocument();
    });
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(onChange).not.toHaveBeenCalled();
  });

  it('explains the refusal instead of silently swallowing a blocked name', async () => {
    vi.mocked(api.listPSASubjects).mockResolvedValue({
      subjects: [{ id: 22210, name: 'Machamp' }],
      fetchedAt: '2026-08-01T00:00:00Z',
    });
    const onChange = renderControlledEditor([{ id: -1, name: 'Machamp' }]);

    fireEvent.click(screen.getByRole('button', { name: /remove machamp/i }));
    fireEvent.click(await screen.findByRole('button', { name: 'Remove' }));
    onChange.mockClear();

    const input = await screen.findByPlaceholderText(/add a subject/i);
    expect(screen.queryByRole('status')).not.toBeInTheDocument();

    fireEvent.change(input, { target: { value: 'MACHAMP' } });
    fireEvent.keyDown(input, { key: 'Enter' });

    expect(onChange).not.toHaveBeenCalled();
    // The old behaviour cleared the input and said nothing at all.
    expect(screen.getByRole('status')).toHaveTextContent(/cannot be added back/i);
    expect(input).toHaveValue('');

    // ...and the notice clears once the operator types something else.
    fireEvent.change(input, { target: { value: 'Gyara' } });
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('suppresses the same-named catalog subject while a legacy (-1) chip is still present', async () => {
    // The wart this guard closes: a -1 "Machamp" carries no portal id, so an
    // id-only comparison would still offer the catalog's 22210 "Machamp" and
    // leave two same-named entries on one campaign — one of which blocks push.
    // Repair has to come from the harvester baseline pull, not from adding the
    // catalog row alongside it.
    vi.mocked(api.listPSASubjects).mockResolvedValue({
      subjects: [{ id: 22210, name: 'Machamp' }, { id: 4807, name: 'Charizard' }],
      fetchedAt: '2026-08-01T00:00:00Z',
    });
    const onChange = renderEditor([{ id: -1, name: 'Machamp' }]);

    const input = await screen.findByPlaceholderText(/add a subject/i);
    fireEvent.change(input, { target: { value: 'cha' } });
    // Charizard proves the catalog loaded and the dropdown opened, so the
    // Machamp assertion below cannot pass vacuously.
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Charizard' })).toBeInTheDocument();
    });

    fireEvent.change(input, { target: { value: 'machamp' } });
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: 'Machamp' })).not.toBeInTheDocument();
    });
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(onChange).not.toHaveBeenCalled();
  });

  it('removes the confirmed subject, not a stale index, when value shifts mid-dialog', async () => {
    // The decision is asynchronous now, unlike window.confirm. If a parent
    // refetch reorders `value` while the dialog is open, a remembered index
    // points at a different subject — and removing the wrong one here is
    // exactly the silent mis-targeting this whole feature exists to prevent.
    vi.mocked(api.listPSASubjects).mockResolvedValue({
      subjects: [{ id: 22210, name: 'Machamp' }],
      fetchedAt: '2026-08-01T00:00:00Z',
    });
    const onChange = vi.fn();
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    function Harness() {
      const [value, setValue] = useState([
        { id: -1, name: 'Blastoise' },
        { id: 4807, name: 'Charizard' },
      ]);
      return (
        <>
          <button onClick={() => setValue([{ id: 99, name: 'Pikachu' }, ...value])}>prepend</button>
          <SubjectListEditor label="Subjects" value={value} onChange={onChange} />
        </>
      );
    }
    render(<QueryClientProvider client={qc}><Harness /></QueryClientProvider>);

    // Grabbed before the dialog opens: Radix marks outside content aria-hidden
    // while a modal is up, so it is unreachable by role query at that point.
    const prepend = screen.getByRole('button', { name: 'prepend' });

    // Blastoise is at index 0 when the dialog opens...
    fireEvent.click(screen.getByRole('button', { name: /remove blastoise/i }));
    await screen.findByRole('alertdialog');
    // ...and at index 1 by the time it is confirmed.
    fireEvent.click(prepend);
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }));

    expect(onChange).toHaveBeenCalledWith([{ id: 99, name: 'Pikachu' }, { id: 4807, name: 'Charizard' }]);
  });

  it('removes a normal chip without a confirmation dialog', () => {
    const onChange = renderEditor([{ id: 4807, name: 'Charizard' }]);

    fireEvent.click(screen.getByRole('button', { name: /remove charizard/i }));

    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
    expect(onChange).toHaveBeenCalledWith([]);
  });
});
