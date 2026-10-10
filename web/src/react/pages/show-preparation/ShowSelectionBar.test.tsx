import { useState } from 'react';
import { expect, it } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import InventorySelectionBar from '../campaign-detail/inventory/InventorySelectionBar';
import { inventoryItem, purchaseId } from './fixtures.test-support';
function mount(disabled = false, outsideView = 0) {
  function Harness() {
    const [selected, setSelected] = useState(new Set([purchaseId]));
    return <InventorySelectionBar selectedItems={selected.size ? [inventoryItem()] : []} selected={selected} disabled={disabled}
      onClear={() => setSelected(new Set())} onRecordSale={() => {}} onListOnDH={() => {}} outsideView={outsideView} onReveal={() => {}} />;
  }
  render(<Harness />);
}
it('has financial selection actions but no show destination or Add entry point', () => {
  mount(); expect(screen.getByRole('button', { name: 'Record sale (1)' })).toBeEnabled(); expect(screen.getByRole('button', { name: 'List on DH (1)' })).toBeEnabled();
  expect(screen.queryByRole('button', { name: /Add.*show/ })).not.toBeInTheDocument(); expect(screen.queryByLabelText('Show list')).not.toBeInTheDocument();
  fireEvent.keyDown(window, { key: 'Escape' }); expect(screen.queryByRole('region', { name: 'Bulk actions for selected cards' })).not.toBeInTheDocument();
});
it.each(['disabled', 'dialog', 'prevented'])('retains financial selection when Escape belongs to %s', priority => {
  mount(priority === 'disabled');
  const dialog = document.createElement('div'); if (priority === 'dialog') { dialog.setAttribute('role', 'dialog'); document.body.append(dialog); }
  const event = new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }); if (priority === 'prevented') event.preventDefault(); fireEvent(window, event);
  expect(screen.getByRole('region', { name: 'Bulk actions for selected cards' })).toBeVisible(); dialog.remove();
});
it('keeps hidden selections recoverable without a show consent requirement', () => {
  mount(false, 1); expect(screen.getByText(/1 selected outside this view/)).toBeVisible(); expect(screen.getByRole('button', { name: 'Reveal selected' })).toBeEnabled();
});
