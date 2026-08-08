package mocks

import (
	"fmt"
	"reflect"
)

// RowScanner is a test double for the postgres package's unexported
// `scanner` interface (`Scan(dest ...any) error`), which *sql.Row and
// *sql.Rows both satisfy structurally. It lets row-scanning functions be
// unit-tested without a database.
//
// Values holds the column values in the exact order the function under
// test scans them. Scan assigns Values[i] into dest[i] via reflection
// rather than a type switch: postgres row scanners scan into a long tail
// of concrete types (string, time.Time, sql.NullString, sql.NullTime,
// sql.NullFloat64, ...), and a type switch would need one case per type
// per test, duplicated across every scanner test in the package.
// Reflection handles all of them uniformly; the cost is that a
// Values[i] whose type doesn't match *dest[i] panics instead of failing
// gracefully, so fixtures must supply exactly the type the real driver
// would produce (e.g. sql.NullString{...}, not *string).
type RowScanner struct {
	// Values are the column values, in scan order.
	Values []any
	// Err, if set, is returned by Scan without assigning anything.
	Err error
}

// Scan assigns each Values[i] into dest[i]. It returns an error if the
// number of destinations does not match the number of supplied values.
func (r *RowScanner) Scan(dest ...any) error {
	if r.Err != nil {
		return r.Err
	}
	if len(dest) != len(r.Values) {
		return fmt.Errorf("mocks.RowScanner: got %d scan destinations, have %d values", len(dest), len(r.Values))
	}
	for i, d := range dest {
		reflect.ValueOf(d).Elem().Set(reflect.ValueOf(r.Values[i]))
	}
	return nil
}
