package arbitrage

import (
	"math"
	"sort"

	"github.com/guarzo/slabledger/internal/domain/constants"
)

// MinAcquisitionProfitCents is the minimum profit ($100) to qualify as an opportunity.
const MinAcquisitionProfitCents = 10000

// AcquisitionOpportunity represents a raw-to-graded arbitrage opportunity.
type AcquisitionOpportunity struct {
	CardName        string         `json:"cardName"`
	SetName         string         `json:"setName"`
	CardNumber      string         `json:"cardNumber"`
	CertNumber      string         `json:"certNumber,omitempty"`
	RawNMCents      int            `json:"rawNMCents"`
	GradedEstimates map[string]int `json:"gradedEstimates"`
	BestGrade       string         `json:"bestGrade"`
	BestGradedCents int            `json:"bestGradedCents"`
	ProfitCents     int            `json:"profitCents"`
	ProfitROI       float64        `json:"profitROI"`
	Source          string         `json:"source"`
}

// computeAcquisitionOpportunity calculates the arbitrage for a single card.
// Returns nil if the best profit is below MinAcquisitionProfitCents.
// If ebayFeePct is invalid (<=0 or >=1), defaults to 12.35%.
func computeAcquisitionOpportunity(
	cardName, setName, cardNumber, certNumber string,
	rawNMCents int,
	gradedEstimates map[string]int,
	ebayFeePct float64,
	source string,
) *AcquisitionOpportunity {
	if rawNMCents <= 0 || len(gradedEstimates) == 0 {
		return nil
	}
	if ebayFeePct <= 0 || ebayFeePct >= 1 {
		ebayFeePct = constants.DefaultMarketplaceFeePct
	}

	bestGrade := ""
	bestProfit := 0
	bestGradedCents := 0

	for grade, gradedCents := range gradedEstimates {
		if gradedCents <= 0 {
			continue
		}
		net := gradedCents - int(math.Round(float64(gradedCents)*ebayFeePct))
		profit := net - rawNMCents
		if profit > bestProfit {
			bestProfit = profit
			bestGrade = grade
			bestGradedCents = gradedCents
		}
	}

	if bestProfit < MinAcquisitionProfitCents {
		return nil
	}

	roi := float64(bestProfit) / float64(rawNMCents)

	return &AcquisitionOpportunity{
		CardName:        cardName,
		SetName:         setName,
		CardNumber:      cardNumber,
		CertNumber:      certNumber,
		RawNMCents:      rawNMCents,
		GradedEstimates: gradedEstimates,
		BestGrade:       bestGrade,
		BestGradedCents: bestGradedCents,
		ProfitCents:     bestProfit,
		ProfitROI:       roi,
		Source:          source,
	}
}

// sortAcquisitionByProfit sorts opportunities by profit descending.
func sortAcquisitionByProfit(opps []AcquisitionOpportunity) {
	sort.Slice(opps, func(i, j int) bool {
		return opps[i].ProfitCents > opps[j].ProfitCents
	})
}
