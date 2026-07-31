package psaportal

// Sentinel is a devalue negative-pointer marker: a JavaScript value with no
// JSON equivalent. SvelteKit's __data.json payloads encode these as negative
// slot pointers rather than real slot indices.
//
// It exists purely for PSA/devalue wire fidelity on the read-modify-write push
// path — DecodeRefPacked preserves it so EncodeRefPacked can re-emit the exact
// value PSA sent. It is not a domain concept and must not leak into
// psacampaign.
//
// Values are verbatim from devalue@5.9.0 src/constants.js.
type Sentinel int

const (
	SentinelUndefined Sentinel = -1
	SentinelHole      Sentinel = -2
	SentinelNaN       Sentinel = -3
	SentinelPosInf    Sentinel = -4
	SentinelNegInf    Sentinel = -5
	SentinelNegZero   Sentinel = -6
	SentinelSparse    Sentinel = -7
)

// sentinelLowest is the most negative valid sentinel; pointers below it are
// malformed rather than meaningful.
const sentinelLowest = SentinelSparse

// sentinelFor reports whether ptr is a devalue sentinel and, if so, which one.
// Zero and positive pointers are real slot indices, not sentinels.
func sentinelFor(ptr int) (Sentinel, bool) {
	if ptr >= 0 || ptr < int(sentinelLowest) {
		return 0, false
	}
	return Sentinel(ptr), true
}

// String names the JavaScript value this sentinel stands for.
func (s Sentinel) String() string {
	switch s {
	case SentinelUndefined:
		return "undefined"
	case SentinelHole:
		return "hole"
	case SentinelNaN:
		return "NaN"
	case SentinelPosInf:
		return "Infinity"
	case SentinelNegInf:
		return "-Infinity"
	case SentinelNegZero:
		return "-0"
	case SentinelSparse:
		return "sparse"
	}
	return "unknown"
}
