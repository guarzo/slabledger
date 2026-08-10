import { render, screen, waitFor, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import CardIntakeTab from './CardIntakeTab';
import { api } from '../../../js/api';
import { AUTO_RETRY_DELAYS_MS, MAX_AUTO_RETRIES } from './cardIntakeRetry';

// See CardIntakeTab.clearAll.test.tsx for why fetchWithRetry is the choke point
// being stubbed rather than spreading the `api` singleton.
beforeEach(() => {
  localStorage.clear();
  vi.spyOn(api, 'fetchWithRetry').mockRejectedValue(
    new Error('unstubbed API call — add a vi.spyOn for this endpoint'),
  );
  vi.spyOn(api, 'scanCert').mockResolvedValue({ status: 'new' });
  vi.spyOn(api, 'resolveCert').mockResolvedValue({
    certNumber: '12345678',
    cardName: 'Umbreon VMAX',
    grade: 10,
    year: '2021',
    category: 'EVOLVING SKIES',
    subject: 'Umbreon VMAX',
  });
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();
});

// Scan one cert and stage it, then press Import once.
async function stageAndImport(user: ReturnType<typeof userEvent.setup>) {
  render(<CardIntakeTab />);
  const input = screen.getByPlaceholderText(/Scan or type cert number/i);
  await user.type(input, '12345678{enter}');
  await screen.findByText(/1 new cert staged/i);
  await user.click(screen.getByRole('button', { name: /Import 1 New/i }));
}

const transientFailure = {
  imported: 0,
  alreadyExisted: 0,
  soldExisting: 0,
  failed: 1,
  errors: [{ certNumber: '12345678', error: 'PSA circuit open', retryable: true }],
};

const success = {
  imported: 1,
  alreadyExisted: 0,
  soldExisting: 0,
  failed: 0,
  errors: [],
};

// The headline behaviour: a transient PSA failure recovers on its own. The old
// screen told the operator to click Import again (SLA-108).
it('retries a transient failure automatically, with no second click', async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  vi.useFakeTimers({ shouldAdvanceTime: true });

  const importCerts = vi.spyOn(api, 'importCerts')
    .mockResolvedValueOnce(transientFailure)
    .mockResolvedValueOnce(success);

  await stageAndImport(user);

  // First pass failed and the screen promises to handle it itself.
  const pending = await screen.findByText(/Retrying automatically in/i);
  expect(pending).toHaveTextContent(/attempt 1 of 3/);
  expect(pending).not.toHaveTextContent(/click|press/i);
  expect(importCerts).toHaveBeenCalledTimes(1);

  await act(async () => { vi.advanceTimersByTime(AUTO_RETRY_DELAYS_MS[0]); });

  await waitFor(() => expect(importCerts).toHaveBeenCalledTimes(2));
  // Second pass succeeded: the failure message is gone and nothing is staged.
  await waitFor(() => expect(screen.queryByText(/Retrying automatically in/i)).toBeNull());
});

// If the staged rows are gone by the time the timer fires, the episode must end
// honestly rather than leaving "Retrying automatically in Ns" on screen
// describing a timer that no longer exists — the same dishonest message this
// change exists to remove. Clear all already handles its own case; dismissing
// the last retry row individually is the path that reaches the import handler
// with nothing to send.
it('ends the episode when the staged rows are dismissed before a retry fires', async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  vi.useFakeTimers({ shouldAdvanceTime: true });

  const importCerts = vi.spyOn(api, 'importCerts').mockResolvedValue(transientFailure);

  await stageAndImport(user);
  await screen.findByText(/Retrying automatically in/i);

  // Drop the only staged row while the retry is still pending.
  await user.click(screen.getByRole('button', { name: /dismiss/i }));

  await act(async () => { vi.advanceTimersByTime(AUTO_RETRY_DELAYS_MS[0]); });

  // No second import was attempted, and no stale promise is left on screen.
  expect(importCerts).toHaveBeenCalledTimes(1);
  await waitFor(() => expect(screen.queryByText(/Retrying automatically in/i)).toBeNull());
});

it('gives up honestly after the retry budget is spent', async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  vi.useFakeTimers({ shouldAdvanceTime: true });

  const importCerts = vi.spyOn(api, 'importCerts').mockResolvedValue(transientFailure);

  await stageAndImport(user);

  for (const delay of AUTO_RETRY_DELAYS_MS) {
    await screen.findByText(/Retrying automatically in/i);
    await act(async () => { vi.advanceTimersByTime(delay); });
  }

  // Initial attempt + one per funded retry, and no more.
  await waitFor(() => expect(importCerts).toHaveBeenCalledTimes(MAX_AUTO_RETRIES + 1));

  const exhausted = await screen.findByText(/still unavailable after 3 automatic retries/i);
  expect(exhausted).toHaveTextContent(/nothing was lost/i);
  expect(exhausted).toHaveTextContent(/press Import/i);

  // No fourth retry is lurking behind the exhausted message.
  await act(async () => { vi.advanceTimersByTime(10 * 60_000); });
  expect(importCerts).toHaveBeenCalledTimes(MAX_AUTO_RETRIES + 1);
});

it('retries a whole-batch failure too', async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  vi.useFakeTimers({ shouldAdvanceTime: true });

  const importCerts = vi.spyOn(api, 'importCerts')
    .mockRejectedValueOnce(new Error('Network request failed'))
    .mockResolvedValueOnce(success);

  await stageAndImport(user);

  const pending = await screen.findByText(/Network request failed/i);
  expect(pending).toHaveTextContent(/Retrying automatically in/i);

  await act(async () => { vi.advanceTimersByTime(AUTO_RETRY_DELAYS_MS[0]); });
  await waitFor(() => expect(importCerts).toHaveBeenCalledTimes(2));
});

// A permanent failure must not be retried at all — the backend already told us
// it will fail identically every time.
it('does not retry a permanent failure', async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  vi.useFakeTimers({ shouldAdvanceTime: true });

  const importCerts = vi.spyOn(api, 'importCerts').mockResolvedValue({
    imported: 0,
    alreadyExisted: 0,
    soldExisting: 0,
    failed: 1,
    errors: [{ certNumber: '12345678', error: 'cert not found', retryable: false }],
  });

  await stageAndImport(user);

  await waitFor(() => expect(importCerts).toHaveBeenCalledTimes(1));
  await act(async () => { vi.advanceTimersByTime(10 * 60_000); });

  expect(importCerts).toHaveBeenCalledTimes(1);
  expect(screen.queryByText(/Retrying automatically in/i)).toBeNull();
});
