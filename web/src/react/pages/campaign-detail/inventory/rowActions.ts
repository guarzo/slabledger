import type { AgingItem } from '../../../../types/campaigns';
import { type ActionIntent, deriveActionIntent, canDismiss } from './inventoryCalcs';

// Single source of truth for row action labels. Both DesktopRow and MobileCard
// use these names — no more "Fix" vs "Fix Pricing", "Fix DH" vs "Fix DH Match".
export const ACTION_LABELS = {
  sell: 'Sell',
  setPrice: 'Set Price',
  fixPricing: 'Fix Pricing',
  fixDHMatch: 'Fix DH Match',
  removeDHMatch: 'Remove DH Match',
  retryDHMatch: 'Retry DH Match',
  listOnDH: 'List on DH',
  dismiss: 'Dismiss from DH',
  restore: 'Restore to DH',
  delete: 'Delete',
} as const;

export interface RowActionHandlers {
  onRecordSale: () => void;
  onSetPrice?: () => void;
  onFixPricing?: () => void;
  onFixDHMatch?: () => void;
  onUnmatchDH?: () => void;
  onRetryDHMatch?: () => void;
  onListOnDH?: (purchaseId: string) => void;
  onDismiss?: () => void;
  onUndismiss?: () => void;
  onDelete?: () => void;
}

export interface RowActionFlags {
  dhListingLoading?: boolean;
}

export interface ResolvedAction {
  key: string;
  label: string;
  onSelect: () => void;
  disabled?: boolean;
  /** Reversible actions skip confirm; destructive ones go through ConfirmDialog. */
  confirm?: { title: string; message: string; confirmLabel: string; variant?: 'danger' };
}

export interface ResolvedRowActions {
  primary: ResolvedAction | null;
  fallbackPrimary: ResolvedAction;
  overflow: ResolvedAction[];
}

// resolveRowActions is the single wiring point between a row component's props
// and the resolution rules below. Both DesktopRow and MobileCard used to
// hand-assemble the same ten-key handlers object; because nine of the ten keys
// are optional, adding an action to one view and forgetting the other still
// type-checked and the action simply vanished from that viewport (SLA-87).
//
// `source` is deliberately the row's whole props object rather than a
// re-assembled literal: DesktopRowProps and MobileCardProps both extend
// RowActionHandlers and RowActionFlags, so a new optional action added to those
// interfaces reaches both views without any per-view edit.
export function resolveRowActions(
  item: AgingItem,
  source: RowActionHandlers & RowActionFlags,
): ResolvedRowActions {
  const primary = resolveContextualPrimary(item, source, source);
  return {
    primary,
    fallbackPrimary: { key: 'sell', label: ACTION_LABELS.sell, onSelect: source.onRecordSale },
    overflow: resolveOverflowActions(item, source, source, primary),
  };
}

// resolveContextualPrimary returns the single recommended next action for a row,
// derived from the item's state. When this returns null, Sell is the row's
// primary affordance. When it returns an action, Sell demotes into the overflow.
export function resolveContextualPrimary(
  item: AgingItem,
  handlers: RowActionHandlers,
  flags: RowActionFlags,
): ResolvedAction | null {
  const intent: ActionIntent = deriveActionIntent(item);
  switch (intent) {
    case 'list':
      if (!handlers.onListOnDH) return null;
      return {
        key: 'list',
        label: flags.dhListingLoading ? 'Listing…' : ACTION_LABELS.listOnDH,
        disabled: !!flags.dhListingLoading,
        onSelect: () => handlers.onListOnDH?.(item.purchase.id),
      };
    case 'set_and_list':
      if (!handlers.onSetPrice) return null;
      return { key: 'set_and_list', label: ACTION_LABELS.setPrice, onSelect: handlers.onSetPrice };
    case 'fix_match':
      if (!handlers.onFixDHMatch) return null;
      return { key: 'fix_match', label: ACTION_LABELS.fixDHMatch, onSelect: handlers.onFixDHMatch };
    case 'restore':
      if (!handlers.onUndismiss) return null;
      return { key: 'restore', label: ACTION_LABELS.restore, onSelect: handlers.onUndismiss };
    case 'none':
      return null;
  }
}

// resolveOverflowActions builds the full overflow menu in display order.
// The contextual primary, when present, is hoisted above and excluded here.
export function resolveOverflowActions(
  item: AgingItem,
  handlers: RowActionHandlers,
  _flags: RowActionFlags,
  primary: ResolvedAction | null,
): ResolvedAction[] {
  const intent = deriveActionIntent(item);
  const showDismiss = canDismiss(intent);
  const out: ResolvedAction[] = [];

  // Sell becomes a regular menu item only when it's NOT the row's loud button —
  // i.e. when there is a contextual primary. Otherwise Sell is the row's
  // primary affordance and doesn't need a duplicate in the menu.
  if (primary) {
    out.push({ key: 'sell', label: ACTION_LABELS.sell, onSelect: handlers.onRecordSale });
  }

  if (handlers.onSetPrice && primary?.key !== 'set_and_list') {
    out.push({ key: 'setPrice', label: ACTION_LABELS.setPrice, onSelect: handlers.onSetPrice });
  }
  if (handlers.onFixPricing) {
    out.push({ key: 'fixPricing', label: ACTION_LABELS.fixPricing, onSelect: handlers.onFixPricing });
  }
  if (handlers.onFixDHMatch && primary?.key !== 'fix_match') {
    out.push({ key: 'fixDHMatch', label: ACTION_LABELS.fixDHMatch, onSelect: handlers.onFixDHMatch });
  }
  if (handlers.onUnmatchDH) {
    out.push({ key: 'removeDHMatch', label: ACTION_LABELS.removeDHMatch, onSelect: handlers.onUnmatchDH });
  }
  if (handlers.onRetryDHMatch) {
    out.push({ key: 'retryDHMatch', label: ACTION_LABELS.retryDHMatch, onSelect: handlers.onRetryDHMatch });
  }
  if (showDismiss && handlers.onDismiss) {
    // Dismiss is reversible (Restore brings it back), so no confirm.
    out.push({ key: 'dismiss', label: ACTION_LABELS.dismiss, onSelect: handlers.onDismiss });
  }
  if (handlers.onDelete) {
    // This confirm is the *only* prompt on the delete path — `handleDelete` used
    // to raise a second, native one on top of it. Naming the card and the sale
    // here is what let that redundant window.confirm go.
    out.push({
      key: 'delete',
      label: ACTION_LABELS.delete,
      onSelect: handlers.onDelete,
      confirm: {
        title: `Delete "${item.purchase.cardName || 'this card'}"?`,
        message: 'This permanently removes the purchase and any associated sale. This cannot be undone.',
        confirmLabel: 'Delete',
        variant: 'danger',
      },
    });
  }

  return out;
}
