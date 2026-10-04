package inventory

// AssertPSAIntakeIdentity binds the cert/grader journal fence to the actual
// outbound issuer. PSA import and NewInStockItem send PSA, not p.Grader; a
// different local issuer must never escape a retained PSA cert hold this way.
func AssertPSAIntakeIdentity(p *Purchase) error {
	if p == nil || p.Grader != "PSA" {
		return NewReturnConflict("unsupported_grader", "DH PSA intake requires an exact PSA purchase identity")
	}
	return nil
}
