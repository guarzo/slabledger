/** Feature owners discard session-scoped work before logout or identity replacement. */
export const AUTH_CONTEXT_RESET_EVENT = 'slabledger:auth-context-reset';
export const SHOW_SCAN_STORAGE_PREFIX = 'slabledger:show-scans:';

export function clearShowScanSessionEntries(): void {
  const keys = Array.from({ length: sessionStorage.length }, (_, index) => sessionStorage.key(index));
  for (const key of keys) if (key?.startsWith(SHOW_SCAN_STORAGE_PREFIX)) sessionStorage.removeItem(key);
}
/** Eager auth utility also covers reset before a lazy feature has been loaded. */
export function resetAuthContext(): void {
  try { clearShowScanSessionEntries(); }
  catch { /* Active owners still discard in-memory commands on the event. */ }
  finally { window.dispatchEvent(new Event(AUTH_CONTEXT_RESET_EVENT)); }
}
