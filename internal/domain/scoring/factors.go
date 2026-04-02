package scoring

func ComputeMarketTrend(priceChangePct, confidence float64, source string) Factor {
	return Factor{
		Name:       FactorMarketTrend,
		Value:      clamp(priceChangePct/20.0, -1.0, 1.0),
		Confidence: confidence,
		Source:     source,
	}
}

func ComputeLiquidity(salesPerMonth, confidence float64, source string) Factor {
	var v float64
	switch {
	case salesPerMonth >= 10:
		v = 1.0
	case salesPerMonth >= 5:
		v = 0.5
	case salesPerMonth >= 2:
		v = 0.0
	case salesPerMonth >= 0.5:
		v = -0.5
	default:
		v = -1.0
	}
	return Factor{
		Name:       FactorLiquidity,
		Value:      v,
		Confidence: confidence,
		Source:     source,
	}
}

func ComputeROIPotential(roiPct, confidence float64, source string) Factor {
	return Factor{
		Name:       FactorROIPotential,
		Value:      clamp(roiPct/50.0, -1.0, 1.0),
		Confidence: confidence,
		Source:     source,
	}
}

func ComputeScarcity(psa10Pop int, confidence float64, source string) Factor {
	var v float64
	switch {
	case psa10Pop < 250:
		v = 0.8
	case psa10Pop < 1000:
		v = 0.6
	case psa10Pop < 2500:
		v = 0.4
	case psa10Pop < 5000:
		v = 0.2
	case psa10Pop < 7500:
		v = 0.0
	case psa10Pop < 10000:
		v = -0.15
	default:
		v = -0.3
	}
	return Factor{
		Name:       FactorScarcity,
		Value:      v,
		Confidence: confidence,
		Source:     source,
	}
}

func ComputeMarketAlignment(trend30dPct, confidence float64, source string) Factor {
	return Factor{
		Name:       FactorMarketAlignment,
		Value:      clamp(trend30dPct/10.0, -1.0, 1.0),
		Confidence: confidence,
		Source:     source,
	}
}

func ComputePortfolioFit(concentrationRisk string, confidence float64, source string) Factor {
	var v float64
	switch concentrationRisk {
	case "low":
		v = 0.8
	case "high":
		v = -0.8
	default:
		v = 0.0
	}
	return Factor{
		Name:       FactorPortfolioFit,
		Value:      v,
		Confidence: confidence,
		Source:     source,
	}
}

func ComputeGradeFit(gradeROI, campaignAvgROI, confidence float64, source string) Factor {
	diff := gradeROI - campaignAvgROI
	return Factor{
		Name:       FactorGradeFit,
		Value:      clamp(diff/30.0, -1.0, 1.0),
		Confidence: confidence,
		Source:     source,
	}
}

func ComputeCreditPressure(utilizationPct, confidence float64, source string) Factor {
	var v float64
	switch {
	case utilizationPct > 95:
		v = -1.0
	case utilizationPct >= 85:
		v = -0.6
	case utilizationPct >= 70:
		v = -0.3
	default:
		v = 0.0
	}
	return Factor{
		Name:       FactorCreditPressure,
		Value:      v,
		Confidence: confidence,
		Source:     source,
	}
}

func ComputeCarryingCost(daysHeld int, confidence float64, source string) Factor {
	v := min(float64(daysHeld)/180.0, 1.0)
	return Factor{
		Name:       FactorCarryingCost,
		Value:      v,
		Confidence: confidence,
		Source:     source,
	}
}

func ComputeCrackAdvantage(crackROI, gradedROI, confidence float64, source string) Factor {
	diff := crackROI - gradedROI
	return Factor{
		Name:       FactorCrackAdvantage,
		Value:      clamp(diff/50.0, -1.0, 1.0),
		Confidence: confidence,
		Source:     source,
	}
}

func ComputeSellThrough(sellThroughPct, confidence float64, source string) Factor {
	return Factor{
		Name:       FactorSellThrough,
		Value:      clamp((sellThroughPct-50.0)/50.0, -1.0, 1.0),
		Confidence: confidence,
		Source:     source,
	}
}

func ComputeSpendEfficiency(fillRatePct, roiPct, confidence float64, source string) Factor {
	var v float64
	switch {
	case fillRatePct < 20:
		v = -0.6
	case fillRatePct >= 95 && roiPct > 10:
		v = 0.6
	case fillRatePct >= 60 && fillRatePct <= 80:
		v = 0.3
	default:
		v = 0.0
	}
	return Factor{
		Name:       FactorSpendEfficiency,
		Value:      v,
		Confidence: confidence,
		Source:     source,
	}
}

func ComputeCoverageImpact(fillsGap bool, overlapCount int, confidence float64, source string) Factor {
	var v float64
	if fillsGap {
		v = 0.8
	} else if overlapCount > 0 {
		v = -0.3
	}
	return Factor{
		Name:       FactorCoverageImpact,
		Value:      v,
		Confidence: confidence,
		Source:     source,
	}
}
