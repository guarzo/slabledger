import { beforeEach, expect, it, vi } from 'vitest';
import { clearPendingScans, readPendingScans, writePendingScans } from './showScanRecovery';
import { AUTH_CONTEXT_RESET_EVENT } from '../../utils/authContextEvents';
import { listId } from './fixtures.test-support';
const pending = { listId, command: { commandId: '55555555-5555-4555-8555-555555555555', certNumber: '00123AB', withdrawalGeneration: 4 } };
beforeEach(() => { sessionStorage.clear(); vi.restoreAllMocks(); });
it('persists immutable payloads under the authenticated user and never recovers another identity', () => {
  writePendingScans('7', [pending]); expect(readPendingScans('7')).toEqual([pending]); expect(readPendingScans('8')).toEqual([]);
  clearPendingScans('7'); expect(readPendingScans('7')).toEqual([]);
});
it.each([
  { version: 9, scope: '7', scans: [pending] }, { version: 1, scope: '8', scans: [pending] },
  { version: 1, scope: '7', scans: [{ ...pending, command: { ...pending.command, withdrawalGeneration: -1 } }] },
  { version: 1, scope: '7', scans: [{ ...pending, command: { ...pending.command, token: 'secret' } }] },
  { version: 1, scope: '7', scans: [pending, pending] },
])('fails closed on invalid recovery schema', envelope => {
  writePendingScans('7', [pending]); const key = sessionStorage.key(0)!; sessionStorage.setItem(key, JSON.stringify(envelope));
  expect(() => readPendingScans('7')).toThrow(/recovery/i);
});
it('clears every versioned show-scan entry on auth reset without clearing unrelated tab data', () => {
  writePendingScans('7', [pending]); writePendingScans('8', [pending]); sessionStorage.setItem('other-feature', 'keep');
  window.dispatchEvent(new Event(AUTH_CONTEXT_RESET_EVENT)); expect(readPendingScans('7')).toEqual([]); expect(readPendingScans('8')).toEqual([]);
  expect(sessionStorage.getItem('other-feature')).toBe('keep');
});
it('does not silently promise persistence when sessionStorage fails', () => {
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('quota'); });
  expect(() => writePendingScans('7', [pending])).toThrow(/recovery.*storage/i);
});
