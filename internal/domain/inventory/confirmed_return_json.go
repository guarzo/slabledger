package inventory

import "encoding/json"

// The state boundary deliberately excludes all server idempotency keys. Keep
// the internal Sale intact for fresh dispatch/source checks.
func (s ConfirmedReturnState) MarshalJSON() ([]byte, error) {
	type state ConfirmedReturnState
	wire := state(s)
	if s.Sale != nil {
		sale := *s.Sale
		sale.DHIdempotencyKey = ""
		wire.Sale = &sale
	}
	return json.Marshal(wire)
}
func (e ConfirmedReturnEpisode) MarshalJSON() ([]byte, error) {
	type episode ConfirmedReturnEpisode
	type receipt struct {
		DHInventoryID  int    `json:"dhInventoryId"`
		ItemStatus     string `json:"itemStatus"`
		ExternalSaleID int64  `json:"externalSaleId"`
		Restored       bool   `json:"restored"`
	}
	var r *receipt
	if e.ObservedReceipt != nil {
		v := e.ObservedReceipt
		r = &receipt{v.DHInventoryID, v.ItemStatus, v.ExternalSaleID, v.Restored}
	}
	return json.Marshal(struct {
		episode
		ObservedReceipt *receipt `json:"observedReceipt,omitempty"`
	}{episode(e), r})
}
