package correlation

import (
	"time"
)

type CorrelationSpec struct {
	ID          string
	PackageID   string
	Requires    []string
	Correlation CorrelationClause
	Alert       AlertSpec
}

type CorrelationClause struct {
	By       string
	Window   time.Duration
	Ordered  bool
	Sequence []SequenceStep
}

type SequenceStep struct {
	RuleID string
}

type AlertSpec struct {
	Title    string
	Severity string
	MitreID  string
	CVEID    string
}
