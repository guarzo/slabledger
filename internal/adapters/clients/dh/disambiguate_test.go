package dh

import "testing"

func TestDisambiguate(t *testing.T) {
	candidates := []CertResolutionCandidate{
		{DHCardID: 269, CardName: "Charizard ex", SetName: "Pokemon 151", CardNumber: "199"},
		{DHCardID: 277, CardName: "Charizard ex", SetName: "Pokemon Paldean Fates", CardNumber: "234"},
		{DHCardID: 283, CardName: "Charizard ex", SetName: "Pokemon Obsidian Flames", CardNumber: "228"},
	}

	tests := []struct {
		name       string
		candidates []CertResolutionCandidate
		cardNumber string
		want       int
	}{
		{"exact match single", candidates, "234", 277},
		{"leading zero match", candidates, "0199", 269},
		{"no match", candidates, "999", 0},
		{"empty card number", candidates, "", 0},
		{"nil candidates", nil, "234", 0},
		{"empty candidates", []CertResolutionCandidate{}, "234", 0},
		{
			"multiple candidates same number",
			[]CertResolutionCandidate{
				{DHCardID: 22296, CardNumber: "96"},
				{DHCardID: 23392, CardNumber: "96"},
				{DHCardID: 23397, CardNumber: "96"},
			},
			"096",
			0,
		},
		{
			"card number zero matches candidate zero",
			[]CertResolutionCandidate{
				{DHCardID: 100, CardNumber: "0"},
				{DHCardID: 200, CardNumber: "5"},
			},
			"0",
			100,
		},
		{
			"card number double zero matches candidate zero",
			[]CertResolutionCandidate{
				{DHCardID: 100, CardNumber: "0"},
				{DHCardID: 200, CardNumber: "5"},
			},
			"00",
			100,
		},
		{
			"card number triple zero matches candidate zero",
			[]CertResolutionCandidate{
				{DHCardID: 100, CardNumber: "0"},
				{DHCardID: 200, CardNumber: "5"},
			},
			"000",
			100,
		},
		{
			"card number zero no matching candidate",
			[]CertResolutionCandidate{
				{DHCardID: 200, CardNumber: "5"},
			},
			"0",
			0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Disambiguate(tc.candidates, tc.cardNumber)
			if got != tc.want {
				t.Errorf("Disambiguate(%q) = %d, want %d", tc.cardNumber, got, tc.want)
			}
		})
	}
}
