// Package recovery defines the domain-neutral contract used by services to
// describe a tool failure that can be safely resolved by an explicit user
// decision. It deliberately has no dependency on assistant orchestration.
package recovery

import (
	"errors"
	"strings"
)

type Category string

const (
	CategoryAutomaticCorrection        Category = "automatic_correction"
	CategoryNeedsInput                 Category = "needs_input"
	CategoryRecoverableBusinessBlocker Category = "recoverable_business_blocker"
	CategoryAuthorizationRequired      Category = "authorization_required"
	CategoryTransientDependency        Category = "transient_dependency"
	CategoryTerminalFailure            Category = "terminal_failure"
)

type Action struct {
	ID                   string `json:"id"`
	Label                string `json:"label"`
	Description          string `json:"description,omitempty"`
	RiskLevel            string `json:"risk_level"`
	Executor             string `json:"executor,omitempty"`
	ConfirmationRequired bool   `json:"confirmation_required,omitempty"`
	ResumesRun           bool   `json:"resumes_run,omitempty"`
	InputRequired        bool   `json:"input_required,omitempty"`
	RetrySafe            bool   `json:"retry_safe,omitempty"`
	KeepsOpen            bool   `json:"keeps_open,omitempty"`
}

type Descriptor struct {
	Code      string                 `json:"code"`
	Category  Category               `json:"category"`
	Summary   string                 `json:"summary"`
	Detail    string                 `json:"detail,omitempty"`
	RiskLevel string                 `json:"risk_level"`
	Context   map[string]interface{} `json:"context,omitempty"`
	Actions   []Action               `json:"actions"`
}

// DescribedError is implemented only by failures for which the backend can
// safely offer explicit recovery actions. Unknown errors fail closed.
type DescribedError interface {
	error
	RecoveryDescriptor() Descriptor
}

func Describe(err error) (Descriptor, bool) {
	var described DescribedError
	if err == nil || !errors.As(err, &described) {
		return Descriptor{}, false
	}
	descriptor := described.RecoveryDescriptor()
	if strings.TrimSpace(descriptor.Code) == "" ||
		strings.TrimSpace(string(descriptor.Category)) == "" ||
		strings.TrimSpace(descriptor.Summary) == "" ||
		len(descriptor.Actions) == 0 {
		return Descriptor{}, false
	}
	if !interactiveCategory(descriptor.Category) {
		return Descriptor{}, false
	}
	seen := make(map[string]bool, len(descriptor.Actions))
	for _, action := range descriptor.Actions {
		actionID := strings.TrimSpace(action.ID)
		if actionID == "" ||
			strings.TrimSpace(action.Label) == "" ||
			strings.TrimSpace(action.RiskLevel) == "" ||
			seen[actionID] {
			return Descriptor{}, false
		}
		if action.ResumesRun && strings.TrimSpace(action.Executor) == "" {
			return Descriptor{}, false
		}
		seen[actionID] = true
	}
	return descriptor, true
}

func interactiveCategory(category Category) bool {
	switch category {
	case CategoryNeedsInput,
		CategoryRecoverableBusinessBlocker,
		CategoryAuthorizationRequired,
		CategoryTransientDependency:
		return true
	default:
		return false
	}
}
