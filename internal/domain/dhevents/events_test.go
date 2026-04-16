package dhevents

import "testing"

func TestTypeAndSourceConstantsAreDistinct(t *testing.T) {
	types := []Type{
		TypeEnrolled, TypePushed, TypeListed, TypeUnlisted, TypeChannelSynced,
		TypeSold, TypeOrphanSale, TypeAlreadySold, TypeHeld,
		TypeDismissed, TypeUnmatched, TypeCardIDResolved,
	}
	seen := make(map[Type]bool, len(types))
	for _, ty := range types {
		if seen[ty] {
			t.Errorf("duplicate Type constant: %q", ty)
		}
		seen[ty] = true
	}

	sources := []Source{
		SourceDHOrdersPoll, SourceDHInventoryPoll, SourceCertIntake,
		SourcePSAImport, SourceManualUI,
		SourceCLRefresh, SourceDHListing,
	}
	seenSrc := make(map[Source]bool, len(sources))
	for _, s := range sources {
		if seenSrc[s] {
			t.Errorf("duplicate Source constant: %q", s)
		}
		seenSrc[s] = true
	}
}
