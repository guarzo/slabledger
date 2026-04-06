// Package fusion implements multi-source data aggregation with confidence scoring.
//
// The fusion engine combines price data from secondary sources to produce more
// accurate and confident price estimates.
//
// # Fusion Algorithm
//
//  1. Fetch prices from all available sources in parallel
//  2. Group prices by grade (PSA 10, PSA 9, etc.)
//  3. Apply weighted median fusion with outlier detection
//  4. Calculate confidence score based on source agreement
//
// # Source Weights
//
//   - DoubleHolo: 0.90 (recent sales data)
//
// PriceCharting provides market data only; it is not included as a fusion source.
//
// # Confidence Scoring
//
// Confidence increases with:
//   - More sources (1 source = 0.70, 2 sources = 0.90)
//   - Better agreement between sources (low variance)
//   - Fewer outliers detected
//
// # Usage
//
//	fusionEngine := fusion.NewFusionEngine(config, logger)
//	result, err := fusionEngine.FusePrices(ctx, []PriceData{...})
//
//	fmt.Printf("Confidence: %.2f\n", result.Confidence)
//	fmt.Printf("Sources: %d\n", result.SourceCount)
package fusion
