import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import BulkSelectionMissingCLWarning from './BulkSelectionMissingCLWarning';

describe('BulkSelectionMissingCLWarning', () => {
  it('renders nothing when no missing-CL ids are present', () => {
    const { container } = render(
      <BulkSelectionMissingCLWarning
        missingCLIds={[]}
        selectedCount={5}
        onDeselect={vi.fn()}
        onHighlight={vi.fn()}
      />
    );
    expect(container.firstChild).toBeNull();
  });

  it('shows "{n} of {total} cards have no CL value" when ids are present', () => {
    render(
      <BulkSelectionMissingCLWarning
        missingCLIds={['a', 'b']}
        selectedCount={3}
        onDeselect={vi.fn()}
        onHighlight={vi.fn()}
      />
    );
    expect(screen.getByText(/2 of 3 cards have no CL value/i)).toBeInTheDocument();
  });

  it('Deselect button passes the missing-CL ids to onDeselect', () => {
    const onDeselect = vi.fn();
    render(
      <BulkSelectionMissingCLWarning
        missingCLIds={['a', 'b']}
        selectedCount={3}
        onDeselect={onDeselect}
        onHighlight={vi.fn()}
      />
    );
    fireEvent.click(screen.getByRole('button', { name: /Deselect/i }));
    expect(onDeselect).toHaveBeenCalledWith(['a', 'b']);
  });

  it('Highlight button passes the missing-CL ids to onHighlight', () => {
    const onHighlight = vi.fn();
    render(
      <BulkSelectionMissingCLWarning
        missingCLIds={['a', 'b']}
        selectedCount={3}
        onDeselect={vi.fn()}
        onHighlight={onHighlight}
      />
    );
    fireEvent.click(screen.getByRole('button', { name: /Highlight/i }));
    expect(onHighlight).toHaveBeenCalledWith(['a', 'b']);
  });
});
