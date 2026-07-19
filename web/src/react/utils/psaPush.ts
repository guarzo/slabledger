import type { PSAPushStatus } from '../../types/campaigns';
import type { StatusTone } from '../ui/StatusPill';

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

/**
 * Full campaign-level PSA sync state: whether the campaign is linked to a
 * portal campaign at all, layered with any in-progress push. Every campaign
 * has exactly one of these — there is no "nothing to show" state, so a
 * healthy sync and an unlinked campaign never look the same at a glance.
 */
export type SyncState = 'not-linked' | 'synced' | PushIndicatorState;

export function syncState(isLinked: boolean, push: PSAPushStatus | null | undefined): SyncState {
  const pushState = push ? classifyPushStatus(push) : null;
  if (pushState) return pushState;
  return isLinked ? 'synced' : 'not-linked';
}

export const SYNC_LABELS: Record<SyncState, string> = {
  'not-linked': 'Not on PSA',
  synced: 'Synced',
  pending: 'Pending',
  inflight: 'Pushing',
  failed: 'Failed',
};

export const SYNC_TONES: Record<SyncState, StatusTone> = {
  'not-linked': 'neutral',
  synced: 'success',
  pending: 'warning',
  inflight: 'info',
  failed: 'danger',
};
