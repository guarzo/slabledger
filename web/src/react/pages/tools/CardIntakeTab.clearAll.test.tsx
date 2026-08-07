import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import CardIntakeTab from './CardIntakeTab';
import { api } from '../../../js/api';

// Spying on the real singleton rather than spreading it: `api` is an APIClient
// instance whose endpoint methods live on the prototype, so `{ ...mod.api }`
// copies none of them and every endpoint this file does not name becomes
// undefined. Stubbing fetchWithRetry — the single choke point behind
// get/post/put/deleteResource — keeps an unstubbed endpoint off the network.
beforeEach(() => {
  localStorage.clear();
  vi.spyOn(api, 'fetchWithRetry').mockRejectedValue(
    new Error('unstubbed API call — add a vi.spyOn for this endpoint'),
  );
  vi.spyOn(api, 'scanCert').mockResolvedValue({
    status: 'existing',
    cardName: 'Pikachu',
  });
});

afterEach(() => {
  vi.restoreAllMocks();
});

it('Clear all wipes every scanned row after confirmation', async () => {
  const user = userEvent.setup();
  render(<CardIntakeTab />);

  const input = screen.getByPlaceholderText(/Scan or type cert number/i);
  await user.type(input, '12345678{enter}');

  // Row appears and status bar shows "1 scanned"
  await waitFor(() => expect(screen.getByText(/1 scanned/)).toBeInTheDocument());

  // Open Clear all → confirm dialog
  await user.click(screen.getByRole('button', { name: /clear all/i }));
  const dialog = await screen.findByRole('alertdialog');
  await user.click(within(dialog).getByRole('button', { name: /clear all/i }));

  // Status bar (and its "scanned" count) is gone — list empty
  await waitFor(() => expect(screen.queryByText(/scanned/)).toBeNull());

  // Scan input regains focus so the next cert can be typed immediately
  await waitFor(() => expect(input).toHaveFocus());
});
