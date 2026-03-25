//go:build !race

package pricecharting

// When race detector is disabled, set the flag to false
var raceEnabled = false
