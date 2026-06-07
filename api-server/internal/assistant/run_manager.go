package assistant

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// RunManager 运行管理器
type RunManager struct {
	mu   sync.RWMutex
	runs map[string]*ActiveRun // key: sessionID
}

const runEventHistoryLimit = 600

// ActiveRun 活跃运行
type ActiveRun struct {
	RunID     string
	SessionID string
	ctx       context.Context
	cancel    context.CancelFunc
	events    chan AssistantEvent
	startedAt time.Time

	// 审批暂停/恢复状态
	waitingApproval *WaitingApprovalState
	waitingMu       sync.RWMutex

	subscribers []chan AssistantEvent
	history     []AssistantEvent
	subMu       sync.RWMutex
}

// WaitingApprovalState 审批等待状态
type WaitingApprovalState struct {
	ApprovalID  string                 `json:"approval_id"`
	ToolCallID  string                 `json:"tool_call_id"`
	ToolName    string                 `json:"tool_name"`
	Args        map[string]interface{} `json:"args"`
	Operator    string                 `json:"operator"`
	RequestedAt time.Time              `json:"requested_at"`
}

// SetWaitingApproval 设置审批等待状态
func (r *ActiveRun) SetWaitingApproval(state *WaitingApprovalState) {
	r.waitingMu.Lock()
	defer r.waitingMu.Unlock()
	r.waitingApproval = state
}

// GetWaitingApproval 获取审批等待状态
func (r *ActiveRun) GetWaitingApproval() *WaitingApprovalState {
	r.waitingMu.RLock()
	defer r.waitingMu.RUnlock()
	return r.waitingApproval
}

// ClearWaitingApproval 清除审批等待状态
func (r *ActiveRun) ClearWaitingApproval() {
	r.waitingMu.Lock()
	defer r.waitingMu.Unlock()
	r.waitingApproval = nil
}

// NewRunManager 创建运行管理器
func NewRunManager() *RunManager {
	return &RunManager{
		runs: make(map[string]*ActiveRun),
	}
}

// Start 启动新运行
func (m *RunManager) Start(sessionID string) *ActiveRun {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Cancel existing run if any
	if existing, ok := m.runs[sessionID]; ok {
		existing.cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	run := &ActiveRun{
		RunID:     "run_" + uuid.New().String()[:8],
		SessionID: sessionID,
		ctx:       ctx,
		cancel:    cancel,
		events:    make(chan AssistantEvent, 100),
		startedAt: time.Now(),
	}

	m.runs[sessionID] = run
	return run
}

// Get 获取活跃运行
func (m *RunManager) Get(sessionID string) (*ActiveRun, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	run, ok := m.runs[sessionID]
	return run, ok
}

// Context 获取运行上下文
func (r *ActiveRun) Context() context.Context {
	return r.ctx
}

// Publish 发布事件
func (m *RunManager) Publish(sessionID string, event AssistantEvent) {
	m.mu.RLock()
	run, ok := m.runs[sessionID]
	m.mu.RUnlock()

	if !ok {
		return
	}

	run.subMu.Lock()
	run.history = append(run.history, event)
	if len(run.history) > runEventHistoryLimit {
		run.history = append([]AssistantEvent(nil), run.history[len(run.history)-runEventHistoryLimit:]...)
	}
	subscribers := append([]chan AssistantEvent(nil), run.subscribers...)
	run.subMu.Unlock()

	// Send to all subscribers
	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
			// Skip if subscriber is slow
		}
	}
}

// Subscribe 订阅事件
func (m *RunManager) Subscribe(sessionID string) (<-chan AssistantEvent, func(), error) {
	m.mu.RLock()
	run, ok := m.runs[sessionID]
	m.mu.RUnlock()

	if !ok {
		return nil, nil, fmt.Errorf("no active run for session %s", sessionID)
	}

	ch := make(chan AssistantEvent, runEventHistoryLimit+100)

	run.subMu.Lock()
	for _, event := range run.history {
		ch <- event
	}
	run.subscribers = append(run.subscribers, ch)
	run.subMu.Unlock()

	unsubscribe := func() {
		run.subMu.Lock()
		defer run.subMu.Unlock()
		for i, sub := range run.subscribers {
			if sub == ch {
				run.subscribers = append(run.subscribers[:i], run.subscribers[i+1:]...)
				close(ch)
				break
			}
		}
	}

	return ch, unsubscribe, nil
}

// Cancel 取消运行
func (m *RunManager) Cancel(sessionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	run, ok := m.runs[sessionID]
	if !ok {
		return false
	}

	run.cancel()
	return true
}

// Finish 完成运行
func (m *RunManager) Finish(sessionID string) {
	m.mu.Lock()
	run, ok := m.runs[sessionID]
	if ok {
		delete(m.runs, sessionID)
	}
	m.mu.Unlock()

	if ok {
		run.cancel()
		run.subMu.Lock()
		for _, ch := range run.subscribers {
			close(ch)
		}
		run.subscribers = nil
		run.subMu.Unlock()
	}
}

// IsRunning 检查是否正在运行
func (m *RunManager) IsRunning(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.runs[sessionID]
	return ok
}
