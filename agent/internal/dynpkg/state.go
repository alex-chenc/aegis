package dynpkg

import (
	"fmt"
	"sync"
	"time"
)

type PackageState string

const (
	StatePending            PackageState = "pending"
	StateDownloading        PackageState = "downloading"
	StateVerifying          PackageState = "verifying"
	StateCheckingAllowlist  PackageState = "checking_allowlist"
	StateInstalling         PackageState = "installing"
	StateActive             PackageState = "active"
	StateDegraded           PackageState = "degraded"
	StateDisabledByRate     PackageState = "disabled_by_rate"
	StateDisabledByPolicy   PackageState = "disabled_by_policy"
	StateSignatureFailed    PackageState = "signature_failed"
	StateBlockedByAllowlist PackageState = "blocked_by_hook_allowlist"
	StateLoadFailed         PackageState = "load_failed"
	StateUninstalled        PackageState = "uninstalled"
	StateRolledBack         PackageState = "rolled_back"
	StateReviewRejected     PackageState = "review_rejected"
)

var validTransitions = map[PackageState][]PackageState{
	StatePending:           {StateDownloading},
	StateDownloading:       {StateVerifying, StateLoadFailed},
	StateVerifying:         {StateCheckingAllowlist, StateSignatureFailed},
	StateCheckingAllowlist: {StateInstalling, StateBlockedByAllowlist},
	StateInstalling:        {StateActive, StateLoadFailed},
	StateActive:            {StateDegraded, StateDisabledByRate, StateDisabledByPolicy, StateUninstalled},
	StateDegraded:          {StateDisabledByRate, StateDisabledByPolicy, StateUninstalled},
	StateDisabledByRate:    {StateDisabledByPolicy, StateUninstalled},
	StateDisabledByPolicy:  {StateUninstalled},
	StateRolledBack:        {StateUninstalled},
	StateReviewRejected:    {StatePending},
}

type TransitionRecord struct {
	From      PackageState
	To        PackageState
	Timestamp time.Time
	Reason    string
}

type StateMachine struct {
	Current PackageState
	History []TransitionRecord
	mu      sync.Mutex
}

func NewStateMachine(initial PackageState) *StateMachine {
	return &StateMachine{
		Current: initial,
		History: []TransitionRecord{},
	}
}

func (sm *StateMachine) CanTransition(to PackageState) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	allowed, exists := validTransitions[sm.Current]
	if !exists {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

func (sm *StateMachine) Transition(to PackageState, reason string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	allowed, exists := validTransitions[sm.Current]
	if !exists {
		return fmt.Errorf("no valid transitions from state %s", sm.Current)
	}
	valid := false
	for _, s := range allowed {
		if s == to {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid transition from %s to %s", sm.Current, to)
	}

	record := TransitionRecord{
		From:      sm.Current,
		To:        to,
		Timestamp: time.Now(),
		Reason:    reason,
	}
	sm.History = append(sm.History, record)
	sm.Current = to
	return nil
}

func (sm *StateMachine) GetHistory() []TransitionRecord {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	result := make([]TransitionRecord, len(sm.History))
	copy(result, sm.History)
	return result
}
