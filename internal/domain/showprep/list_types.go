package showprep

type List struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}
type Item struct {
	ID                     string     `json:"id"`
	PurchaseID             string     `json:"purchaseId"`
	CardName               string     `json:"cardName"`
	CertNumber             string     `json:"certNumber"`
	Grader                 string     `json:"grader"`
	Grade                  float64    `json:"grade"`
	AddedAt                string     `json:"addedAt"`
	PackedAt               string     `json:"packedAt"`
	Version                int64      `json:"version"`
	AcknowledgedPriceCents int        `json:"acknowledgedPriceCents"`
	AcknowledgedStatus     Status     `json:"acknowledgedStatus"`
	Evaluation             Evaluation `json:"evaluation"`
	PriceChanged           bool       `json:"priceChanged"`
	SupportChanged         bool       `json:"supportChanged"`
	LastCommand            string     `json:"-"`
}
type Summary struct {
	TotalCount          int `json:"totalCount"`
	PackedCount         int `json:"packedCount"`
	NotReceivedCount    int `json:"notReceivedCount"`
	UnavailableCount    int `json:"unavailableCount"`
	KnownValueCents     int `json:"knownValueCents"`
	MissingPriceCount   int `json:"missingPriceCount"`
	AmbiguousPriceCount int `json:"ambiguousPriceCount"`
}
type ListDetail struct {
	List    List    `json:"list"`
	Items   []Item  `json:"items"`
	Summary Summary `json:"summary"`
}
type AddItem struct {
	PurchaseID        string `json:"purchaseId"`
	EvaluationVersion string `json:"evaluationVersion"`
}
type UpdateItem struct {
	Version           int64  `json:"version"`
	EvaluationVersion string `json:"evaluationVersion"`
	Packed            *bool  `json:"packed"`
	Acknowledge       bool   `json:"acknowledge"`
}
