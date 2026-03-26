package campaigns

import "testing"

func TestBuildEbayTitle_MultiByteTruncation(t *testing.T) {
	// Japanese characters are 3 bytes each in UTF-8.
	// Build a title that exceeds 80 runes to trigger truncation.
	longJapaneseName := "リザードンポケモンカードゲーム"
	title := buildEbayTitle(longJapaneseName, "ポケモンカードゲーム 151 スカーレット バイオレット", "004", "9")
	if len([]rune(title)) > ebayMaxTitleLen {
		t.Errorf("title rune length %d exceeds max %d", len([]rune(title)), ebayMaxTitleLen)
	}
	// Verify the title is valid UTF-8 (no split multi-byte chars).
	for i, r := range title {
		if r == 0xFFFD {
			t.Errorf("replacement char at byte %d — title was split mid-character", i)
		}
	}
}

func TestBuildEbayTitle_ShortTitle(t *testing.T) {
	title := buildEbayTitle("Charizard", "Base Set", "4", "8")
	want := "Charizard Pokemon Base Set 4 PSA 8"
	if title != want {
		t.Errorf("title = %q, want %q", title, want)
	}
}

func TestFormatGrade_WholeNumber(t *testing.T) {
	if got := formatGrade(9); got != "9" {
		t.Errorf("formatGrade(9) = %q, want %q", got, "9")
	}
}

func TestFormatGrade_HalfGrade(t *testing.T) {
	if got := formatGrade(9.5); got != "9.5" {
		t.Errorf("formatGrade(9.5) = %q, want %q", got, "9.5")
	}
}
