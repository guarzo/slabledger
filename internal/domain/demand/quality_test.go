package demand

import "testing"

func TestAggregateCharacterQuality(t *testing.T) {
	tests := []struct {
		name string
		rows []CardCharacterQuality
		want map[string]string
	}{
		{
			name: "no rows yields empty map",
			rows: nil,
			want: map[string]string{},
		},
		{
			name: "single full card makes the character full",
			rows: []CardCharacterQuality{
				{CharacterName: "Charizard", DataQuality: QualityFull},
			},
			want: map[string]string{"Charizard": QualityFull},
		},
		{
			name: "all proxy cards stay proxy",
			rows: []CardCharacterQuality{
				{CharacterName: "Mewtwo", DataQuality: QualityProxy},
				{CharacterName: "Mewtwo", DataQuality: QualityProxy},
			},
			want: map[string]string{"Mewtwo": QualityProxy},
		},
		{
			name: "any full card wins over proxy regardless of order",
			rows: []CardCharacterQuality{
				{CharacterName: "Charizard", DataQuality: QualityProxy},
				{CharacterName: "Charizard", DataQuality: QualityFull},
				{CharacterName: "Mewtwo", DataQuality: QualityFull},
				{CharacterName: "Mewtwo", DataQuality: QualityProxy},
			},
			want: map[string]string{"Charizard": QualityFull, "Mewtwo": QualityFull},
		},
		{
			name: "unnamed characters and blank qualities are ignored",
			rows: []CardCharacterQuality{
				{CharacterName: "", DataQuality: QualityFull},
				{CharacterName: "Pikachu", DataQuality: ""},
				{CharacterName: "Pikachu", DataQuality: QualityProxy},
			},
			want: map[string]string{"Pikachu": QualityProxy},
		},
		{
			name: "unrecognised quality values do not beat proxy",
			rows: []CardCharacterQuality{
				{CharacterName: "Snorlax", DataQuality: "partial"},
			},
			want: map[string]string{"Snorlax": QualityProxy},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AggregateCharacterQuality(tt.rows)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d characters (%v), want %d (%v)", len(got), got, len(tt.want), tt.want)
			}
			for char, want := range tt.want {
				if got[char] != want {
					t.Errorf("character %q: got quality %q, want %q", char, got[char], want)
				}
			}
		})
	}
}
