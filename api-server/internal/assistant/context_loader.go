package assistant

import (
	"context"
	"fmt"

	"api-server/internal/repository"
	"github.com/google/uuid"
)

// ContextLoader 上下文加载器（对齐设计文档 9 节）
type ContextLoader struct {
	hostRepo       *repository.HostRepository
	alertRepo      *repository.AlertRepository
	taskRepo       *repository.TaskLogRepository
	vulnRepo       *repository.VulnerabilityRepo
	contextRefRepo repository.AssistantContextRefRepository
}

// ContextLoaderDeps 上下文加载器依赖
type ContextLoaderDeps struct {
	HostRepo       *repository.HostRepository
	AlertRepo      *repository.AlertRepository
	TaskRepo       *repository.TaskLogRepository
	VulnRepo       *repository.VulnerabilityRepo
	ContextRefRepo repository.AssistantContextRefRepository
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
		hostRepo:       deps.HostRepo,
		alertRepo:      deps.AlertRepo,
		taskRepo:       deps.TaskRepo,
		vulnRepo:       deps.VulnRepo,
		contextRefRepo: deps.ContextRefRepo,
	}
}

// ResolveSession 解析会话上下文
func (l *ContextLoader) ResolveSession(ctx context.Context, sessionID string) ([]ContextObject, error) {
	if l.contextRefRepo == nil {
		return nil, nil
	}

	refs, err := l.contextRefRepo.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	var objects []ContextObject
	for _, ref := range refs {
		obj := ContextObject{
			ObjectType: ref.ObjectType,
			ObjectID:   ref.ObjectID,
			Title:      ref.Title,
			Summary:    ref.Summary,
			RoutePath:  ref.RoutePath,
			Data:       unmarshalJSON(ref.Snapshot),
		}
		objects = append(objects, obj)
	}

	return objects, nil
}

// Resolve 解析上下文对象（对齐设计文档 9 节）
func (l *ContextLoader) Resolve(ctx context.Context, objectType, objectID string) (*ContextObject, error) {
	switch objectType {
	case "host":
		return l.resolveHost(ctx, objectID)
	case "alert":
		return l.resolveAlert(ctx, objectID)
	case "task":
		return l.resolveTask(ctx, objectID)
	case "vulnerability", "vuln":
		return l.resolveVulnerability(ctx, objectID)
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

	parsedID, err := uuid.Parse(hostID)
	if err != nil {
		return nil, fmt.Errorf("invalid host ID: %w", err)
	}

	host, err := l.hostRepo.FindByID(parsedID)
	if err != nil {
		return nil, err
	}

	return &ContextObject{
		ObjectType: "host",
		ObjectID:   hostID,
		Title:      host.Hostname,
		Summary:    fmt.Sprintf("IP: %s, OS: %s", host.IPAddress, host.OSType),
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
		Title:      alert.RuleTitle,
		Summary:    fmt.Sprintf("Severity: %s, Host: %s", alert.Severity, alert.HostID),
		RoutePath:  "/detection/alerts",
		Data:       alert,
	}, nil
}

func (l *ContextLoader) resolveTask(ctx context.Context, taskID string) (*ContextObject, error) {
	if l.taskRepo == nil {
		return nil, fmt.Errorf("task repo not available")
	}

	parsedID, err := uuid.Parse(taskID)
	if err != nil {
		return nil, fmt.Errorf("invalid task ID: %w", err)
	}

	task, err := l.taskRepo.FindByID(parsedID)
	if err != nil {
		return nil, err
	}

	return &ContextObject{
		ObjectType: "task",
		ObjectID:   taskID,
		Title:      fmt.Sprintf("Task: %s", task.TaskType),
		Summary:    fmt.Sprintf("Status: %s", task.Status),
		RoutePath:  "/baseline/tasks/" + taskID,
		Data:       task,
	}, nil
}

func (l *ContextLoader) resolveVulnerability(ctx context.Context, cveID string) (*ContextObject, error) {
	if l.vulnRepo == nil {
		return nil, fmt.Errorf("vulnerability repo not available")
	}

	vuln, err := l.vulnRepo.FindByCveID(cveID)
	if err != nil {
		return nil, err
	}

	scoreStr := ""
	if vuln.CvssScore != nil {
		scoreStr = fmt.Sprintf(", CVSS: %.1f", *vuln.CvssScore)
	}

	return &ContextObject{
		ObjectType: "vulnerability",
		ObjectID:   cveID,
		Title:      vuln.CveID,
		Summary:    fmt.Sprintf("Severity: %s%s", vuln.Severity, scoreStr),
		RoutePath:  "/vulnerabilities",
		Data:       vuln,
	}, nil
}
