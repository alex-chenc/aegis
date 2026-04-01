package alert_generator

import (
	"dc/internal/block_manager"
	"dc/internal/model"
	"dc/pkg/logger"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type AlertGenerator struct {
	blockManager *block_manager.BlockManager
	logger       *zap.Logger
}

func NewAlertGenerator(blockMgr *block_manager.BlockManager) *AlertGenerator {
	return &AlertGenerator{
		blockManager: blockMgr,
		logger:       logger.Get(),
	}
}

func (g *AlertGenerator) GenerateAlert(event *model.RuntimeEvent) *model.Alert {
	if event.MatchedRuleID == "" {
		return nil
	}

	alert := &model.Alert{
		AlertID:     "ALT-" + uuid.New().String()[:8],
		HostID:      event.HostID,
		Severity:    event.Severity,
		Status:      "pending",
		Description: g.generateDescription(event),
		MitreID:     event.MitreID,
		RuleID:      event.MatchedRuleID,
		HitCount:    1,
		FirstSeenAt: time.Now(),
		LastSeenAt:  time.Now(),
		ProcessName: event.ProcessName,
		CommandLine: event.CommandLine,
	}

	// Check auto-block policy
	if g.blockManager.ShouldAutoBlock(alert.MitreID) {
		alert.AutoDispose = true
		alert.Status = "resolved"
		g.logger.Info("Alert marked for auto-dispose",
			zap.String("alert_id", alert.AlertID),
			zap.String("mitre_id", alert.MitreID),
		)
	}

	g.logger.Info("Alert generated",
		zap.String("alert_id", alert.AlertID),
		zap.String("severity", alert.Severity),
		zap.String("mitre_id", alert.MitreID),
	)

	return alert
}

func (g *AlertGenerator) generateDescription(event *model.RuntimeEvent) string {
	if event.ProcessName != "" {
		return event.ProcessName + " triggered rule " + event.MatchedRuleID
	}
	return "Event triggered rule " + event.MatchedRuleID
}