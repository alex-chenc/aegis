package block_manager

import (
	"dc/internal/model"
	"sync"
)

type BlockManager struct {
	policies sync.Map
}

func NewBlockManager() *BlockManager {
	return &BlockManager{}
}

func (m *BlockManager) LoadPolicy(mitreID string) (*model.BlockPolicy, bool) {
	val, ok := m.policies.Load(mitreID)
	if !ok {
		return nil, false
	}
	return val.(*model.BlockPolicy), true
}

func (m *BlockManager) StorePolicy(policy *model.BlockPolicy) {
	m.policies.Store(policy.MitreID, policy)
}

func (m *BlockManager) ShouldAutoBlock(mitreID string) bool {
	policy, ok := m.LoadPolicy(mitreID)
	if !ok {
		return false
	}
	return policy.Enabled && policy.AutoDispose
}

func (m *BlockManager) IsBlocked(mitreID string) bool {
	policy, ok := m.LoadPolicy(mitreID)
	if !ok {
		return false
	}
	return policy.Enabled
}