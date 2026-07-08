package service

import "strings"

// AggregateShellGenerationStatus ranks CVE-level shell readiness for list sorting/display.
// Values: generated | generating | none
// Order: generated (0) > generating (1) > none (2)
func AggregateShellGenerationRank(status string) int {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "generated":
		return 0
	case "generating":
		return 1
	default:
		return 2
	}
}

// CompareAggregateShellStatus returns negative if a should appear before b.
func CompareAggregateShellStatus(a, b string) int {
	return AggregateShellGenerationRank(a) - AggregateShellGenerationRank(b)
}
