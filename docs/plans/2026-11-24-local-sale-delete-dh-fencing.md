# Unblock local sale and delete from remote-only DH uncertainty

## Problem

Recording a sale (`SaleStore.CreateSale`) and deleting a purchase
(`PurchaseStore.DeletePurchase`) both go through `withLocalPurchaseMutation` →
`ConfirmedReturnStore.assertAllowed`, the same fence used before DH writes
(introduced in #715). That fence refuses on:

1. **any** open `dh_mutation_attempts` row for the card (`preceding_dh_mutation_uncertain`), whatever its kind;
2. a retained return episode whose target (dh_inventory_id/cert/grader) no longer matches the purchase (`identity_conflict`).

#740 added a warned delete, but only for open `list/intake_patch_sync` retryable attempts.

Two production cards are stuck with no user way out (neither ever reached DH —
`dhInventoryId` null, `dhPushStatus` pending):

| Cert | Card | Blocker |
|---|---|---|
| 173686392 | Infernape Lv.X DP10 PSA 7 | open attempt `push/psa_import` |
| 170407390 | Sabrina's Gengar Holo | retained return episode names a DH inventory id; purchase has none → `identity_conflict` |

## Decision (approved by user 2026-11-24): allow both sale and delete

Local sale and local delete change only SlabLedger's books. Uncertainty about
remote DH state must not block them, except where it could double-record or
reverse a sale.

### Sale (`CreateSale`)
- Still refuse if an open attempt could create, settle or reverse a sale:
  kinds `sale`, `void`, `return`, `return_to_stock`, `recover_void_reset_delete`
  (confirm the full list from the code; anything financial stays blocking).
- Exception (added after the Regigigas 69007166 un-sell): an open `sale/record`
  attempt blocks only while its local sale row exists. After un-sell the row is
  gone and the attempt is remote-only uncertainty, so a new local sale and a
  warned delete go through; the DH sale path stays fenced behind it.
- Still refuse if a return episode is **in progress** (state != completed).
- Do **not** refuse on open `push` / `list` / `price` / `live_price` / `link`
  etc. attempts, nor on a *completed* return episode whose target doesn't match.
- Leave the attempts open; DH-side reconciliation stays fenced as before.

### Delete (`DeletePurchase`)
- Generalise the #740 warned delete: when the only blockers are non-financial
  open attempts and/or a completed-but-mismatched return episode, require the
  existing confirmation (`WithFailedListingDeleteConfirmation` / existing error
  code so the current frontend flow works), then mark those attempts
  `abandoned` with an honest receipt and delete.
- Financial attempts / in-progress returns still block, unwarnable.
- Check FKs from `confirmed_dh_returns` / `dh_mutation_attempts` to the purchase.

### Unchanged
Every path that writes to DH (`AssertMutationAllowed`, push/list/price/sale-sync,
`execDHMutation`, campaign delete) keeps the full fence.

## Tests
- open `push/psa_import` attempt: sale succeeds; warned delete succeeds (unwarned delete returns the confirmation-required conflict)
- completed return episode with mismatched target: sale succeeds; warned delete succeeds
- open `sale` attempt: sale and delete both still refused
- in-progress return episode: still refused
- DH-mutation path with an open push attempt: still refused

## Frontend
Delete confirmation copy currently speaks of a failed listing; make it generic
("DH state for this card is uncertain; DH may still have it listed") if needed.

## After deploy
Record Infernape 173686392 sale: 2026-11-20, inperson, comp $360, 85% → $306.
