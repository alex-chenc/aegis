package repository

import (
	"testing"
	"time"

	"api-server/internal/model"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGetDisplayStatus(t *testing.T) {
	tests := []struct {
		name     string
		session  *model.AISession
		expected string
	}{
		{
			name: "有结论-已完成",
			session: &model.AISession{
				ID:         uuid.New(),
				SessionID:  "test-1",
				Status:     "completed",
				Conclusion: model.JSONB{"verdict": "benign", "summary": "良性活动"},
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			},
			expected: "completed",
		},
		{
			name: "无结论-未完成",
			session: &model.AISession{
				ID:        uuid.New(),
				SessionID: "test-2",
				Status:    "active",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			expected: "active",
		},
		{
			name: "空结论-未完成",
			session: &model.AISession{
				ID:         uuid.New(),
				SessionID:  "test-3",
				Status:     "completed",
				Conclusion: model.JSONB{},
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			},
			expected: "active",
		},
		{
			name: "状态active无结论-未完成",
			session: &model.AISession{
				ID:        uuid.New(),
				SessionID: "test-4",
				Status:    "active",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			expected: "active",
		},
		{
			name: "状态paused无结论-未完成",
			session: &model.AISession{
				ID:        uuid.New(),
				SessionID: "test-5",
				Status:    "paused",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			expected: "active",
		},
		{
			name: "状态cancelled无结论-未完成",
			session: &model.AISession{
				ID:        uuid.New(),
				SessionID: "test-6",
				Status:    "cancelled",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			expected: "active",
		},
		{
			name: "状态active有结论-已完成",
			session: &model.AISession{
				ID:         uuid.New(),
				SessionID:  "test-7",
				Status:     "active",
				Conclusion: model.JSONB{"verdict": "malicious", "summary": "恶意活动"},
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			},
			expected: "completed",
		},
		{
			name: "nil结论-未完成",
			session: &model.AISession{
				ID:        uuid.New(),
				SessionID: "test-8",
				Status:    "completed",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			expected: "active",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetDisplayStatus(tt.session)
			if result != tt.expected {
				t.Errorf("GetDisplayStatus() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestAISessionRepository_FindList_FilterByDisplayStatus(t *testing.T) {
	db := setupSessionTestDB(t)
	repo := NewAISessionRepository(db)

	// 创建测试数据
	sessions := []*model.AISession{
		{
			ID:         uuid.New(),
			SessionID:  "session-completed-1",
			Status:     "completed",
			Conclusion: model.JSONB{"verdict": "benign"},
			CreatedAt:  time.Now().Add(-3 * time.Hour),
			UpdatedAt:  time.Now().Add(-3 * time.Hour),
		},
		{
			ID:         uuid.New(),
			SessionID:  "session-completed-2",
			Status:     "completed",
			Conclusion: model.JSONB{"verdict": "malicious"},
			CreatedAt:  time.Now().Add(-2 * time.Hour),
			UpdatedAt:  time.Now().Add(-2 * time.Hour),
		},
		{
			ID:        uuid.New(),
			SessionID: "session-active-1",
			Status:    "active",
			CreatedAt: time.Now().Add(-1 * time.Hour),
			UpdatedAt: time.Now().Add(-1 * time.Hour),
		},
		{
			ID:        uuid.New(),
			SessionID: "session-paused-1",
			Status:    "paused",
			CreatedAt: time.Now().Add(-30 * time.Minute),
			UpdatedAt: time.Now().Add(-30 * time.Minute),
		},
		{
			ID:        uuid.New(),
			SessionID: "session-cancelled-1",
			Status:    "cancelled",
			CreatedAt: time.Now().Add(-15 * time.Minute),
			UpdatedAt: time.Now().Add(-15 * time.Minute),
		},
	}

	for _, s := range sessions {
		if err := repo.Create(s); err != nil {
			t.Fatalf("failed to create session %s: %v", s.SessionID, err)
		}
	}

	// 测试按completed过滤（有结论的）
	completedSessions, total, err := repo.FindList(1, 10, "completed")
	if err != nil {
		t.Fatalf("FindList(completed) error: %v", err)
	}
	if total != 2 {
		t.Errorf("FindList(completed) total = %d, want 2", total)
	}
	for _, s := range completedSessions {
		displayStatus := GetDisplayStatus(s)
		if displayStatus != "completed" {
			t.Errorf("expected completed session, got %s for %s", displayStatus, s.SessionID)
		}
	}

	// 测试按active过滤（无结论的）
	activeSessions, total, err := repo.FindList(1, 10, "active")
	if err != nil {
		t.Fatalf("FindList(active) error: %v", err)
	}
	if total != 3 {
		t.Errorf("FindList(active) total = %d, want 3", total)
	}
	for _, s := range activeSessions {
		displayStatus := GetDisplayStatus(s)
		if displayStatus != "active" {
			t.Errorf("expected active session, got %s for %s", displayStatus, s.SessionID)
		}
	}

	// 测试不过滤（返回全部）
	allSessions, total, err := repo.FindList(1, 10, "")
	if err != nil {
		t.Fatalf("FindList(all) error: %v", err)
	}
	if total != 5 {
		t.Errorf("FindList(all) total = %d, want 5", total)
	}
	if len(allSessions) != 5 {
		t.Errorf("FindList(all) returned %d sessions, want 5", len(allSessions))
	}
}

func setupSessionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// 使用内存SQLite数据库进行测试
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}

	// 创建表结构
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS ai_analysis_session (
			id TEXT PRIMARY KEY,
			session_id TEXT UNIQUE NOT NULL,
			alert_ids TEXT,
			host_ids TEXT,
			host_filter TEXT,
			time_range TEXT,
			status VARCHAR(20) DEFAULT 'active',
			max_iterations INTEGER DEFAULT 15,
			message_count INTEGER DEFAULT 0,
			tool_call_count INTEGER DEFAULT 0,
			created_at DATETIME,
			updated_at DATETIME,
			concluded_at DATETIME,
			conclusion TEXT
		)
	`).Error; err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	return db
}
