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
	waitingApproval  *WaitingApprovalState
	waitingRecovery  *WaitingRecoveryState
	approvalWaiters  map[string]chan ApprovalDecision
	rejectedApproval *ApprovalDecision
	waitingMu        sync.RWMutex

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

// WaitingRecoveryState records the durable recovery request that stopped the
// agent-runtime loop. Unlike approval, recovery is resumed through a new run.
type WaitingRecoveryState struct {
	RecoveryID  string    `json:"recovery_id"`
	ToolCallID  string    `json:"tool_call_id"`
	ToolName    string    `json:"tool_name"`
	RequestedAt time.Time `json:"requested_at"`
}

// ApprovalDecision 表示用户对一次工具审批的处理结果。
type ApprovalDecision struct {
	ApprovalID string
	Approved   bool
	Operator   string
	Comment    string
	DecidedAt  time.Time
}

// SetWaitingApproval 设置审批等待状态
func (r *ActiveRun) SetWaitingApproval(state *WaitingApprovalState) {
	r.waitingMu.Lock()
	defer r.waitingMu.Unlock()
	if r.approvalWaiters == nil {
		r.approvalWaiters = make(map[string]chan ApprovalDecision)
	}
	if _, ok := r.approvalWaiters[state.ApprovalID]; !ok {
		r.approvalWaiters[state.ApprovalID] = make(chan ApprovalDecision, 1)
	}
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
	if r.waitingApproval != nil && r.approvalWaiters != nil {
		delete(r.approvalWaiters, r.waitingApproval.ApprovalID)
	}
	r.waitingApproval = nil
}

// SetWaitingRecovery marks the current run as stopped by a recoverable blocker.
func (r *ActiveRun) SetWaitingRecovery(state *WaitingRecoveryState) {
	if state == nil || state.RecoveryID == "" {
		return
	}
	r.waitingMu.Lock()
	defer r.waitingMu.Unlock()
	if r.waitingRecovery == nil {
		copy := *state
		r.waitingRecovery = &copy
	}
}

// GetWaitingRecovery returns a copy of the recovery pause state.
func (r *ActiveRun) GetWaitingRecovery() *WaitingRecoveryState {
	r.waitingMu.RLock()
	defer r.waitingMu.RUnlock()
	if r.waitingRecovery == nil {
		return nil
	}
	copy := *r.waitingRecovery
	return &copy
}

// WaitForApproval 等待用户审批当前工具调用。
func (r *ActiveRun) WaitForApproval(ctx context.Context, approvalID string) (ApprovalDecision, error) {
	r.waitingMu.RLock()
	ch, ok := r.approvalWaiters[approvalID]
	r.waitingMu.RUnlock()
	if !ok {
		return ApprovalDecision{}, fmt.Errorf("approval %s is not waiting", approvalID)
	}

	select {
	case decision := <-ch:
		return decision, nil
	case <-ctx.Done():
		return ApprovalDecision{}, ctx.Err()
	}
}

// ResolveApproval 将审批结果通知给正在等待的工具调用。
func (r *ActiveRun) ResolveApproval(decision ApprovalDecision) bool {
	r.waitingMu.RLock()
	ch, ok := r.approvalWaiters[decision.ApprovalID]
	r.waitingMu.RUnlock()
	if !ok {
		return false
	}
	if decision.DecidedAt.IsZero() {
		decision.DecidedAt = time.Now()
	}
	select {
	case ch <- decision:
		return true
	default:
		return false
	}
}

// MarkApprovalRejected 记录审批拒绝并取消当前运行。
func (r *ActiveRun) MarkApprovalRejected(decision ApprovalDecision) {
	r.waitingMu.Lock()
	if decision.DecidedAt.IsZero() {
		decision.DecidedAt = time.Now()
	}
	r.rejectedApproval = &decision
	r.waitingMu.Unlock()
	r.cancel()
}

// RejectedApproval 获取审批拒绝状态。
func (r *ActiveRun) RejectedApproval() *ApprovalDecision {
	r.waitingMu.RLock()
	defer r.waitingMu.RUnlock()
	if r.rejectedApproval == nil {
		return nil
	}
	decision := *r.rejectedApproval
	return &decision
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
		RunID:           "run_" + uuid.New().String()[:8],
		SessionID:       sessionID,
		ctx:             ctx,
		cancel:          cancel,
		events:          make(chan AssistantEvent, 100),
		startedAt:       time.Now(),
		approvalWaiters: make(map[string]chan ApprovalDecision),
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

// History 获取事件历史
func (r *ActiveRun) History() []AssistantEvent {
	r.subMu.RLock()
	defer r.subMu.RUnlock()
	result := make([]AssistantEvent, len(r.history))
	copy(result, r.history)
	return result
}
