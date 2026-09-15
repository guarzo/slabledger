import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import type { AgingItem } from '../../../../types/campaigns';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import RealSelectionBar, { type InventorySelectionBarProps } from './InventorySelectionBar';

function InventorySelectionBar(props: Pick<InventorySelectionBarProps, 'selectedItems' | 'onRecordSale' | 'onListOnDH' | 'onClear' | 'disabled'>) {
  return <QueryClientProvider client={new QueryClient()}><MemoryRouter><RealSelectionBar {...props}
    selected={new Set(props.selectedItems.map(i => i.purchase.id))} selectedVersions={{}} evaluations={{}} onAdded={() => {}} /></MemoryRouter></QueryClientProvider>;
}

function makeItem(id: string, clValueCents: number | undefined): AgingItem {
  return {
    purchase: {
      id,
      campaignId: 'c1',
      cardName: `Card ${id}`,
      setName: 'Set',
      certNumber: id,
      grader: 'PSA',
      gradeValue: 10,
      cardNumber: id,
      buyCostCents: 1000,
      psaSourcingFeeCents: 0,
      clValueCents,
      frontImageUrl: '',
      purchaseDate: '2026-01-01',
      receivedAt: '2026-01-02',
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    },
    daysHeld: 10,
    campaignName: 'Campaign 1',
    currentMarket: undefined,
    signal: undefined,
    priceAnomaly: false,
  } as AgingItem;
}

afterEach(() => {
  cleanup();
});

describe('InventorySelectionBar', () => {
  it('retains missing selected IDs for review without opening empty financial forms', () => {
    const onRecordSale = vi.fn(); const onListOnDH = vi.fn();
    render(<QueryClientProvider client={new QueryClient()}><MemoryRouter><RealSelectionBar
      selected={new Set(['deleted'])} selectedItems={[]} selectedVersions={{ deleted: 'old' }} evaluations={{}}
      onAdded={() => {}} onClear={() => {}} onRecordSale={onRecordSale} onListOnDH={onListOnDH} /></MemoryRouter></QueryClientProvider>);
    expect(screen.getByText(/Review and reselect/)).toHaveTextContent('deleted');
    expect(screen.getByRole('button', { name: /Record sale/ })).toBeDisabled();
    expect(screen.getByRole('button', { name: /List on DH/ })).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: /Record sale/ }));
    expect(onRecordSale).not.toHaveBeenCalled(); expect(onListOnDH).not.toHaveBeenCalled();
  });
  it('renders nothing when selectedItems is empty', () => {
    const { container } = render(
      <InventorySelectionBar
        selectedItems={[]}
        onRecordSale={vi.fn()}
        onListOnDH={vi.fn()}
        onClear={vi.fn()}
      />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it('renders count and total list price summing clValueCents', () => {
    const items = [makeItem('1', 5000), makeItem('2', 6000), makeItem('3', undefined)];
    render(
      <InventorySelectionBar
        selectedItems={items}
        onRecordSale={vi.fn()}
        onListOnDH={vi.fn()}
        onClear={vi.fn()}
      />,
    );
    expect(screen.getByText(/3 selected/, { selector: 'span' })).toBeInTheDocument();
    expect(screen.getByText(/\$110\.00 list/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Record sale \(3\)/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /List on DH \(3\)/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^Clear$/ })).toBeInTheDocument();
  });

  it('omits the list-price segment when no item has a CL value', () => {
    const items = [makeItem('1', undefined), makeItem('2', undefined)];
    render(
      <InventorySelectionBar
        selectedItems={items}
        onRecordSale={vi.fn()}
        onListOnDH={vi.fn()}
        onClear={vi.fn()}
      />,
    );
    expect(screen.getByText(/2 selected/, { selector: 'span' })).toBeInTheDocument();
    expect(screen.queryByText(/\$.* list$/)).not.toBeInTheDocument();
  });

  it('invokes the matching callback when each button is clicked', () => {
    const onRecordSale = vi.fn();
    const onListOnDH = vi.fn();
    const onClear = vi.fn();
    render(
      <InventorySelectionBar
        selectedItems={[makeItem('1', 5000)]}
        onRecordSale={onRecordSale}
        onListOnDH={onListOnDH}
        onClear={onClear}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: /Record sale/ }));
    fireEvent.click(screen.getByRole('button', { name: /List on DH/ }));
    fireEvent.click(screen.getByRole('button', { name: /^Clear$/ }));
    expect(onRecordSale).toHaveBeenCalledTimes(1);
    expect(onListOnDH).toHaveBeenCalledTimes(1);
    expect(onClear).toHaveBeenCalledTimes(1);
  });

  it('clears selection on Escape when not disabled', () => {
    const onClear = vi.fn();
    render(
      <InventorySelectionBar
        selectedItems={[makeItem('1', 5000)]}
        onRecordSale={vi.fn()}
        onListOnDH={vi.fn()}
        onClear={onClear}
      />,
    );
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClear).toHaveBeenCalledTimes(1);
  });

  it('does not clear on Escape when disabled', () => {
    const onClear = vi.fn();
    render(
      <InventorySelectionBar
        selectedItems={[makeItem('1', 5000)]}
        onRecordSale={vi.fn()}
        onListOnDH={vi.fn()}
        onClear={onClear}
        disabled
      />,
    );
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClear).not.toHaveBeenCalled();
  });

  it('does not invoke any callback when buttons are clicked while disabled', () => {
    const onRecordSale = vi.fn();
    const onListOnDH = vi.fn();
    const onClear = vi.fn();
    render(
      <InventorySelectionBar
        selectedItems={[makeItem('1', 5000)]}
        onRecordSale={onRecordSale}
        onListOnDH={onListOnDH}
        onClear={onClear}
        disabled
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: /Record sale/ }));
    fireEvent.click(screen.getByRole('button', { name: /List on DH/ }));
    fireEvent.click(screen.getByRole('button', { name: /^Clear$/ }));
    expect(onRecordSale).not.toHaveBeenCalled();
    expect(onListOnDH).not.toHaveBeenCalled();
    expect(onClear).not.toHaveBeenCalled();
  });
});
