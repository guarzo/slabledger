// Pipeline integration tests: normalization audit.
//
// Run with: go test -tags integration ./internal/integration/ -run TestNormalization -v
//
//go:build integration

package integration

import (
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/platform/cardutil"

	_ "github.com/joho/godotenv/autoload"
)

// TestNormalizationAudit verifies what the DH pricing source sees after
// normalization — no API calls required. Logs normalized forms for manual review.
func TestNormalizationAudit(t *testing.T) {
	for _, card := range uniqueInventory() {
		t.Run(card.CertNumber+"_"+card.CardName, func(t *testing.T) {
			// --- DH / card-match normalization ---
			ppName := cardutil.NormalizePurchaseName(card.CardName)
			ppName = cardutil.StripVariantSuffix(ppName)
			ppSet := cardutil.NormalizeSetNameForSearch(card.SetName)
			t.Logf("DH: name=%q set=%q", ppName, ppSet)

			if strings.Contains(ppName, "-HOLO") || strings.Contains(ppName, "-REV.FOIL") {
				t.Errorf("name has unexpanded abbreviation: %s", ppName)
			}
			if strings.HasPrefix(strings.ToUpper(ppSet), "JAPANESE ") {
				t.Logf("  NOTE: set preserves JAPANESE prefix (NormalizeSetNameForSearch retains it by design): %s", ppSet)
			}
			for _, code := range []string{"PRE EN", "M24 EN", "MEW EN", "SVP EN"} {
				if strings.Contains(strings.ToUpper(ppSet), code) {
					t.Errorf("set contains PSA code %q: %s", code, ppSet)
				}
			}

			// --- Set overlap sanity check ---
			selfMatch := cardutil.MatchesSetOverlap(card.SetName, card.SetName)
			t.Logf("SetOverlap: self-match=%v set=%q", selfMatch, card.SetName)
			if !selfMatch {
				t.Errorf("MatchesSetOverlap self-match failed for %q", card.SetName)
			}
		})
	}
}

