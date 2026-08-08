package csvimport

// ParseError captures a per-row parse failure with field-level detail.
type ParseError struct {
	Row     int    `json:"row"`
	Field   string `json:"field"`
	Message string `json:"message"`
}
