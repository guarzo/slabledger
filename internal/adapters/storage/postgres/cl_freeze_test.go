package postgres

import (
	"strconv"
	"strings"
	"testing"
)

// TestCLFreezeLatenessMatchesMaxAgeDays enforces the sync that cl_freeze.go's
// comment can only ask for. clFreezeLateness must embed the window as a literal
// -- Go cannot interpolate an int into a string constant -- so clFreezeMaxAgeDays
// is documentation unless something checks it. Without this test, changing the
// constant silently leaves the query running the old window, and every freeze
// decision in UpdatePurchaseCLValue would use a bound no reader expects.
//
// This needs no database: both operands are compile-time constants.
func TestCLFreezeLatenessMatchesMaxAgeDays(t *testing.T) {
	want := "::date - " + strconv.Itoa(clFreezeMaxAgeDays) + ","
	if !strings.Contains(clFreezeLateness, want) {
		t.Errorf("clFreezeLateness does not use clFreezeMaxAgeDays (%d): expected to find %q in:\n%s",
			clFreezeMaxAgeDays, want, clFreezeLateness)
	}
}
