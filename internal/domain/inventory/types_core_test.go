package inventory

import "testing"

func TestCampaignSetKind(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{
			name: "external campaign ID yields external kind",
			id:   ExternalCampaignID,
			want: "external",
		},
		{
			name: "arbitrary uuid yields standard kind",
			id:   "3f2b1c4d-5e6a-7b8c-9d0e-1f2a3b4c5d6e",
			want: "standard",
		},
		{
			name: "empty ID yields standard kind",
			id:   "",
			want: "standard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Campaign{ID: tt.id}
			c.SetKind()
			if c.Kind != tt.want {
				t.Errorf("SetKind() with ID=%q: Kind = %q, want %q", tt.id, c.Kind, tt.want)
			}
		})
	}
}

func TestNeedsDHPush(t *testing.T) {
	received := "2026-04-20"
	tests := []struct {
		name string
		p    Purchase
		want bool
	}{
		{
			name: "ReceivedAtSet_Eligible",
			p:    Purchase{ReceivedAt: &received},
			want: true,
		},
		{
			name: "PSAShipDateSet_Eligible",
			p:    Purchase{PSAShipDate: "2026-04-20"},
			want: true,
		},
		{
			name: "BothSet_Eligible",
			p:    Purchase{ReceivedAt: &received, PSAShipDate: "2026-04-20"},
			want: true,
		},
		{
			name: "NeitherSet_NotEligible",
			p:    Purchase{},
			want: false,
		},
		{
			name: "ShippedButSoldOnDH_NotEligible",
			p:    Purchase{PSAShipDate: "2026-04-20", DHStatus: DHStatusSold},
			want: false,
		},
		{
			name: "ShippedButAlreadyPushed_NotEligible",
			p:    Purchase{PSAShipDate: "2026-04-20", DHInventoryID: 123},
			want: false,
		},
		{
			name: "ShippedButStatusPending_NotEligible",
			p:    Purchase{PSAShipDate: "2026-04-20", DHPushStatus: DHPushStatusPending},
			want: false,
		},
		{
			name: "ShippedButStatusHeld_NotEligible",
			p:    Purchase{PSAShipDate: "2026-04-20", DHPushStatus: DHPushStatusHeld},
			want: false,
		},
		{
			name: "ShippedButStatusUnmatched_NotEligible",
			p:    Purchase{PSAShipDate: "2026-04-20", DHPushStatus: DHPushStatusUnmatched},
			want: false,
		},
		{
			name: "ShippedButStatusManual_NotEligible",
			p:    Purchase{PSAShipDate: "2026-04-20", DHPushStatus: DHPushStatusManual},
			want: false,
		},
		{
			name: "ShippedButStatusDismissed_NotEligible",
			p:    Purchase{PSAShipDate: "2026-04-20", DHPushStatus: DHPushStatusDismissed},
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.p.NeedsDHPush(); got != tc.want {
				t.Errorf("NeedsDHPush() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNormalizeDHStatus(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "in_stock passes through", in: DHStatusInStock, want: DHStatusInStock},
		{name: "listed passes through", in: DHStatusListed, want: DHStatusListed},
		{name: "sold passes through", in: DHStatusSold, want: DHStatusSold},
		{name: "undocumented skipped is rejected", in: "skipped", want: ""},
		{name: "failed is rejected", in: "failed", want: ""},
		{name: "empty stays empty", in: "", want: ""},
		{name: "arbitrary garbage is rejected", in: "in-stock", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeDHStatus(tc.in); got != tc.want {
				t.Errorf("NormalizeDHStatus(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestDHStatusForPush(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want DHStatus
	}{
		{name: "listed passes through", in: DHStatusListed, want: DHStatusListed},
		{name: "in_stock passes through", in: DHStatusInStock, want: DHStatusInStock},
		{name: "sold passes through", in: DHStatusSold, want: DHStatusSold},
		{name: "empty (new push) defaults to in_stock", in: "", want: DHStatusInStock},
		// A non-inventory value (e.g. DH's psa_import "skipped" = already
		// present, untouched) must NOT be guessed as in_stock — that would let
		// the price-sync PATCH delist a real "listed" item. Drop to "" so the
		// inventory poll backfills DH's authoritative status.
		{name: "skipped drops to empty for poll backfill", in: "skipped", want: ""},
		{name: "failed drops to empty", in: "failed", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DHStatusForPush(tc.in); got != tc.want {
				t.Errorf("DHStatusForPush(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
