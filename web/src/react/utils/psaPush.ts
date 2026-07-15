import type { PSAPushStatus } from '../../types/campaigns';

/** UI bucket for a push row's status. `pushed` (resolved) maps to null —
    no indicator, no special modal treatment. */
export type PushIndicatorState = 'pending' | 'inflight' | 'failed';

export function classifyPushStatus(status: PSAPushStatus): PushIndicatorState | null {
  switch (status) {
    case 'pending': return 'pending';
    case 'approved':
    case 'pushing': return 'inflight';
    case 'failed': return 'failed';
    default: return null; // pushed = resolved
  }
}
