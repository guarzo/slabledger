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

func TestClampHistoryLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{name: "zero uses the default", limit: 0, want: DefaultHistoryLimit},
		{name: "negative uses the default", limit: -5, want: DefaultHistoryLimit},
		{name: "one is honoured", limit: 1, want: 1},
		{name: "in-range value passes through", limit: 250, want: 250},
		{name: "max is honoured", limit: MaxHistoryLimit, want: MaxHistoryLimit},
		{name: "over max is capped", limit: MaxHistoryLimit + 1, want: MaxHistoryLimit},
		{name: "far over max is capped", limit: 1_000_000, want: MaxHistoryLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClampHistoryLimit(tt.limit); got != tt.want {
				t.Errorf("ClampHistoryLimit(%d) = %d, want %d", tt.limit, got, tt.want)
			}
		})
	}
}
