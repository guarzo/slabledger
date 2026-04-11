-- =============================================================================
-- LCS Bulk Sale — 2026-04-11
-- =============================================================================
--
-- Context:
--   All non-vaulted PSA-graded cards were sold to a local card shop (LCS) at
--   72% of Card Ladder comp value. The LCS did not provide an itemized receipt.
--   Cards were identified by cross-referencing the Shopify product export
--   (cards on hand) against already-sold purchases in the database.
--
-- What this does:
--   1. Deletes any CGC-graded purchases (and cascaded sales)
--   2. Inserts 163 sale records for unsold purchases matching the Shopify cert
--      list, at 72% of cl_value_cents, channel=inperson, 0% fees
--
-- Usage:
--   # DRY RUN — preview only, no changes (comment out BEGIN/COMMIT/INSERT/DELETE):
--   sqlite3 data/slabledger.db < scripts/lcs-bulk-sale-2026-04-11.sql
--
--   # EXECUTE — make sure to back up first:
--   cp data/slabledger.db data/slabledger.db.bak
--   sqlite3 data/slabledger.db < scripts/lcs-bulk-sale-2026-04-11.sql
--
-- Rollback:
--   DELETE FROM campaign_sales
--   WHERE sale_date = '2026-04-11' AND sale_channel = 'inperson';
--
-- =============================================================================

.mode column
.headers on
PRAGMA foreign_keys = ON;

-- ---------------------------------------------------------------------------
-- Step 0: Preview
-- ---------------------------------------------------------------------------

SELECT '=== PREVIEW: Cards to sell ===' AS step;
SELECT COUNT(*) AS card_count,
       SUM(p.cl_value_cents) / 100.0 AS total_cl_value_usd,
       SUM(CAST(ROUND(p.cl_value_cents * 0.72) AS INTEGER)) / 100.0 AS total_sale_price_usd,
       SUM(p.buy_cost_cents + p.psa_sourcing_fee_cents) / 100.0 AS total_cost_basis_usd,
       SUM(CAST(ROUND(p.cl_value_cents * 0.72) AS INTEGER) - p.buy_cost_cents - p.psa_sourcing_fee_cents) / 100.0 AS total_net_profit_usd
FROM campaign_purchases p
LEFT JOIN campaign_sales s ON s.purchase_id = p.id
WHERE s.id IS NULL
  AND p.was_refunded = 0
  AND p.cl_value_cents > 0
  AND p.cert_number IN ('05442200','100498352','114377551','114572935','115278920','115278924','115278942','116320466','120887274','124156370','125552854','126147045','127785538','128737957','129586221','129663417','129663673','130352505','130604469','130686546','130727299','131926105','132563403','132662256','132964508','133478793','133652871','133785822','134047364','134193968','134213350','134321543','134604989','134761258','134907885','134907889','134907899','135794903','136134864','137222981','137302676','137354056','137618102','138614706','138683277','139108110','139288930','139288933','139288935','139288939','139288941','139288942','139288951','139288953','139288954','139288955','139288956','139288957','139288960','139288961','139288963','139288964','139288965','139288966','139288967','139288974','139288981','139288985','139289001','139289016','139289018','139289021','139402294','139450443','140052753','140824770','140860476','140860480','141069953','141069954','141069955','141069956','141191045','141376899','141376908','141438008','141545724','141639648','141758747','142156548','142156550','142917771','143050982','143165422','143210177','143257869','143257872','143885730','144108595','144121955','144121961','144121965','144121967','144121973','144121977','144121979','144121983','144121990','144121995','144121998','144122000','144248404','144288941','144600265','144600266','145139452','145139495','145294967','145334813','145455452','145525829','145652009','145853860','145982182','146025542','147111444','147147367','147240361','147364343','147833274','148068955','148366798','148575206','148823392','148832830','148989725','148989726','149188834','149188835','149188836','149188838','149350651','149668533','150019466','150151003','151061567','152295612','152295615','152420095','153514148','17979513','191055543','193721788','41317523','47465934','47465939','54996535','6008861228','6009311053','6014654132','6025976243','6027449263','6031192251','6032733298','6034456056','6034572040','6034611283','6046531265','6050018038','6050108272','6052121054','6052481028','6056527012','6056527019','6056527093','6056527116','6059337203','6063570134','6064589234','6065892156','6066919038','6075168176','6098919001','68615871','77036851','82632702','85755647','86452748','87257901','97901008');

SELECT '=== PREVIEW: CGC cards to delete ===' AS step;
SELECT COUNT(*) AS cgc_card_count FROM campaign_purchases WHERE grader = 'CGC';

-- ---------------------------------------------------------------------------
-- Step 1: Execute
-- ---------------------------------------------------------------------------

BEGIN TRANSACTION;

-- 1a. Delete CGC purchases (ON DELETE CASCADE removes any related sales)
DELETE FROM campaign_purchases WHERE grader = 'CGC';

-- 1b. Insert sale records for 163 LCS cards
INSERT INTO campaign_sales (
    id,
    purchase_id,
    sale_channel,
    sale_price_cents,
    sale_fee_cents,
    sale_date,
    days_to_sell,
    net_profit_cents,
    snapshot_json,
    created_at,
    updated_at,
    original_list_price_cents,
    price_reductions,
    days_listed,
    sold_at_asking_price,
    was_cracked,
    order_id
)
SELECT
    -- Generate UUID v4
    lower(
        hex(randomblob(4)) || '-' ||
        hex(randomblob(2)) || '-4' ||
        substr(hex(randomblob(2)), 2) || '-' ||
        substr('89ab', abs(random()) % 4 + 1, 1) ||
        substr(hex(randomblob(2)), 2) || '-' ||
        hex(randomblob(6))
    )                                                                   AS id,
    p.id                                                                AS purchase_id,
    'inperson'                                                          AS sale_channel,
    CAST(ROUND(p.cl_value_cents * 0.72) AS INTEGER)                     AS sale_price_cents,
    0                                                                   AS sale_fee_cents,
    '2026-04-11'                                                        AS sale_date,
    CAST(julianday('2026-04-11') - julianday(p.purchase_date) AS INTEGER) AS days_to_sell,
    CAST(ROUND(p.cl_value_cents * 0.72) AS INTEGER)
        - p.buy_cost_cents
        - p.psa_sourcing_fee_cents                                      AS net_profit_cents,
    ''                                                                  AS snapshot_json,
    datetime('now')                                                     AS created_at,
    datetime('now')                                                     AS updated_at,
    0                                                                   AS original_list_price_cents,
    0                                                                   AS price_reductions,
    0                                                                   AS days_listed,
    0                                                                   AS sold_at_asking_price,
    0                                                                   AS was_cracked,
    ''                                                                  AS order_id
FROM campaign_purchases p
LEFT JOIN campaign_sales s ON s.purchase_id = p.id
WHERE s.id IS NULL
  AND p.was_refunded = 0
  AND p.cl_value_cents > 0
  AND p.cert_number IN ('05442200','100498352','114377551','114572935','115278920','115278924','115278942','116320466','120887274','124156370','125552854','126147045','127785538','128737957','129586221','129663417','129663673','130352505','130604469','130686546','130727299','131926105','132563403','132662256','132964508','133478793','133652871','133785822','134047364','134193968','134213350','134321543','134604989','134761258','134907885','134907889','134907899','135794903','136134864','137222981','137302676','137354056','137618102','138614706','138683277','139108110','139288930','139288933','139288935','139288939','139288941','139288942','139288951','139288953','139288954','139288955','139288956','139288957','139288960','139288961','139288963','139288964','139288965','139288966','139288967','139288974','139288981','139288985','139289001','139289016','139289018','139289021','139402294','139450443','140052753','140824770','140860476','140860480','141069953','141069954','141069955','141069956','141191045','141376899','141376908','141438008','141545724','141639648','141758747','142156548','142156550','142917771','143050982','143165422','143210177','143257869','143257872','143885730','144108595','144121955','144121961','144121965','144121967','144121973','144121977','144121979','144121983','144121990','144121995','144121998','144122000','144248404','144288941','144600265','144600266','145139452','145139495','145294967','145334813','145455452','145525829','145652009','145853860','145982182','146025542','147111444','147147367','147240361','147364343','147833274','148068955','148366798','148575206','148823392','148832830','148989725','148989726','149188834','149188835','149188836','149188838','149350651','149668533','150019466','150151003','151061567','152295612','152295615','152420095','153514148','17979513','191055543','193721788','41317523','47465934','47465939','54996535','6008861228','6009311053','6014654132','6025976243','6027449263','6031192251','6032733298','6034456056','6034572040','6034611283','6046531265','6050018038','6050108272','6052121054','6052481028','6056527012','6056527019','6056527093','6056527116','6059337203','6063570134','6064589234','6065892156','6066919038','6075168176','6098919001','68615871','77036851','82632702','85755647','86452748','87257901','97901008');

COMMIT;

-- ---------------------------------------------------------------------------
-- Step 2: Verify
-- ---------------------------------------------------------------------------

SELECT '=== RESULTS ===' AS step;

SELECT COUNT(*) AS sales_created
FROM campaign_sales
WHERE sale_date = '2026-04-11' AND sale_channel = 'inperson';

SELECT SUM(sale_price_cents) / 100.0 AS total_sale_price_usd,
       SUM(net_profit_cents) / 100.0 AS total_net_profit_usd,
       MIN(days_to_sell)              AS min_days_to_sell,
       MAX(days_to_sell)              AS max_days_to_sell,
       ROUND(AVG(days_to_sell))       AS avg_days_to_sell
FROM campaign_sales
WHERE sale_date = '2026-04-11' AND sale_channel = 'inperson';

SELECT '=== TOP 5 BY SALE PRICE ===' AS step;
SELECT p.cert_number,
       substr(p.card_name, 1, 40)     AS card_name,
       p.cl_value_cents / 100.0       AS cl_value_usd,
       s.sale_price_cents / 100.0     AS sale_price_usd,
       p.buy_cost_cents / 100.0       AS cost_usd,
       s.net_profit_cents / 100.0     AS net_profit_usd,
       s.days_to_sell
FROM campaign_sales s
JOIN campaign_purchases p ON p.id = s.purchase_id
WHERE s.sale_date = '2026-04-11' AND s.sale_channel = 'inperson'
ORDER BY s.sale_price_cents DESC
LIMIT 5;

SELECT '=== CGC REMAINING ===' AS step;
SELECT COUNT(*) AS remaining_cgc FROM campaign_purchases WHERE grader = 'CGC';
