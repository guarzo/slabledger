package campaigns

// CrackAnalysis contains the crack arbitrage analysis for a single PSA 8 card.
type CrackAnalysis struct {
	PurchaseID        string  `json:"purchaseId"`
	CardName          string  `json:"cardName"`
	CertNumber        string  `json:"certNumber"`
	Grade             float64 `json:"grade"`
	BuyCostCents      int     `json:"buyCostCents"`
	CostBasisCents    int     `json:"costBasisCents"` // buy + sourcing fee
	RawMarketCents    int     `json:"rawMarketCents"`
	BreakevenRawCents int     `json:"breakevenRawCents"`
	GradedNetCents    int     `json:"gradedNetCents"`      // estimated graded sale net (via best channel)
	CrackNetCents     int     `json:"crackNetCents"`       // raw sale net after eBay fees
	CrackAdvantage    int     `json:"crackAdvantageCents"` // crack net - graded net
	IsCrackCandidate  bool    `json:"isCrackCandidate"`
	CrackROI          float64 `json:"crackROI"`
	GradedROI         float64 `json:"gradedROI"`
}

// computeCrackAnalysis determines whether cracking a PSA 8 slab and selling raw is profitable.
// Formula: raw eBay sold price > PSA 8 purchase cost / 0.8765
// The 0.8765 factor accounts for eBay fees (12.35%).
func computeCrackAnalysis(
	purchaseID, cardName, certNumber string,
	grade float64,
	buyCostCents, sourcingFeeCents, rawMarketCents, gradedMarketCents int,
	ebayFeePct float64,
) *CrackAnalysis {
	costBasis := buyCostCents + sourcingFeeCents

	// Breakeven raw price: costBasis / (1 - ebayFeePct)
	if ebayFeePct < 0 || ebayFeePct >= 1 {
		ebayFeePct = DefaultMarketplaceFeePct
	}
	breakevenRaw := int(float64(costBasis) / (1 - ebayFeePct))

	// Net from selling raw on eBay
	crackNet := rawMarketCents - int(float64(rawMarketCents)*ebayFeePct) - costBasis

	// Net from selling graded (estimate: gradedMarket - fees - cost)
	gradedNet := gradedMarketCents - int(float64(gradedMarketCents)*ebayFeePct) - costBasis

	isCrackCandidate := rawMarketCents > breakevenRaw && crackNet > gradedNet

	crackROI := 0.0
	if costBasis > 0 {
		crackROI = float64(crackNet) / float64(costBasis)
	}
	gradedROI := 0.0
	if costBasis > 0 {
		gradedROI = float64(gradedNet) / float64(costBasis)
	}

	return &CrackAnalysis{
		PurchaseID:        purchaseID,
		CardName:          cardName,
		CertNumber:        certNumber,
		Grade:             grade,
		BuyCostCents:      buyCostCents,
		CostBasisCents:    costBasis,
		RawMarketCents:    rawMarketCents,
		BreakevenRawCents: breakevenRaw,
		GradedNetCents:    gradedNet,
		CrackNetCents:     crackNet,
		CrackAdvantage:    crackNet - gradedNet,
		IsCrackCandidate:  isCrackCandidate,
		CrackROI:          crackROI,
		GradedROI:         gradedROI,
	}
}
