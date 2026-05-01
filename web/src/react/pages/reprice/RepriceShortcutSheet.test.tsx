import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi } from 'vitest';
import RepriceShortcutSheet from './RepriceShortcutSheet';

describe('RepriceShortcutSheet', () => {
  it('renders nothing when closed', () => {
    render(<RepriceShortcutSheet open={false} onClose={vi.fn()} />);
    expect(screen.queryByText(/Keyboard shortcuts/i)).not.toBeInTheDocument();
  });

  it('renders all 8 binding rows when open', () => {
    render(<RepriceShortcutSheet open={true} onClose={vi.fn()} />);
    expect(screen.getByText(/Keyboard shortcuts/i)).toBeInTheDocument();
    // Spot-check key bindings
    expect(screen.getByText(/Next row/)).toBeInTheDocument();
    expect(screen.getByText(/Previous row/)).toBeInTheDocument();
    expect(screen.getByText(/Accept current row/)).toBeInTheDocument();
    expect(screen.getByText(/Toggle selection/)).toBeInTheDocument();
    expect(screen.getByText(/Blur input, clear focus, deselect/i)).toBeInTheDocument();
    expect(screen.getByText(/Focus first price input/)).toBeInTheDocument();
    expect(screen.getByText(/Show this sheet/)).toBeInTheDocument();
    expect(screen.getByText(/Open Apply confirm/)).toBeInTheDocument();
  });

  it('calls onClose when the close button is clicked', async () => {
    const onClose = vi.fn();
    render(<RepriceShortcutSheet open={true} onClose={onClose} />);
    await userEvent.click(screen.getByRole('button', { name: /Close/i }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
