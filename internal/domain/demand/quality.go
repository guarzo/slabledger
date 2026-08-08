package demand

// CardCharacterQuality pairs a cached card's character attribution with the
// demand data quality DH reported for that card. It is the input to
// AggregateCharacterQuality.
type CardCharacterQuality struct {
	CharacterName string
	DataQuality   string // "proxy" | "full"
}

// AggregateCharacterQuality rolls per-card demand data quality up to the
// character level, keyed by character name.
//
// The rule is any-card-full: a character is "full" when at least one of its
// contributing cards has full data, and "proxy" otherwise. This follows the
// existing precedent on NicheAcceleration.DataQuality ("full" when any
// contributor has full data) so the two surfaces agree on what the label means.
//
// DH does not report data_quality at the character level — it exists only per
// card, on dh.DemandSignal — so this aggregation is the only source for it.
// Rows without a character name or without a quality value contribute nothing;
// any value other than "full" is treated as proxy rather than trusted verbatim,
// so an unrecognised label can never satisfy a min_data_quality=full filter.
func AggregateCharacterQuality(rows []CardCharacterQuality) map[string]string {
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		if r.CharacterName == "" || r.DataQuality == "" {
			continue
		}
		if r.DataQuality == QualityFull {
			out[r.CharacterName] = QualityFull
			continue
		}
		if _, seen := out[r.CharacterName]; !seen {
			out[r.CharacterName] = QualityProxy
		}
	}
	return out
}
