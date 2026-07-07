package grpc_server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"server/pkg/logger"
)

// apiServerTaskResultPusher pushes agent task results back to the API Server
// over its internal HTTP endpoint (/internal/task-result). This realizes the
// real-time half of auto-verify: when an agent reports a final result, the
// Server notifies the API Server immediately instead of waiting for the
// API Server's 5s poll of the shared task_logs table.
//
// The poll remains as a safety net for restarts / missed pushes.
type apiServerTaskResultPusher struct {
	endpoint string
	client   *http.Client
}

// NewAPIServerTaskResultPusher builds a TaskResultCallback that POSTs results
// to the given API Server HTTP address (e.g. "http://api-server:8082").
func NewAPIServerTaskResultPusher(apiServerHTTPAddr string) TaskResultCallback {
	base := apiServerHTTPAddr
	if base == "" {
		base = "http://api-server:8082"
	}
	return (&apiServerTaskResultPusher{
		endpoint: base + "/internal/task-result",
		client:   &http.Client{Timeout: 5 * time.Second},
	}).push
}

type apiServerTaskResultPayload struct {
	TaskID   string `json:"task_id"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Status   string `json:"status"`
}

func (p *apiServerTaskResultPusher) push(taskID uuid.UUID, stdout, stderr string, exitCode int, status string) {
	payload := apiServerTaskResultPayload{
		TaskID:   taskID.String(),
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: exitCode,
		Status:   status,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		logger.Error("failed to marshal task result for API Server push",
			zap.String("task_id", taskID.String()),
			zap.Error(err),
		)
		return
	}

	resp, err := p.client.Post(p.endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		// Non-fatal: the API Server's 5s poll will pick up the result from the
		// shared task_logs table.
		logger.Warn("failed to push task result to API Server (poll will retry)",
			zap.String("task_id", taskID.String()),
			zap.String("endpoint", p.endpoint),
			zap.Error(err),
		)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		logger.Warn("API Server rejected task result push (poll will retry)",
			zap.String("task_id", taskID.String()),
			zap.Int("http_status", resp.StatusCode),
		)
		return
	}

	logger.Debug("task result pushed to API Server",
		zap.String("task_id", taskID.String()),
		zap.String("status", status),
	)
}
