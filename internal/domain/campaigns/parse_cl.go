package campaigns

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseCLRefreshRows parses CSV records from a Card Ladder export for CL value refresh.
// The first row must be the header row. Returns parsed rows, any parse errors, and a
// fatal error if the CSV structure is invalid (e.g., missing required columns).
func ParseCLRefreshRows(records [][]string) ([]CLExportRow, []ParseError, error) {
	if len(records) < 2 {
		return nil, nil, fmt.Errorf("CSV must have a header row and at least one data row")
	}

	headerMap := BuildHeaderMap(records[0])

	if _, exists := headerMap["slab serial #"]; !exists {
		return nil, nil, fmt.Errorf(`missing required column: "slab serial #"`)
	}

	colIdx := func(name string) int {
		if idx, ok := headerMap[name]; ok {
			return idx
		}
		return -1
	}

	var clRows []CLExportRow
	var parseErrors []ParseError
	for i, rec := range records[1:] {
		rowNum := i + 2

		getField := func(idx int) string {
			if idx >= 0 && idx < len(rec) {
				return strings.TrimSpace(rec[idx])
			}
			return ""
		}

		slabSerial := getField(colIdx("slab serial #"))
		if slabSerial == "" {
			continue
		}

		cvStr := getField(colIdx("current value"))
		if cvStr == "" {
			parseErrors = append(parseErrors, ParseError{
				Row:     rowNum,
				Field:   "current value",
				Message: fmt.Sprintf("Row %d: missing 'current value' for slab serial %s", rowNum, slabSerial),
			})
			continue
		}
		currentValue, err := strconv.ParseFloat(cvStr, 64)
		if err != nil {
			parseErrors = append(parseErrors, ParseError{
				Row:     rowNum,
				Field:   "current value",
				Message: fmt.Sprintf("Row %d: invalid 'current value' %q for slab serial %s", rowNum, cvStr, slabSerial),
			})
			continue
		}

		var population int
		if pop := getField(colIdx("population")); pop != "" {
			population, _ = strconv.Atoi(pop) //nolint:errcheck // best-effort; zero is fine
		}

		clRows = append(clRows, CLExportRow{
			SlabSerial:   slabSerial,
			Card:         getField(colIdx("card")),
			Set:          getField(colIdx("set")),
			Number:       getField(colIdx("number")),
			CurrentValue: currentValue,
			Population:   population,
		})
	}

	return clRows, parseErrors, nil
}

// ParseCLImportRows parses CSV records from a Card Ladder export for full import
// (auto-allocation + refresh). The first row must be the header row. Returns parsed
// rows, any parse errors, and a fatal error if the CSV structure is invalid.
func ParseCLImportRows(records [][]string) ([]CLExportRow, []ParseError, error) {
	if len(records) < 2 {
		return nil, nil, fmt.Errorf("CSV must have a header row and at least one data row")
	}

	headerMap := BuildHeaderMap(records[0])

	requiredHeaders := []string{"slab serial #", "investment", "current value"}
	for _, hdr := range requiredHeaders {
		if _, ok := headerMap[hdr]; !ok {
			return nil, nil, fmt.Errorf("missing required column: %q", hdr)
		}
	}

	colIdx := func(name string) int {
		if idx, ok := headerMap[name]; ok {
			return idx
		}
		return -1
	}

	var clRows []CLExportRow
	var parseErrors []ParseError
	for i, rec := range records[1:] {
		rowNum := i + 2

		getField := func(idx int) string {
			if idx >= 0 && idx < len(rec) {
				return strings.TrimSpace(rec[idx])
			}
			return ""
		}

		slabSerial := getField(colIdx("slab serial #"))
		if slabSerial == "" {
			parseErrors = append(parseErrors, ParseError{
				Row:     rowNum,
				Field:   "slab serial #",
				Message: fmt.Sprintf("Row %d: missing Slab Serial #", rowNum),
			})
			continue
		}

		investmentStr := getField(colIdx("investment"))
		investment, err := strconv.ParseFloat(investmentStr, 64)
		if err != nil {
			parseErrors = append(parseErrors, ParseError{
				Row:     rowNum,
				Field:   "investment",
				Message: fmt.Sprintf("Row %d: invalid Investment %q", rowNum, investmentStr),
			})
			continue
		}

		cvStr := getField(colIdx("current value"))
		if cvStr == "" {
			parseErrors = append(parseErrors, ParseError{
				Row:     rowNum,
				Field:   "current value",
				Message: fmt.Sprintf("Row %d: missing Current Value", rowNum),
			})
			continue
		}
		currentValue, err := strconv.ParseFloat(cvStr, 64)
		if err != nil {
			parseErrors = append(parseErrors, ParseError{
				Row:     rowNum,
				Field:   "current value",
				Message: fmt.Sprintf("Row %d: invalid Current Value %q", rowNum, cvStr),
			})
			continue
		}

		var population int
		if pop := getField(colIdx("population")); pop != "" {
			population, _ = strconv.Atoi(pop) //nolint:errcheck // best-effort; zero is fine
		}

		datePurchased := getField(colIdx("date purchased"))
		if datePurchased != "" {
			converted, dateErr := ParseCLDate(datePurchased)
			if dateErr != nil {
				parseErrors = append(parseErrors, ParseError{
					Row:     rowNum,
					Field:   "date purchased",
					Message: fmt.Sprintf("Row %d: invalid Date Purchased %q: expected M/D/YYYY", rowNum, datePurchased),
				})
				continue
			}
			datePurchased = converted
		}

		clRows = append(clRows, CLExportRow{
			DatePurchased: datePurchased,
			Card:          getField(colIdx("card")),
			Player:        getField(colIdx("player")),
			Set:           getField(colIdx("set")),
			Number:        getField(colIdx("number")),
			Condition:     getField(colIdx("condition")),
			Investment:    investment,
			CurrentValue:  currentValue,
			SlabSerial:    slabSerial,
			Population:    population,
		})
	}

	return clRows, parseErrors, nil
}
