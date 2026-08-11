package postgres

// clFreezeMaxAgeDays bounds how long after a purchase the CL freeze may still
// fire. Past this, a CardLadder observation is no longer "at purchase" in any
// useful sense and writing it would put today's number in a column labelled
// with the purchase date. Forward-only by design: nothing backfills the rows
// the old unguarded behaviour already mislabelled (defect D3).
//
//nolint:unused // documents the policy number the literal 7s in clFreezeLateness must track; see that constant's comment for why it isn't interpolated
const clFreezeMaxAgeDays = 7

// clFreezeLateness is the shared write-time guard, ANDed into every freeze
// clause in UpdatePurchaseCLValue (purchase_store.go). purchase_date is TEXT
// (YYYY-MM-DD, migration 000001) and is validated for non-emptiness only
// (validation.go:193,216), so it can hold arbitrary client text. It is
// therefore never CAST here.
//
// A ::date cast -- and to_date(), which is strict in PG 17 -- raises
// "date/time field value out of range" on a date-SHAPED but calendar-invalid
// value such as 2026-99-99 or 2026-02-30. A raise inside the CASE aborts the
// whole UPDATE, so one planted row would permanently break every later call
// that touches it. Instead: a shape check, then a lexicographic comparison.
// YYYY-MM-DD is fixed-width and zero-padded, so byte order and calendar order
// agree, and no input can raise. The month/day ranges in the regex stop a
// shape-forged value from masquerading as recent; the upper bound stops
// future-dating. Anything failing the shape check compares false and every
// CASE using this fragment falls through to its ELSE -- fail-closed.
//
// Both bounds are inclusive: a purchase_date of exactly today-7 still
// freezes, as does a purchase_date of exactly today.
//
// Residual, accepted: 2026-02-30 passes the shape check and is compared as
// the text it is, shifting the effective window by at most a day or two.
// That is strictly better than a cast that can abort the statement.
//
// The literal 7 below must be kept in sync with clFreezeMaxAgeDays. Go
// constants cannot interpolate an int into a string constant (no const-time
// fmt.Sprintf), so either this stays a duplicated literal with the constant
// kept as documentation, or clFreezeLateness becomes a package-level var
// built via fmt.Sprintf at init. This picks the const + duplicated literal,
// because the guard is a fixed part of the query text, not a value computed
// per-call, and two consts keep the query fully static and inlineable by
// every caller without an init-order dependency.
// Parenthesized because it is now a multi-term conjunction spliced into a
// larger WHEN clause; the parens keep it correct if a future edit adds an OR.
const clFreezeLateness = `(purchase_date ~ '^[0-9]{4}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])' ` +
	`AND left(purchase_date, 10) >= to_char((now() AT TIME ZONE 'utc')::date - 7, 'YYYY-MM-DD') ` +
	`AND left(purchase_date, 10) <= to_char((now() AT TIME ZONE 'utc')::date, 'YYYY-MM-DD'))`
