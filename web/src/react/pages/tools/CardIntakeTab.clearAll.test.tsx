import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import CardIntakeTab from './CardIntakeTab';
import { api } from '../../../js/api';

vi.mock('../../../js/api', async (orig) => {
  const mod = await orig<typeof import('../../../js/api')>();
  return { ...mod, api: { ...mod.api, scanCert: vi.fn() } };
});

beforeEach(() => {
  localStorage.clear();
  (api.scanCert as ReturnType<typeof vi.fn>).mockResolvedValue({
    status: 'existing',
    cardName: 'Pikachu',
  });
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
});
