package service

import (
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

	"ai-benchmark/backend/internal/models"
	"ai-benchmark/backend/internal/repository"
	pb "ai-benchmark/backend/proto/agent_comm"
)

type TaskStatus string

const (
	TaskStatusPending TaskStatus = "PENDING"
	TaskStatusRunning TaskStatus = "RUNNING"
	TaskStatusSuccess TaskStatus = "SUCCESS"
	TaskStatusFailed  TaskStatus = "FAILED"
	TaskStatusTimeout TaskStatus = "TIMEOUT"
)

type TaskType string

const (
	TaskTypeCheck TaskType = "CHECK"
	TaskTypeFix   TaskType = "FIX"
)

type Task struct {
	ID         uuid.UUID
	RuleID     uuid.UUID
	HostID     uuid.UUID
	Type       TaskType
	Status     TaskStatus
	CommandID  string
	Script     string
	StartedAt  time.Time
	FinishedAt time.Time
	Stdout     string
	Stderr     string
	ExitCode   int
}

type TaskEventListener interface {
	OnTaskStatusChange(taskID string, status string, stdout, stderr string)
}

type TaskService struct {
	repo         *repository.Repository
	agentHandler AgentCommander
	tasks        map[uuid.UUID]*Task
	mu           sync.RWMutex
	listener     TaskEventListener
	autoFix      bool
}

type AgentCommander interface {
	SendCommand(hostID string, cmd *pb.ServerCommand) error
	IsAgentConnected(hostID string) bool
}

func NewTaskService(repo *repository.Repository, agentHandler AgentCommander) *TaskService {
	return &TaskService{
		repo:         repo,
		agentHandler: agentHandler,
		tasks:        make(map[uuid.UUID]*Task),
		autoFix:      true,
	}
}

func (s *TaskService) SetListener(listener TaskEventListener) {
	s.listener = listener
}

func (s *TaskService) SetAutoFix(enabled bool) {
	s.autoFix = enabled
}

func (s *TaskService) ExecuteCheck(ruleID, hostID uuid.UUID) (*Task, error) {
	rule, err := s.repo.GetBaselineRuleByID(ruleID)
	if err != nil {
		return nil, err
	}

	if rule.GeneratedCheckScript == "" {
		return nil, ErrScriptNotGenerated
	}

	return s.executeTask(ruleID, hostID, TaskTypeCheck, rule.GeneratedCheckScript)
}

func (s *TaskService) ExecuteFix(ruleID, hostID uuid.UUID) (*Task, error) {
	rule, err := s.repo.GetBaselineRuleByID(ruleID)
	if err != nil {
		return nil, err
	}

	if rule.GeneratedFixScript == "" {
		return nil, ErrScriptNotGenerated
	}

	return s.executeTask(ruleID, hostID, TaskTypeFix, rule.GeneratedFixScript)
}

func (s *TaskService) executeTask(ruleID, hostID uuid.UUID, taskType TaskType, script string) (*Task, error) {
	hostIDStr := hostID.String()

	if s.agentHandler == nil || !s.agentHandler.IsAgentConnected(hostIDStr) {
		return nil, ErrAgentNotConnected
	}

	taskID := uuid.New()
	commandID := taskID.String()

	task := &Task{
		ID:        taskID,
		RuleID:    ruleID,
		HostID:    hostID,
		Type:      taskType,
		Status:    TaskStatusPending,
		CommandID: commandID,
		Script:    script,
		StartedAt: time.Now(),
	}

	s.mu.Lock()
	s.tasks[taskID] = task
	s.mu.Unlock()

	cmd := &pb.ServerCommand{
		CommandId:      commandID,
		Type:           pb.CommandType_EXEC_SCRIPT,
		ScriptContent:  script,
		TimeoutSeconds: 300,
	}

	if err := s.agentHandler.SendCommand(hostIDStr, cmd); err != nil {
		task.Status = TaskStatusFailed
		task.Stderr = err.Error()
		task.FinishedAt = time.Now()
		return task, err
	}

	task.Status = TaskStatusRunning

	taskLog := &models.TaskLog{
		RuleID:    ruleID,
		HostID:    hostID,
		TaskType:  string(taskType),
		Status:    string(TaskStatusRunning),
		StartedAt: task.StartedAt,
	}
	taskLog.ID = taskID
	s.repo.CreateTaskLog(taskLog)

	log.Printf("[TaskService] Task %s (%s) started on host %s", taskID, taskType, hostID)

	return task, nil
}

func (s *TaskService) HandleCommandResult(commandID string, status pb.CommandStatus, stdout, stderr string, exitCode int32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, task := range s.tasks {
		if task.CommandID == commandID {
			task.Stdout = stdout
			task.Stderr = stderr
			task.ExitCode = int(exitCode)
			task.FinishedAt = time.Now()

			switch status {
			case pb.CommandStatus_SUCCESS:
				task.Status = TaskStatusSuccess
			case pb.CommandStatus_FAILED:
				task.Status = TaskStatusFailed
			case pb.CommandStatus_TIMEOUT:
				task.Status = TaskStatusTimeout
			}

			s.repo.UpdateTaskLog(task.ID, map[string]interface{}{
				"status":      string(task.Status),
				"stdout":      stdout,
				"stderr":      stderr,
				"exit_code":   int(exitCode),
				"finished_at": task.FinishedAt,
			})

			log.Printf("[TaskService] Task %s (%s) completed with status %s, exit_code=%d",
				task.ID, task.Type, task.Status, task.ExitCode)

			if s.listener != nil {
				s.listener.OnTaskStatusChange(task.ID.String(), string(task.Status), stdout, stderr)
			}

			if task.Type == TaskTypeCheck && task.Status == TaskStatusFailed && s.autoFix {
				go s.triggerAutoFix(task)
			}

			return
		}
	}
}

func (s *TaskService) triggerAutoFix(checkTask *Task) {
	log.Printf("[TaskService] Auto-fix triggered for rule %s on host %s", checkTask.RuleID, checkTask.HostID)

	time.Sleep(1 * time.Second)

	fixTask, err := s.ExecuteFix(checkTask.RuleID, checkTask.HostID)
	if err != nil {
		log.Printf("[TaskService] Auto-fix failed to start: %v", err)
		return
	}

	log.Printf("[TaskService] Auto-fix task %s started", fixTask.ID)
}

func (s *TaskService) GetTask(taskID uuid.UUID) (*Task, error) {
	s.mu.RLock()
	if task, ok := s.tasks[taskID]; ok {
		s.mu.RUnlock()
		return task, nil
	}
	s.mu.RUnlock()

	taskLog, err := s.repo.GetTaskLogByID(taskID)
	if err != nil {
		return nil, err
	}

	return &Task{
		ID:         taskLog.ID,
		RuleID:     taskLog.RuleID,
		HostID:     taskLog.HostID,
		Type:       TaskType(taskLog.TaskType),
		Status:     TaskStatus(taskLog.Status),
		Stdout:     taskLog.Stdout,
		Stderr:     taskLog.Stderr,
		ExitCode:   taskLog.ExitCode,
		StartedAt:  taskLog.StartedAt,
		FinishedAt: taskLog.FinishedAt,
	}, nil
}

func (s *TaskService) ListTasks(page, pageSize int, hostID, ruleID *uuid.UUID) ([]models.TaskLog, int64, error) {
	return s.repo.GetTaskLogs(page, pageSize, hostID, ruleID)
}

var (
	ErrAgentNotConnected  = &TaskError{Message: "agent not connected"}
	ErrScriptNotGenerated = &TaskError{Message: "script not generated"}
)

type TaskError struct {
	Message string
}

func (e *TaskError) Error() string {
	return e.Message
}
