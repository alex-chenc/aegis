package tools

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

const (
	defaultBaselineOperationInterval   = 15 * time.Second
	defaultBaselineOperationBatchSize  = 20
	defaultBaselineOperationStaleAfter = 24 * time.Hour
)

// BaselineOperationWorker advances durable baseline operations independently
// from the assistant conversation that created them.
type BaselineOperationWorker struct {
	deps      BaselineToolDeps
	interval  time.Duration
	batchSize int
	logger    *zap.Logger
}

func NewBaselineOperationWorker(deps BaselineToolDeps) *BaselineOperationWorker {
	logger := deps.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return &BaselineOperationWorker{
		deps:      deps,
		interval:  defaultBaselineOperationInterval,
		batchSize: defaultBaselineOperationBatchSize,
		logger:    logger,
	}
}

func (w *BaselineOperationWorker) Start(ctx context.Context) {
	if w == nil || w.deps.OperationRepo == nil {
		return
	}
	w.logger.Info("assistant baseline operation worker started",
		zap.Duration("interval", w.interval),
		zap.Int("batch_size", w.batchSize),
	)
	go w.run(ctx)
}

func (w *BaselineOperationWorker) run(ctx context.Context) {
	w.runOnce(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("assistant baseline operation worker stopped")
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *BaselineOperationWorker) runOnce(ctx context.Context) {
	operations, err := w.deps.OperationRepo.ListNonTerminal(ctx, baselineComplianceOperationType, w.batchSize)
	if err != nil {
		w.logger.Warn("assistant baseline operation scan failed", zap.Error(err))
		return
	}
	for index := range operations {
		if ctx.Err() != nil {
			return
		}
		operation := operations[index]
		if !operation.UpdatedAt.IsZero() && time.Since(operation.UpdatedAt) > defaultBaselineOperationStaleAfter {
			_, _ = failBaselineOperation(ctx, w.deps, &operation, "operation_stale",
				fmt.Errorf("operation made no progress for more than %s", defaultBaselineOperationStaleAfter))
			continue
		}
		operationCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		_, advanceErr := advanceBaselineOperation(operationCtx, w.deps, &operation)
		cancel()
		if advanceErr != nil {
			w.logger.Warn("assistant baseline operation advance failed",
				zap.String("operation_id", operation.ID.String()),
				zap.String("operation_status", operation.Status),
				zap.Error(advanceErr),
			)
		}
	}
}
