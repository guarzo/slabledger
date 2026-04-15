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
