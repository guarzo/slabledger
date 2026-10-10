package showprep

// ScanCommand is a single physical-consent intent. Retries keep all fields unchanged.
type ScanCommand struct {
	CommandID            string `json:"commandId"`
	CertNumber           string `json:"certNumber"`
	WithdrawalGeneration int64  `json:"withdrawalGeneration"`
}
type ScanOutcome struct {
	CommandID              string `json:"commandId"`
	ListID                 string `json:"listId"`
	CertNumber             string `json:"certNumber"`
	Outcome                string `json:"outcome"` // confirmed, already_confirmed, rechecked, rejected
	Code                   string `json:"code"`
	ItemID                 string `json:"itemId"`
	PurchaseID             string `json:"purchaseId"`
	CardName               string `json:"cardName"`
	ConfirmedAt            string `json:"confirmedAt"`
	AcknowledgedPriceCents int    `json:"acknowledgedPriceCents"`
	AcknowledgedStatus     Status `json:"acknowledgedStatus"`
}
type ScanCurrent struct {
	State      string      `json:"state"` // present, removed, unavailable, unknown
	Evaluation *Evaluation `json:"evaluation,omitempty"`
}
type ScanResult struct {
	Original ScanOutcome `json:"original"`
	Replay   bool        `json:"replay"`
	Current  ScanCurrent `json:"current"`
}
type CommandReceipt struct {
	CommandID, ListID, Kind, PayloadHash string
	Scan                                 *ScanOutcome
	Transition                           *ConfirmExistingResult
}
