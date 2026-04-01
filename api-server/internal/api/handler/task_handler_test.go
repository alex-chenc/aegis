package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"api-server/internal/repository"
	"api-server/internal/service"
	"api-server/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTaskHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}

	if err := db.Exec(`
		CREATE TABLE task_logs (
			id TEXT PRIMARY KEY,
			task_group_id TEXT NOT NULL,
			rule_id TEXT NULL,
			host_id TEXT NOT NULL,
			vulnerability_id TEXT NULL,
			task_type TEXT NOT NULL,
			status TEXT NOT NULL,
			script_content TEXT NULL,
			stdout TEXT NULL,
			stderr TEXT NULL,
			exit_code INTEGER NULL,
			started_at DATETIME NULL,
			finished_at DATETIME NULL,
			created_at DATETIME NOT NULL
		)
	`).Error; err != nil {
		t.Fatalf("failed to create task_logs table: %v", err)
	}

	if err := db.Exec(`
		CREATE TABLE hosts (
			id TEXT PRIMARY KEY,
			ip_address TEXT NOT NULL,
			hostname TEXT NOT NULL,
			os_type TEXT NOT NULL,
			agent_version TEXT NOT NULL,
			last_heartbeat_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)
	`).Error; err != nil {
		t.Fatalf("failed to create hosts table: %v", err)
	}

	return db
}

func TestGetTaskLogs_NilRuleID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	if logger.Logger == nil {
		err := logger.Init(&logger.Config{
			Level:      "error",
			MaxSize:    10,
			MaxBackups: 1,
			MaxAge:     1,
			Compress:   false,
		})
		if err != nil {
			t.Fatalf("failed to init logger: %v", err)
		}
	}

	db := setupTaskHandlerTestDB(t)
	taskLogRepo := repository.NewTaskLogRepository(db)
	hostRepo := repository.NewHostRepository(db)
	taskService := service.NewTaskService(taskLogRepo, hostRepo, nil, nil, nil, nil)

	taskGroupID := uuid.New()
	hostID := uuid.New()
	vulnID := uuid.New()
	taskID := uuid.New()

	insertSQL := `
		INSERT INTO task_logs (
			id, task_group_id, rule_id, host_id, vulnerability_id, task_type, status, created_at
		) VALUES (?, ?, NULL, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`
	if err := db.Exec(insertSQL, taskID.String(), taskGroupID.String(), hostID.String(), vulnID.String(), "check", "success").Error; err != nil {
		t.Fatalf("failed to insert task log: %v", err)
	}

	handler := &TaskHandler{
		taskService: taskService,
		taskLogRepo: taskLogRepo,
	}

	router := gin.Default()
	router.GET("/tasks/:id/logs", handler.GetTaskLogs)

	req := httptest.NewRequest(http.MethodGet, "/tasks/"+taskGroupID.String()+"/logs", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}
