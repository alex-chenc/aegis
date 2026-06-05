package assistant

import (
	"context"
	"fmt"

	"github.com/alex-chenc/aegis/api-server/internal/repository"
)

// ContextLoader 上下文加载器
type ContextLoader struct {
	hostRepo    repository.HostRepository
	alertRepo   repository.AlertRepository
	taskRepo    repository.TaskLogRepository
}

// ContextLoaderDeps 上下文加载器依赖
type ContextLoaderDeps struct {
	HostRepo  repository.HostRepository
	AlertRepo repository.AlertRepository
	TaskRepo  repository.TaskLogRepository
}

// ContextObject 上下文对象
type ContextObject struct {
	ObjectType string
	ObjectID   string
	Title      string
	Summary    string
	RoutePath  string
	Data       interface{}
}

// NewContextLoader 创建上下文加载器
func NewContextLoader(deps ContextLoaderDeps) *ContextLoader {
	return &ContextLoader{
		hostRepo:  deps.HostRepo,
		alertRepo: deps.AlertRepo,
		taskRepo:  deps.TaskRepo,
	}
}

// Resolve 解析上下文对象
func (l *ContextLoader) Resolve(ctx context.Context, objectType, objectID string) (*ContextObject, error) {
	switch objectType {
	case "host":
		return l.resolveHost(ctx, objectID)
	case "alert":
		return l.resolveAlert(ctx, objectID)
	case "task":
		return l.resolveTask(ctx, objectID)
	default:
		return &ContextObject{
			ObjectType: objectType,
			ObjectID:   objectID,
			Title:      fmt.Sprintf("%s: %s", objectType, objectID),
		}, nil
	}
}

func (l *ContextLoader) resolveHost(ctx context.Context, hostID string) (*ContextObject, error) {
	if l.hostRepo == nil {
		return nil, fmt.Errorf("host repo not available")
	}

	host, err := l.hostRepo.FindByID(hostID)
	if err != nil {
		return nil, err
	}

	return &ContextObject{
		ObjectType: "host",
		ObjectID:   hostID,
		Title:      host.Hostname,
		Summary:    fmt.Sprintf("IP: %s, OS: %s", host.IP, host.OS),
		RoutePath:  "/hosts",
		Data:       host,
	}, nil
}

func (l *ContextLoader) resolveAlert(ctx context.Context, alertID string) (*ContextObject, error) {
	if l.alertRepo == nil {
		return nil, fmt.Errorf("alert repo not available")
	}

	alert, err := l.alertRepo.FindByID(alertID)
	if err != nil {
		return nil, err
	}

	return &ContextObject{
		ObjectType: "alert",
		ObjectID:   alertID,
		Title:      alert.Title,
		Summary:    fmt.Sprintf("Severity: %s, Host: %s", alert.Severity, alert.HostID),
		RoutePath:  "/detection/alerts",
		Data:       alert,
	}, nil
}

func (l *ContextLoader) resolveTask(ctx context.Context, taskID string) (*ContextObject, error) {
	if l.taskRepo == nil {
		return nil, fmt.Errorf("task repo not available")
	}

	task, err := l.taskRepo.FindByID(taskID)
	if err != nil {
		return nil, err
	}

	return &ContextObject{
		ObjectType: "task",
		ObjectID:   taskID,
		Title:      fmt.Sprintf("Task: %s", task.Type),
		Summary:    fmt.Sprintf("Status: %s", task.Status),
		RoutePath:  "/baseline/tasks/" + taskID,
		Data:       task,
	}, nil
}
