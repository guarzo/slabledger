package inventory

import "testing"

func TestCleanMMPlayerName(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		setName string
		want    string
	}{
		{
			name:    "strip variation keywords",
			raw:     "CYNTHIA'S GARCHOMP ex SPECIAL ILLUSTRATION RARE",
			setName: "DRI EN-DESTINED RIVALS",
			want:    "Cynthia's Garchomp ex",
		},
		{
			name:    "strip dash-HOLO suffix",
			raw:     "NIDOKING-HOLO",
			setName: "AQUAPOLIS",
			want:    "Nidoking",
		},
		{
			name:    "strip dash-HOLO and set name",
			raw:     "ENTEI-HOLO EX UNSEEN FORCES",
			setName: "Pokemon EX Unseen Forces",
			want:    "Entei",
		},
		{
			name:    "strip FA prefix and variation",
			raw:     "FA/SHAYMIN EX XY COLLECTION PROMO",
			setName: "XY COLLECTION PROMO",
			want:    "Shaymin ex",
		},
		{
			name:    "strip dash-REV.FOIL",
			raw:     "SNORLAX-REV.FOIL",
			setName: "Pokemon Skyridge",
			want:    "Snorlax",
		},
		{
			name:    "strip dash-HOLO and set context words",
			raw:     "PIKACHU-HOLO BLACK STAR PROMOS",
			setName: "Pokemon Black Star Promos",
			want:    "Pikachu",
		},
		{
			name:    "strip dash-HOLO and 1ST EDITION variation",
			raw:     "DARK CHARIZARD-HOLO 1ST EDITION",
			setName: "ROCKET",
			want:    "Dark Charizard",
		},
		{
			name:    "strip dash-REV.FOIL and set words",
			raw:     "RAYQUAZA-REV.FOIL DELTA SPECIES",
			setName: "EX DELTA SPECIES",
			want:    "Rayquaza",
		},
		{
			name:    "strip FA prefix and promo suffix",
			raw:     "FA/GIRATINA PROMO-TEAM PLASMA",
			setName: "Pokemon Black & White Promo",
			want:    "Giratina",
		},
		{
			name:    "simple name no cleaning needed",
			raw:     "Charizard",
			setName: "Base Set",
			want:    "Charizard",
		},
		{
			name:    "empty set name still cleans",
			raw:     "UMBREON-HOLO",
			setName: "",
			want:    "Umbreon",
		},
		{
			name:    "M GENGAR EX with set name embedded",
			raw:     "M GENGAR EX PHANTOM FORCES",
			setName: "Pokemon Xy Phantom Forces",
			want:    "M Gengar ex",
		},
		{
			name:    "preserve ex suffix correctly",
			raw:     "MEW ex SPECIAL ILLUSTRATION RARE",
			setName: "",
			want:    "Mew ex",
		},
		{
			name:    "preserve GX suffix",
			raw:     "FA/UMBREON GX",
			setName: "Sun & Moon",
			want:    "Umbreon GX",
		},
		{
			name:    "GOLD STAR dash suffix",
			raw:     "UMBREON-GOLD STAR CLASSIC COLL-POP SERIES 5",
			setName: "Pokemon Celebrations Classic Collection",
			want:    "Umbreon",
		},
		{
			name:    "single word keeps value",
			raw:     "HOLO",
			setName: "",
			want:    "Holo",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cleanMMPlayerName(tc.raw, tc.setName)
			if got != tc.want {
				t.Errorf("cleanMMPlayerName(%q, %q) = %q, want %q", tc.raw, tc.setName, got, tc.want)
			}
		})
	}
}
