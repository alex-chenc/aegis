package block_manager

import (
	"context"

	"dc/internal/model"
	"dc/internal/repository"
	"dc/pkg/logger"

	"go.uber.org/zap"
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

// LoadPolicies loads all block policies from the database into memory
func (m *BlockManager) LoadPolicies(ctx context.Context, repo *repository.BlockPolicyRepository) error {
	policies, err := repo.FindAll()
	if err != nil {
		logger.Error("Failed to load block policies from database", zap.Error(err))
		return err
	}

	for _, policy := range policies {
		// StorePolicy expects a pointer, so take the address
		m.StorePolicy(&policy)
		logger.Debug("Loaded block policy",
			zap.String("mitre_id", policy.MitreID),
			zap.Bool("enabled", policy.Enabled),
			zap.Bool("auto_dispose", policy.AutoDispose),
		)
	}

	logger.Info("Loaded block policies", zap.Int("count", len(policies)))
	return nil
}