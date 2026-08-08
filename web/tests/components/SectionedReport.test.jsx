/**
 * Tests for SectionedReport's collapse state, which is persisted through the
 * shared `useLocalStorage` hook (SLA-51). The storage-key format is asserted
 * explicitly because it was inherited from a local helper this component used
 * to hand-roll — changing it would silently orphan users' collapsed sections.
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import SectionedReport from '../../src/react/components/advisor/SectionedReport';

const SCHEMA = [{ heading: 'Summary' }];
const MARKDOWN = '## Summary\n\nThe body text.\n';
const CACHE_KEY = 'advisor-run-1';
// SectionCard namespaces per section: `${cacheKey}:${normalize(heading)}`.
const STORAGE_KEY = `sectioned-report-collapsed:${CACHE_KEY}:summary`;

// tests/setup.js replaces localStorage with a no-op vi.fn stub that stores
// nothing. Persistence is the behavior under test here, so back the stub with a
// real in-memory store for this file.
const storage = window.localStorage;

function installStore() {
  const store = new Map();
  storage.getItem.mockImplementation(k => (store.has(k) ? store.get(k) : null));
  storage.setItem.mockImplementation((k, v) => { store.set(k, String(v)); });
  storage.removeItem.mockImplementation(k => { store.delete(k); });
  storage.clear.mockImplementation(() => { store.clear(); });
}

function renderReport() {
  return render(<SectionedReport markdown={MARKDOWN} schema={SCHEMA} cacheKey={CACHE_KEY} />);
}

describe('SectionedReport collapse persistence', () => {
  beforeEach(() => {
    installStore();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('renders expanded by default when nothing is stored', () => {
    renderReport();

    expect(screen.getByText('The body text.')).toBeInTheDocument();
    expect(storage.getItem(STORAGE_KEY)).toBeNull();
  });

  it('persists the collapsed state under the namespaced key', async () => {
    const user = userEvent.setup();
    renderReport();

    await user.click(screen.getByRole('button', { name: 'Collapse Summary' }));

    expect(screen.queryByText('The body text.')).not.toBeInTheDocument();
    expect(storage.getItem(STORAGE_KEY)).toBe('true');
  });

  it('restores collapsed state written before the hook was shared', () => {
    // The previous local helper wrote String(bool); the shared hook writes
    // JSON.stringify(bool). Both produce "true"/"false", so pre-existing values
    // must still round-trip.
    storage.setItem(STORAGE_KEY, 'true');

    renderReport();

    expect(screen.queryByText('The body text.')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Expand Summary' })).toBeInTheDocument();
  });

  it('expands again and writes false', async () => {
    storage.setItem(STORAGE_KEY, 'true');
    const user = userEvent.setup();
    renderReport();

    await user.click(screen.getByRole('button', { name: 'Expand Summary' }));

    expect(screen.getByText('The body text.')).toBeInTheDocument();
    expect(storage.getItem(STORAGE_KEY)).toBe('false');
  });
});
