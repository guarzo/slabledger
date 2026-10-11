package showprep

import "sort"

// MaxWithdrawalGeneration is the largest integer exactly representable on the wire.
const MaxWithdrawalGeneration int64 = 9007199254740991

type DeleteListCommand struct {
	CommandID     string `json:"commandId"`
	ExpectedState string `json:"expectedState"`
}
type DeleteListResult struct {
	ListID  string `json:"listId"`
	Deleted bool   `json:"deleted"`
	Replay  bool   `json:"replay"`
}
type ConfirmExistingCommand struct {
	CommandID     string   `json:"commandId"`
	ExpectedState string   `json:"expectedState"`
	ItemIDs       []string `json:"itemIds"`
}
type ConfirmExistingResult struct {
	ListID           string   `json:"listId"`
	CommandID        string   `json:"commandId"`
	ConfirmedItemIDs []string `json:"confirmedItemIds"`
	Replay           bool     `json:"replay"`
}
type RetiredList struct {
	ListID, DeleteCommandID, PayloadHash, RetiredAt string
}

// ListStateFingerprint binds lifecycle consent to persisted state, never current
// market evidence or availability. Sorting a copy keeps reads order-independent.
func ListStateFingerprint(list List, items []Item) string {
	type member struct {
		ID, PurchaseID, CardName, CertNumber, Grader string
		Grade                                        float64
		AddedAt, PackedAt                            string
		Version                                      int64
		AcknowledgedPriceCents                       int
		AcknowledgedStatus                           Status
		LastCommand                                  string
	}
	members := make([]member, 0, len(items))
	for _, i := range items {
		members = append(members, member{i.ID, i.PurchaseID, i.CardName, i.CertNumber, i.Grader, i.Grade, i.AddedAt, i.PackedAt, i.Version, i.AcknowledgedPriceCents, i.AcknowledgedStatus, i.LastCommand})
	}
	sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
	return Fingerprint(struct {
		List    List
		Members []member
	}{list, members})
}
