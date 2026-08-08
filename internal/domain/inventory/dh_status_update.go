package inventory

// DHInventoryStatusUpdate carries the fields that UpdateInventoryStatus can
// mutate on a DH inventory item. Image URLs are optional; when either is set
// on a transition to "listed", DH uses them instead of doing its own PSA
// lookup, which keeps the listing path functional when PSA is rate-limited
// or authentication is failing.
//
// Lives in inventory rather than dhlisting because both dhlisting and
// dhpricing declare an UpdateInventoryStatus method satisfied by the same
// adapter. Go interface satisfaction is structural on the method signature,
// so the two interfaces must name one identical type — and a sibling may
// only reach it through the inventory hub (the flat sibling rule; see
// scripts/check-imports.sh).
type DHInventoryStatusUpdate struct {
	Status            string
	ListingPriceCents int // 0 means omit
	CertImageURLFront string
	CertImageURLBack  string
}
