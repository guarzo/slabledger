ALTER TABLE campaign_sales
    DROP COLUMN cl_value_at_sale_observed_at,
    DROP COLUMN cl_value_at_sale_source;

ALTER TABLE campaign_purchases
    DROP COLUMN cl_value_at_purchase_observed_at,
    DROP COLUMN cl_value_at_purchase_source,
    DROP COLUMN cl_card_confidence_at_purchase;
