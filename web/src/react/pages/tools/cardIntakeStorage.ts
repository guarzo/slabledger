import type { CertRow } from './cardIntakeTypes';

const STORAGE_KEY_PREFIX = 'intake:queue:';

export function storageKey(): string {
  const d = new Date();
  const yyyy = d.getFullYear();
  const mm = String(d.getMonth() + 1).padStart(2, '0');
  const dd = String(d.getDate()).padStart(2, '0');
  return `${STORAGE_KEY_PREFIX}${yyyy}-${mm}-${dd}`;
}

export function loadQueue(): Map<string, CertRow> {
  try {
    const raw = localStorage.getItem(storageKey());
    if (!raw) return new Map();
    const entries: [string, CertRow][] = JSON.parse(raw);
    const cleaned = entries.map(([k, v]): [string, CertRow] => [
      k,
      {
        ...v,
        ...(v.status === 'scanning' || v.status === 'importing'
          ? { status: v.purchaseId ? 'existing' as const : 'resolving' as const } : {}),
        // In-flight work and durable return state are reloaded from the server,
        // never resumed or acknowledged from yesterday's browser projection.
        returnBusy: false, returnLoading: false, returnState: undefined,
        returnStatusError: undefined, returnError: undefined, returnStale: false,
      },
    ]);
    return new Map(cleaned);
  } catch {
    return new Map();
  }
}

export function saveQueue(certs: Map<string, CertRow>) {
  try {
    if (certs.size === 0) {
      localStorage.removeItem(storageKey());
      return;
    }
    localStorage.setItem(storageKey(), JSON.stringify(Array.from(certs.entries())));
  } catch {
    // quota exceeded or disabled — non-fatal
  }
}
