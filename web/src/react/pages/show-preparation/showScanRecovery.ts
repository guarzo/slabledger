import type { ScanCommand } from '../../../types/showprep';
import { isScanCommand, isScanUUID } from '../../../js/api/showScanValidation';
import { AUTH_CONTEXT_RESET_EVENT, SHOW_SCAN_STORAGE_PREFIX, clearShowScanSessionEntries } from '../../utils/authContextEvents';

export interface PendingShowScan { readonly listId: string; readonly command: Readonly<ScanCommand> }
const PREFIX = SHOW_SCAN_STORAGE_PREFIX;
const key = (scope: string) => `${PREFIX}v1:${scope}`;
const failure = () => new Error('Show scan recovery unavailable in session storage. Resolve storage access before scanning.');
function validScans(value: unknown): value is PendingShowScan[] {
  return Array.isArray(value) && value.length <= 200 && new Set(value.map(scan => scan?.command?.commandId)).size === value.length
    && value.every(scan => scan && typeof scan === 'object' && Object.keys(scan).sort().join(',') === 'command,listId'
      && isScanUUID(scan.listId) && isScanCommand(scan.command));
}
export function readPendingScans(scope: string): PendingShowScan[] {
  if (!scope) return [];
  try {
    const raw = sessionStorage.getItem(key(scope));
    if (!raw) return [];
    const value = JSON.parse(raw);
    if (!value || value.version !== 1 || value.scope !== scope || Object.keys(value).sort().join(',') !== 'scans,scope,version' || !validScans(value.scans)) throw failure();
    return value.scans;
  } catch { throw failure(); }
}
export function writePendingScans(scope: string, scans: PendingShowScan[]): void {
  if (!scope || !validScans(scans)) throw failure();
  try {
    if (scans.length) sessionStorage.setItem(key(scope), JSON.stringify({ version: 1, scope, scans }));
    else sessionStorage.removeItem(key(scope));
  } catch { throw failure(); }
}
export function clearPendingScans(scope: string): void {
  try { sessionStorage.removeItem(key(scope)); } catch { throw failure(); }
}
// Listener is feature-local: AuthContext knows only the reset event, never this page.
window.addEventListener(AUTH_CONTEXT_RESET_EVENT, () => {
  try {
    clearShowScanSessionEntries();
  } catch {
    // Active owners also receive the event and block/discard work independently.
  }
});
