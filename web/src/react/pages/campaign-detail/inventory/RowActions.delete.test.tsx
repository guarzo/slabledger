import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import RowActions from './RowActions';
import { resolveRowActions } from './rowActions';
import { api } from '../../../../js/api';
import type { AgingItem, ConfirmedReturnState } from '../../../../types/campaigns';

vi.mock('../../../../js/api', () => ({ api: { getConfirmedReturnState: vi.fn() } }));
afterEach(() => { cleanup(); vi.resetAllMocks(); });
const warning = 'A DH listing update for this item failed. DH may still have it listed; check DH after deleting.';
const item = { purchase: { id: 'p', cardName: 'Card', campaignId: 'c' }, daysHeld: 0 } as AgingItem;

function showDelete(variant: 'desktop' | 'mobile' = 'desktop') {
  const onDelete = vi.fn();
  const action = resolveRowActions(item, { onRecordSale: vi.fn(), onDelete }).overflow.find(a => a.key === 'delete')!;
  render(<RowActions primary={action} fallbackPrimary={action} overflow={[]} variant={variant} />);
  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
  return onDelete;
}

describe.each(['desktop', 'mobile'] as const)('Delete warning (%s)', variant => {
  it('uses the existing confirmation and sends acknowledgement only after confirm', async () => {
    vi.mocked(api.getConfirmedReturnState).mockResolvedValue({ failedListingDeleteAllowed: true } as ConfirmedReturnState);
    const onDelete = showDelete(variant);
    expect(await screen.findByText(new RegExp(warning.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')))).toBeInTheDocument();
    expect(api.getConfirmedReturnState).toHaveBeenCalledWith('p');
    expect(onDelete).not.toHaveBeenCalled();
    const buttons = screen.getAllByRole('button', { name: 'Delete' });
    fireEvent.click(buttons[buttons.length - 1]);
    expect(onDelete).toHaveBeenCalledExactlyOnceWith(true);
  });

  it('cancel sends nothing', async () => {
    vi.mocked(api.getConfirmedReturnState).mockResolvedValue({ failedListingDeleteAllowed: true } as ConfirmedReturnState);
    const onDelete = showDelete(variant);
    fireEvent.click(await screen.findByRole('button', { name: 'Cancel' }));
    expect(onDelete).not.toHaveBeenCalled();
  });

  it.each([false, 'error'])('keeps ordinary deletion without consent when preview is %s', async preview => {
    if (preview === 'error') vi.mocked(api.getConfirmedReturnState).mockRejectedValue(new Error('unavailable'));
    else vi.mocked(api.getConfirmedReturnState).mockResolvedValue({} as ConfirmedReturnState);
    const onDelete = showDelete(variant);
    await screen.findByText('This permanently removes the purchase and any associated sale. This cannot be undone.');
    expect(screen.queryByText(warning)).not.toBeInTheDocument();
    const buttons = screen.getAllByRole('button', { name: 'Delete' });
    fireEvent.click(buttons[buttons.length - 1]);
    expect(onDelete).toHaveBeenCalledExactlyOnceWith(false);
  });
});
