import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, act, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import CopyableCert from './CopyableCert';

describe('CopyableCert', () => {
  const writeText = vi.fn().mockResolvedValue(undefined);

  // userEvent.setup() installs its own jsdom clipboard stub (a getter on
  // navigator.clipboard), so our mock must be (re)applied after setup() runs
  // in each test, not in beforeEach.
  function stubClipboard() {
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    });
  }

  beforeEach(() => {
    writeText.mockClear();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('writes the raw cert number to the clipboard on click', async () => {
    const user = userEvent.setup();
    stubClipboard();
    render(<CopyableCert certNumber="12345678">Cert #12345678</CopyableCert>);
    await user.click(screen.getByRole('button'));
    expect(writeText).toHaveBeenCalledWith('12345678');
  });

  it('stops propagation so a parent click handler does not fire', async () => {
    const user = userEvent.setup();
    stubClipboard();
    const parentClick = vi.fn();
    render(
      <div onClick={parentClick}>
        <CopyableCert certNumber="12345678" />
      </div>,
    );
    await user.click(screen.getByRole('button'));
    expect(parentClick).not.toHaveBeenCalled();
  });

  it('renders nothing when certNumber is empty', () => {
    const { container } = render(<CopyableCert certNumber="" />);
    expect(container).toBeEmptyDOMElement();
  });

  it('does not throw when the clipboard write rejects', async () => {
    const user = userEvent.setup();
    stubClipboard();
    writeText.mockRejectedValueOnce(new Error('denied'));
    render(<CopyableCert certNumber="12345678" />);
    await user.click(screen.getByRole('button'));
    expect(writeText).toHaveBeenCalled();
  });

  it('shows a copied flash after a successful copy and clears it', async () => {
    vi.useFakeTimers();
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    });
    render(<CopyableCert certNumber="12345678">Cert #12345678</CopyableCert>);
    await act(async () => {
      fireEvent.click(screen.getByRole('button'));
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(screen.getByText(/copied/i)).toBeInTheDocument();
    act(() => { vi.advanceTimersByTime(1100); });
    expect(screen.queryByText(/copied/i)).not.toBeInTheDocument();
  });
});
