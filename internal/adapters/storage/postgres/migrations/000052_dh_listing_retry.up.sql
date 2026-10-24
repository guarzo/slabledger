ALTER TABLE dh_mutation_attempts
 ADD COLUMN listing_retryable BOOLEAN NOT NULL DEFAULT FALSE,
 ADD CONSTRAINT dh_listing_retry_valid CHECK (
  NOT listing_retryable OR (
   outcome='open' AND kind='list' AND phase='intake_patch_sync'
   AND dh_inventory_id>0 AND idempotency_key='' AND return_operation_id IS NULL
  )
 );
