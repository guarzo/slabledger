package cardutil

import "testing"

func TestSetLanguage(t *testing.T) {
	tests := []struct {
		name    string
		setName string
		want    string
	}{
		{
			name:    "simplified chinese gem pack vol 1, cert 130221147",
			setName: "SIMPLIFIED CHINESE CBB1 C-GEM PACK VOL 1",
			want:    LangChinese,
		},
		{
			name:    "simplified chinese gem pack vol 2, cert 123238115",
			setName: "SIMPLIFIED CHINESE CBB2 C-GEM PACK VOL 2",
			want:    LangChinese,
		},
		{
			name:    "japanese mega symphonia, cert 139414865",
			setName: "JAPANESE M1S-MEGA SYMPHONIA",
			want:    LangJapanese,
		},
		{
			name:    "japanese shiny treasure ex, cert 132537172",
			setName: "JAPANESE SV4a-SHINY TREASURE ex",
			want:    LangJapanese,
		},
		{
			name:    "korean prefix (no repo precedent; convention defined by this task)",
			setName: "KOREAN S1-SWORD SHIELD",
			want:    LangKorean,
		},
		{
			name:    "korean marker mid-string",
			setName: "2024 POKEMON KOREAN PROMO CARD",
			want:    LangKorean,
		},
		{
			name:    "plain english promo, cert 72973327",
			setName: "SWSH BLACK STAR PROMO",
			want:    LangEnglish,
		},
		{
			name:    "plain english set, cert 145396462",
			setName: "CELEBRATIONS CLASSIC COLLECTION",
			want:    LangEnglish,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SetLanguage(tt.setName); got != tt.want {
				t.Errorf("SetLanguage(%q) = %q, want %q", tt.setName, got, tt.want)
			}
		})
	}
}

func TestIsJapaneseSet(t *testing.T) {
	tests := []struct {
		name    string
		setName string
		want    bool
	}{
		{"japanese prefix", "JAPANESE M1S-MEGA SYMPHONIA", true},
		{"english set", "SWSH BLACK STAR PROMO", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsJapaneseSet(tt.setName); got != tt.want {
				t.Errorf("IsJapaneseSet(%q) = %v, want %v", tt.setName, got, tt.want)
			}
		})
	}
}

func TestIsKoreanSet(t *testing.T) {
	tests := []struct {
		name    string
		setName string
		want    bool
	}{
		{"korean prefix", "KOREAN S1-SWORD SHIELD", true},
		{"korean contains", "2024 POKEMON KOREAN PROMO CARD", true},
		{"english set", "SWSH BLACK STAR PROMO", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsKoreanSet(tt.setName); got != tt.want {
				t.Errorf("IsKoreanSet(%q) = %v, want %v", tt.setName, got, tt.want)
			}
		})
	}
}
