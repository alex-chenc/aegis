package agentsession

import (
	"context"
	"sync"
	"time"

	"aegis-agent/internal/logger"

	"go.uber.org/zap"
)

type Reporter interface {
	ReportSessionDeltas(context.Context, []SessionDelta) error
}

type ManagerConfig struct {
	Enabled       bool
	ScanConfig    ScanConfig
	CursorStore   CursorStore
	Redactor      *Redactor
	SpoolPath     string
	SpoolCapacity int
}

type Manager struct {
	cfg      ManagerConfig
	scanner  *Scanner
	reporter Reporter
	spool    *Spool
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func NewManager(cfg ManagerConfig, reporter Reporter) *Manager {
	cfg.ScanConfig = cfg.ScanConfig.withDefaults()
	return &Manager{
		cfg:      cfg,
		scanner:  NewScanner(cfg.ScanConfig, cfg.CursorStore, cfg.Redactor),
		reporter: reporter,
		spool:    NewSpool(cfg.SpoolPath, cfg.SpoolCapacity),
	}
}

func (m *Manager) Start(parent context.Context) error {
	if m == nil || !m.cfg.Enabled {
		return nil
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	m.cancel = cancel
	m.wg.Add(1)
	go m.loop(ctx)
	logger.Info("agent_session_scanner_started",
		zap.Duration("scan_interval", m.cfg.ScanConfig.ScanInterval),
		zap.Int("root_count", len(m.cfg.ScanConfig.Roots)))
	return nil
}

func (m *Manager) Stop() {
	if m == nil {
		return
	}
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
	logger.Info("agent_session_scanner_stopped")
}

// ScanNow performs one bounded static scan immediately. It is used by the
// explicit collection button as well as the periodic loop; cursor state is
// committed only after the redacted batch has been accepted by the reporter.
func (m *Manager) ScanNow(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	m.scanOnce(ctx)
	return nil
}

func (m *Manager) loop(ctx context.Context) {
	defer m.wg.Done()
	// Scan immediately after registration; subsequent scans remain static and
	// bounded, with no Hook or filesystem watcher dependency.
	m.scanOnce(ctx)
	ticker := time.NewTicker(m.cfg.ScanConfig.ScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.scanOnce(ctx)
		}
	}
}

func (m *Manager) scanOnce(ctx context.Context) {
	if m.reporter != nil && m.spool != nil {
		if err := m.spool.Drain(ctx, m.reporter); err != nil {
			logger.Warn("agent_session_spool_replay_deferred", zap.String("error_code", "agent_session_spool_replay_deferred"), zap.Error(err))
			return
		}
	}
	started := time.Now()
	result, err := m.scanner.Scan(ctx)
	if err != nil {
		logger.Warn("agent_session_scan_failed",
			zap.String("error_code", "agent_session_scan_failed"),
			zap.Error(err))
		return
	}
	if result.BudgetExhausted {
		logger.Warn("agent_session_scan_budget_exhausted",
			zap.Int("files_discovered", result.FilesDiscovered),
			zap.Int64("bytes_read", result.BytesRead))
	}
	logger.Debug("agent_session_scan_completed",
		zap.Int("files_discovered", result.FilesDiscovered),
		zap.Int("files_processed", result.FilesProcessed),
		zap.Int("session_count", len(result.Sessions)),
		zap.Int64("bytes_read", result.BytesRead),
		zap.Duration("latency", time.Since(started)))
	if m.reporter == nil || len(result.Sessions) == 0 {
		if err := m.scanner.Commit(result); err != nil {
			logger.Warn("agent_session_cursor_commit_failed", zap.Error(err))
		}
		return
	}
	if err := m.reporter.ReportSessionDeltas(ctx, result.Sessions); err != nil {
		logger.Warn("agent_session_batch_deferred",
			zap.String("error_code", "agent_session_batch_deferred"),
			zap.Int("session_count", len(result.Sessions)),
			zap.Error(err))
		if m.spool != nil {
			if spoolErr := m.spool.Append(result.Sessions); spoolErr != nil {
				logger.Warn("agent_session_spool_write_failed", zap.Error(spoolErr))
			}
		}
		return
	}
	if err := m.scanner.Commit(result); err != nil {
		logger.Warn("agent_session_cursor_commit_failed", zap.String("error_code", "agent_session_cursor_commit_failed"), zap.Error(err))
	}
}
