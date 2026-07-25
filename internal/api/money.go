package api

import (
	"fmt"
	"math"
)

// Reddit reports and accepts monetary values as integer microcurrency: the real
// amount multiplied by 1e6. bid_value 500000 is $0.50. Getting the factor wrong is
// a 1,000,000x budget error, so every conversion goes through this file.
const microFactor = 1_000_000

// DollarsToMicros converts a dollar amount (e.g. 0.50) to microcurrency (500000).
// It rounds to the nearest micro to avoid float drift like 0.1 -> 99999, and rejects
// values that would overflow int64 rather than silently wrapping to a negative budget.
func DollarsToMicros(dollars float64) (int64, error) {
	if math.IsNaN(dollars) || math.IsInf(dollars, 0) {
		return 0, fmt.Errorf("invalid amount %v", dollars)
	}
	micros := math.Round(dollars * microFactor)
	if micros >= math.MaxInt64 || micros <= math.MinInt64 {
		return 0, fmt.Errorf("amount %.2f is out of range", dollars)
	}
	return int64(micros), nil
}

// MicrosToDollars converts microcurrency (500000) back to dollars (0.50).
func MicrosToDollars(micros int64) float64 {
	return float64(micros) / microFactor
}

// FormatMicros renders a microcurrency value as a plain dollar string with two
// decimals, e.g. 500000 -> "0.50". No currency symbol: the account currency is not
// always USD and the caller decides how to label it.
func FormatMicros(micros int64) string {
	return fmt.Sprintf("%.2f", MicrosToDollars(micros))
}

// The CONVERSION_*_TOTAL_VALUE report fields are the exception: they are scaled by
// 100 (cents), not 1e6. These helpers keep that oddity in one place so a caller
// reading a conversion-value field cannot accidentally divide by the wrong factor.
const centsFactor = 100

// CentsToDollars converts a hundredths-scaled value (the CONVERSION_*_TOTAL_VALUE
// report fields) to dollars: 150 -> 1.50.
func CentsToDollars(cents int64) float64 {
	return float64(cents) / centsFactor
}
