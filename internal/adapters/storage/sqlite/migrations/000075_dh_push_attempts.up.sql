-- Track consecutive DH push skip attempts so a cert that DH can't match (e.g.
-- ambiguous-with-failed-disambig, matched-with-zero card ID, transient push
-- errors) isn't resolved every 5 min forever. The scheduler increments this
-- on every processSkipped outcome and flips the row to 'unmatched' once the
-- counter crosses its cap. Reset to 0 whenever the row transitions back into
-- 'pending' or 'matched'.

ALTER TABLE campaign_purchases
ADD COLUMN dh_push_attempts INTEGER NOT NULL DEFAULT 0;
