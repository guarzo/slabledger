-- Repair purchases that were marked sold locally but later had their
-- campaign_sales row deleted (return / un-sell). Before this migration,
-- DeleteSaleByPurchaseID only removed the sales row, leaving dh_status='sold'
-- and a stale dh_inventory_id. Those rows landed in the "Pending DH Listing"
-- tab but the List action 409'd with "Purchase is not in_stock on DH".
--
-- Reset the DH linkage on any purchase whose campaign_sales row is gone but
-- whose dh_status is still 'sold', mirroring ResetDHFieldsForRepushDueToDelete.
-- The push scheduler will re-enroll these on its next cycle. dh_unlisted_detected_at
-- is stamped so the UI badges them as "removed from DH — will be re-pushed".
UPDATE campaign_purchases
SET dh_inventory_id = 0,
    dh_push_status = 'pending',
    dh_push_attempts = 0,
    dh_status = '',
    dh_listing_price_cents = 0,
    dh_channels_json = '[]',
    dh_hold_reason = '',
    dh_unlisted_detected_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE dh_status = 'sold'
  AND NOT EXISTS (
    SELECT 1 FROM campaign_sales cs WHERE cs.purchase_id = campaign_purchases.id
  );
