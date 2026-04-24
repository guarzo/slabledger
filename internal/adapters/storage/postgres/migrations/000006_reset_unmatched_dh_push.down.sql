-- No-op: the up migration resets stuck 'unmatched' rows to 'pending' so they
-- retry under the new psa_import path. Reverting the reset would require
-- knowing which rows were originally unmatched, which we didn't preserve.
-- Rolling back to the old flow (dead /certs/resolve endpoint + skip-attempts
-- cap) would immediately re-latch these rows to unmatched anyway.
SELECT 1;
