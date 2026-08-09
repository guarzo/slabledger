import { describe, it, expect, vi } from 'vitest';
import { resolveRowActions, ACTION_LABELS, type RowActionHandlers, type RowActionFlags } from './rowActions';
import type { AgingItem, Purchase } from '../../../../types/campaigns';

type TestPurchase = Pick<Purchase,
  'id' | 'cardName' | 'gradeValue' | 'certNumber' | 'receivedAt' |
  'campaignId' | 'clValueCents' | 'buyCostCents' | 'psaSourcingFeeCents' | 'purchaseDate' |
  'createdAt' | 'updatedAt' | 'dhInventoryId' | 'dhStatus' | 'reviewedPriceCents' | 'dhPushStatus'
>;

function makeItem(purchaseOverrides?: Partial<TestPurchase>): AgingItem {
  const purchase: TestPurchase = {
    id: 'p1',
    cardName: 'Charizard',
    gradeValue: 9,
    buyCostCents: 1000,
    psaSourcingFeeCents: 0,
    purchaseDate: '2026-01-01',
    certNumber: 'PSA-1',
    receivedAt: undefined,
    campaignId: 'camp-1',
    clValueCents: 2000,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...purchaseOverrides,
  };
  return { purchase: purchase as AgingItem['purchase'], daysHeld: 0 };
}

/** Every optional handler wired, which is what a row gets when the parent
    supplies the full callback set. */
function allHandlers(): RowActionHandlers & RowActionFlags {
  return {
    onRecordSale: vi.fn(),
    onSetPrice: vi.fn(),
    onFixPricing: vi.fn(),
    onFixDHMatch: vi.fn(),
    onUnmatchDH: vi.fn(),
    onRetryDHMatch: vi.fn(),
    onListOnDH: vi.fn(),
    onDismiss: vi.fn(),
    onUndismiss: vi.fn(),
    onDelete: vi.fn(),
  };
}

// resolveRowActions is the single wiring point DesktopRow and MobileCard share.
// Before SLA-87 each view hand-assembled the same ten-key handlers object, and
// because nine of the ten keys are optional, an action added to one view and
// forgotten in the other type-checked fine and silently vanished from that
// viewport. Parity is now structural — both prop interfaces extend
// RowActionHandlers, so the compiler carries a new action to both — and these
// tests pin the behaviour that wiring produces.
describe('resolveRowActions', () => {
  it('always offers Sell as the fallback primary, wired to onRecordSale', () => {
    const handlers = allHandlers();
    const { fallbackPrimary } = resolveRowActions(makeItem(), handlers);

    expect(fallbackPrimary.key).toBe('sell');
    expect(fallbackPrimary.label).toBe(ACTION_LABELS.sell);
    fallbackPrimary.onSelect();
    expect(handlers.onRecordSale).toHaveBeenCalledOnce();
  });

  it('passes every wired handler through to the overflow menu', () => {
    // Received but unmatched → intent 'fix_match', which is the state that
    // exposes the widest overflow set (dismiss included).
    const item = makeItem({ receivedAt: '2026-02-01T00:00:00Z' });
    const { primary, overflow } = resolveRowActions(item, allHandlers());

    expect(primary?.key).toBe('fix_match');
    // Sell demotes into the menu whenever a contextual primary exists, and the
    // hoisted primary is not duplicated there.
    expect(overflow.map(a => a.key)).toEqual([
      'sell', 'setPrice', 'fixPricing', 'removeDHMatch', 'retryDHMatch', 'dismiss', 'delete',
    ]);
  });

  it('omits actions whose handler the parent did not supply', () => {
    const item = makeItem({ receivedAt: '2026-02-01T00:00:00Z' });
    const { primary, overflow } = resolveRowActions(item, { onRecordSale: vi.fn() });

    // No onFixDHMatch means the contextual primary cannot resolve; Sell stays
    // the loud button and the menu is empty rather than half-wired.
    expect(primary).toBeNull();
    expect(overflow).toEqual([]);
  });

  it('reads the loading flag off the same object as the handlers', () => {
    // 'list' intent: received, matched, priced, not yet listed.
    const item = makeItem({
      receivedAt: '2026-02-01T00:00:00Z',
      dhInventoryId: 1,
      reviewedPriceCents: 5000,
    });
    const { primary } = resolveRowActions(item, { ...allHandlers(), dhListingLoading: true });

    expect(primary?.key).toBe('list');
    expect(primary?.label).toBe('Listing…');
    expect(primary?.disabled).toBe(true);
  });
});
