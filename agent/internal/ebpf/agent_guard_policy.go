package ebpf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"

	"aegis-agent/internal/agentguard"

	"github.com/cilium/ebpf"
)

type lsmGuardSubject struct {
	InstanceSlot uint64
	UnitSlot     uint64
	PolicySlot   uint64
	ProcessEpoch uint64
	Flags        uint32
	Padding      uint32
}

// lsmPolicyPathKey exactly mirrors policy_path_key in agent_guard_lsm.bpf.c.
// Data begins immediately after prefixlen; no compiler padding can enter the
// LPM prefix. The first 8 data bytes are native-endian policy_slot.
type lsmPolicyPathKey struct {
	PrefixLen uint32
	Data      [264]byte
}

type lsmPathRuleValue struct {
	RuleSlot      uint64
	OperationMask uint32
	Action        uint32
	MatchKind     uint32
	PathLen       uint32
}

type lsmEscapePolicyValue struct {
	Actions   [8]uint32
	RuleSlots [8]uint64
}

var agentGuardPolicyUpdateMu sync.Mutex

func (l *Loader) ApplyAgentGuardKernelPolicy(
	policy agentguard.CompiledKernelPolicy,
	subjects []agentguard.KernelSubject,
) error {
	agentGuardPolicyUpdateMu.Lock()
	defer agentGuardPolicyUpdateMu.Unlock()
	collection := l.collections["agent_guard_lsm"]
	if collection == nil {
		return errors.New("agent_guard_lsm_not_loaded")
	}
	guarded := collection.Maps["guarded_pids"]
	paths := collection.Maps["path_rules"]
	escape := collection.Maps["unit_escape_policies"]
	if guarded == nil || paths == nil || escape == nil {
		return errors.New("agent_guard_lsm_maps_missing")
	}
	// Fail open during replacement: remove subjects before changing rules, and
	// expose the new policy only after every rule map has been populated.
	if err := clearBPFMap[uint32, lsmGuardSubject](guarded); err != nil {
		return fmt.Errorf("clear guarded pids: %w", err)
	}
	if err := clearBPFMap[lsmPolicyPathKey, lsmPathRuleValue](paths); err != nil {
		return fmt.Errorf("clear path rules: %w", err)
	}
	if err := clearBPFMap[uint64, lsmEscapePolicyValue](escape); err != nil {
		return fmt.Errorf("clear escape policies: %w", err)
	}
	for _, rule := range policy.PathRules {
		key, err := buildLSMPolicyPathKey(rule.PolicySlot, rule.Path)
		if err != nil {
			return err
		}
		value := lsmPathRuleValue{
			RuleSlot: rule.RuleSlot, OperationMask: rule.OperationMask,
			Action: uint32(rule.Action), MatchKind: rule.Match, PathLen: uint32(len(rule.Path)),
		}
		if err := paths.Put(key, value); err != nil {
			return fmt.Errorf("agent_guard_lsm_path_rule_update_failed: %w", err)
		}
	}
	escapeByUnit := make(map[uint64]lsmEscapePolicyValue)
	for _, subject := range subjects {
		value := escapeByUnit[subject.UnitSlot]
		for _, rule := range policy.EscapeRules {
			if rule.PolicySlot != subject.PolicySlot || rule.Rule != "load_bpf_or_module" {
				continue
			}
			value.Actions[0] = uint32(rule.Action)
			value.RuleSlots[0] = rule.RuleSlot
		}
		escapeByUnit[subject.UnitSlot] = value
	}
	for unitSlot, value := range escapeByUnit {
		if err := escape.Put(unitSlot, value); err != nil {
			return fmt.Errorf("agent_guard_lsm_escape_rule_update_failed: %w", err)
		}
	}
	for _, subject := range subjects {
		value := lsmGuardSubject{
			InstanceSlot: subject.InstanceSlot, UnitSlot: subject.UnitSlot,
			PolicySlot: subject.PolicySlot, ProcessEpoch: subject.ProcessEpoch,
		}
		if err := guarded.Put(subject.PID, value); err != nil {
			_ = clearBPFMap[uint32, lsmGuardSubject](guarded)
			return fmt.Errorf("agent_guard_lsm_subject_update_failed: %w", err)
		}
	}
	return nil
}

func buildLSMPolicyPathKey(policySlot uint64, path string) (lsmPolicyPathKey, error) {
	if path == "" || len(path) > 255 {
		return lsmPolicyPathKey{}, errors.New("agent_guard_lsm_path_invalid")
	}
	key := lsmPolicyPathKey{PrefixLen: uint32(64 + len(path)*8)}
	binary.LittleEndian.PutUint64(key.Data[:8], policySlot)
	copy(key.Data[8:], path)
	return key, nil
}

func clearBPFMap[K comparable, V any](target *ebpf.Map) error {
	iterator := target.Iterate()
	var key K
	var value V
	keys := make([]K, 0)
	for iterator.Next(&key, &value) {
		keys = append(keys, key)
	}
	if err := iterator.Err(); err != nil {
		return err
	}
	for _, item := range keys {
		if err := target.Delete(item); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
			return err
		}
	}
	return nil
}

func (c *Collector) ApplyAgentGuardKernelPolicy(
	policy agentguard.CompiledKernelPolicy,
	subjects []agentguard.KernelSubject,
) error {
	c.mu.RLock()
	loader := c.loader
	c.mu.RUnlock()
	if loader == nil {
		return errors.New("agent_guard_lsm_not_loaded")
	}
	return loader.ApplyAgentGuardKernelPolicy(policy, subjects)
}
