ALTER TABLE dh_mutation_attempts
 DROP CONSTRAINT dh_mutation_attempts_outcome_check,
 ADD CONSTRAINT dh_mutation_attempts_outcome_check
 CHECK(outcome IN ('open','succeeded','rejected','abandoned'));
