package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"
	pb "api-server/pkg/api/v1"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	assetCollectionStatusCollecting = "collecting"
	assetCollectionStatusAnalyzing  = "analyzing"
	assetCollectionStatusCompleted  = "completed"
	assetCollectionStatusFailed     = "failed"
	assetCollectionStatusCancelled  = "cancelled"

	processSnapshotChunkLimit = 100
	processSnapshotMaxCount   = 2000
)

// AssetCollectionService 资产采集服务
type AssetCollectionService struct {
	repo            *repository.AssetCollectionRepository
	serverClient    ServerClientInterface
	analysisService *AssetAnalysisService
	logger          *zap.Logger
}

// ServerClientInterface Server 客户端接口
// TODO: Uncomment after regenerating proto code with protoc
// type ServerClientInterface interface {
// 	CollectHostAssets(ctx context.Context, req *pb.CollectHostAssetsRequest) (*pb.CollectHostAssetsResponse, error)
// }

// Placeholder interface for now
type ServerClientInterface interface {
	ListConnectedAgents(ctx context.Context) (*pb.ListConnectedAgentsResponse, error)
	ExecuteTool(ctx context.Context, callID, hostID, tool, arguments string, timeoutSeconds int32) (*pb.ToolExecuteResponse, error)
}

// NewAssetCollectionService 创建资产采集服务
func NewAssetCollectionService(
	repo *repository.AssetCollectionRepository,
	serverClient ServerClientInterface,
	logger *zap.Logger,
) *AssetCollectionService {
	return &AssetCollectionService{
		repo:         repo,
		serverClient: serverClient,
		logger:       logger,
	}
}

// SetAnalysisService wires the optional LLM application analysis step.
func (s *AssetCollectionService) SetAnalysisService(analysisService *AssetAnalysisService) {
	s.analysisService = analysisService
}

// TriggerAssetCollection 触发资产采集
func (s *AssetCollectionService) TriggerAssetCollection(ctx context.Context, req model.TriggerAssetCollectionRequest, requestedBy string) (*model.AssetCollectionTask, error) {
	s.logger.Info("Triggering asset collection",
		zap.String("scope", req.Scope),
		zap.Strings("host_ids", req.HostIDs),
		zap.Strings("types", req.Types))

	collectTypes := normalizeCollectTypes(req.Types)
	targetHostIDs, err := s.resolveTargetHostIDs(ctx, req)
	if err != nil {
		s.logger.Error("Failed to resolve asset collection target hosts",
			zap.String("scope", req.Scope),
			zap.Strings("host_ids", req.HostIDs),
			zap.Error(err))
		return nil, err
	}

	// 创建任务
	task := &model.AssetCollectionTask{
		ID:            uuid.New(),
		TaskType:      "full",
		TriggerSource: "manual",
		Scope:         req.Scope,
		HostFilter:    mustMarshalJSON(targetHostIDs),
		CollectTypes:  mustMarshalJSON(collectTypes),
		Status:        assetCollectionStatusCollecting,
		CurrentStage:  "process_snapshot",
		RequestedBy:   requestedBy,
	}

	if err := s.repo.CreateTask(task); err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	// 异步执行采集
	go s.executeCollection(context.Background(), task.ID, targetHostIDs, collectTypes)

	return task, nil
}

func (s *AssetCollectionService) resolveTargetHostIDs(ctx context.Context, req model.TriggerAssetCollectionRequest) ([]string, error) {
	if req.Scope != "all_hosts" {
		if len(req.HostIDs) == 0 {
			return nil, fmt.Errorf("host_ids is required for scope %s", req.Scope)
		}
		return req.HostIDs, nil
	}

	if s.serverClient == nil {
		return nil, fmt.Errorf("server client is required to resolve online hosts")
	}

	listCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := s.serverClient.ListConnectedAgents(listCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to list connected agents: %w", err)
	}

	var hostIDs []string
	for _, agent := range resp.Agents {
		if agent.Connected && agent.HostId != "" {
			hostIDs = append(hostIDs, agent.HostId)
		}
	}
	if len(hostIDs) == 0 {
		return nil, fmt.Errorf("no connected hosts available for asset collection")
	}

	s.logger.Info("Resolved asset collection target hosts",
		zap.String("scope", req.Scope),
		zap.Int("count", len(hostIDs)))

	return hostIDs, nil
}

// executeCollection 执行采集任务
func (s *AssetCollectionService) executeCollection(ctx context.Context, taskID uuid.UUID, hostIDs []string, types []string) {
	s.logger.Info("Starting asset collection execution", zap.String("task_id", taskID.String()))

	// 更新任务状态为采集中
	task, err := s.repo.GetTask(taskID)
	if err != nil {
		s.logger.Error("Failed to get task", zap.Error(err))
		return
	}

	now := time.Now()
	task.Status = assetCollectionStatusCollecting
	task.CurrentStage = "process_snapshot"
	task.StartedAt = &now
	task.TotalHosts = len(hostIDs)
	if err := s.repo.UpdateTask(task); err != nil {
		s.logger.Error("Failed to update task", zap.Error(err))
		return
	}

	// 遍历主机执行采集
	successCount := 0
	failCount := 0

	for _, hostID := range hostIDs {
		if err := s.collectHost(ctx, taskID, hostID, types); err != nil {
			s.logger.Error("Failed to collect host",
				zap.String("host_id", hostID),
				zap.Error(err))
			failCount++
		} else {
			successCount++
		}
	}

	// 更新任务状态
	task.SuccessHosts = successCount
	task.FailedHosts = failCount
	finishedAt := time.Now()
	task.FinishedAt = &finishedAt

	if failCount == 0 {
		task.Status = assetCollectionStatusCompleted
		task.CurrentStage = "completed"
	} else {
		task.Status = assetCollectionStatusFailed
		task.CurrentStage = "failed"
		if successCount > 0 {
			task.ErrorMessage = fmt.Sprintf("%d hosts failed", failCount)
		}
	}

	if err := s.repo.UpdateTask(task); err != nil {
		s.logger.Error("Failed to update task final status", zap.Error(err))
	}

	s.logger.Info("Asset collection completed",
		zap.String("task_id", taskID.String()),
		zap.Int("success", successCount),
		zap.Int("failed", failCount))
}

// collectHost 采集单个主机
func (s *AssetCollectionService) collectHost(ctx context.Context, taskID uuid.UUID, hostID string, types []string) error {
	// 创建任务主机记录
	hostUUID, err := uuid.Parse(hostID)
	if err != nil {
		return fmt.Errorf("invalid host_id: %w", err)
	}

	taskHost := &model.AssetCollectionTaskHost{
		ID:     uuid.New(),
		TaskID: taskID,
		HostID: hostUUID,
		Status: assetCollectionStatusCollecting,
	}
	if err := s.repo.CreateTaskHost(taskHost); err != nil {
		return fmt.Errorf("failed to create task host: %w", err)
	}

	now := time.Now()
	taskHost.CollectStartedAt = &now

	if s.serverClient == nil {
		return s.failTaskHost(taskHost, "server client is not configured")
	}

	snapshot, err := s.collectProcessSnapshot(ctx, hostID)
	if err != nil {
		return s.failTaskHost(taskHost, err.Error())
	}
	snapshotJSON := mustMarshalJSON(snapshot)

	processSnapshot := &model.HostProcessSnapshot{
		ID:              uuid.New(),
		TaskID:          &taskID,
		HostID:          hostUUID,
		Hostname:        snapshot.Hostname,
		IPAddress:       snapshot.IPAddress,
		ProcessCount:    len(snapshot.Processes),
		ListenPortCount: countListenPorts(snapshot.Processes),
		SnapshotHash:    computeSnapshotHash(string(snapshotJSON)),
		SnapshotJSON:    snapshotJSON,
		CollectedAt:     time.Now(),
	}
	if err := s.repo.CreateProcessSnapshot(processSnapshot); err != nil {
		s.logger.Error("Failed to create process snapshot", zap.Error(err))
	}

	taskHost.Hostname = snapshot.Hostname
	taskHost.IPAddress = snapshot.IPAddress
	taskHost.ProcessCount = len(snapshot.Processes)
	taskHost.RawSnapshotID = &processSnapshot.ID
	if err := s.repo.UpdateTaskHost(taskHost); err != nil {
		s.logger.Warn("Failed to update task host process snapshot status",
			zap.String("host_id", hostID),
			zap.Error(err))
	}

	applicationCount := 0
	if hasCollectType(types, "application_analysis") {
		if s.analysisService == nil {
			return s.failTaskHost(taskHost, "application analysis service is not configured")
		}

		if err := s.updateTaskStage(taskID, assetCollectionStatusAnalyzing, "application_analysis"); err != nil {
			s.logger.Warn("Failed to update task to analyzing",
				zap.String("task_id", taskID.String()),
				zap.Error(err))
		}

		taskHost.Status = assetCollectionStatusAnalyzing
		if err := s.repo.UpdateTaskHost(taskHost); err != nil {
			s.logger.Warn("Failed to update task host to analyzing",
				zap.String("host_id", hostID),
				zap.Error(err))
		}

		count, err := s.analysisService.AnalyzeHostApplications(ctx, taskID, hostUUID, snapshot)
		if err != nil {
			return s.failTaskHost(taskHost, fmt.Sprintf("application analysis failed: %v", err))
		}
		applicationCount = count
	}

	taskHost.Hostname = snapshot.Hostname
	taskHost.IPAddress = snapshot.IPAddress
	taskHost.Status = assetCollectionStatusCompleted
	taskHost.SoftwareCount = 0
	taskHost.ProcessCount = len(snapshot.Processes)
	taskHost.ApplicationCount = applicationCount
	taskHost.RawSnapshotID = &processSnapshot.ID
	taskHost.CollectFinishedAt = ptrTime(time.Now())
	if err := s.repo.UpdateTaskHost(taskHost); err != nil {
		return fmt.Errorf("failed to update task host: %w", err)
	}

	s.logger.Info("Asset collection host completed",
		zap.String("host_id", hostID),
		zap.Int("process_count", len(snapshot.Processes)),
		zap.Int("application_count", applicationCount))

	return nil
}

func (s *AssetCollectionService) collectProcessSnapshot(ctx context.Context, hostID string) (HostAssetSnapshot, error) {
	offset := 0
	snapshot := HostAssetSnapshot{
		HostID:      hostID,
		Processes:   []ProcessAsset{},
		CollectedAt: time.Now(),
	}

	for {
		args := map[string]interface{}{
			"host_id":              hostID,
			"offset":               offset,
			"limit":                processSnapshotChunkLimit,
			"include_listen_ports": true,
			"max_process_count":    processSnapshotMaxCount,
		}
		argsJSON, err := json.Marshal(args)
		if err != nil {
			return snapshot, fmt.Errorf("failed to build process snapshot arguments: %w", err)
		}

		chunkCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		resp, err := s.serverClient.ExecuteTool(chunkCtx, uuid.New().String(), hostID, "AssetCollectProcessSnapshot", string(argsJSON), 45)
		cancel()
		if err != nil {
			return snapshot, fmt.Errorf("process snapshot chunk call failed: %w", err)
		}
		if resp == nil || !resp.Success {
			errMsg := "process snapshot chunk call failed"
			if resp != nil && resp.Error != "" {
				errMsg = resp.Error
			}
			return snapshot, fmt.Errorf("%s", errMsg)
		}

		var chunk ProcessSnapshotChunk
		if err := json.Unmarshal([]byte(resp.Result), &chunk); err != nil {
			return snapshot, fmt.Errorf("failed to parse process snapshot chunk: %w", err)
		}

		if offset == 0 {
			snapshot.HostID = chunk.HostID
			snapshot.Hostname = chunk.Hostname
			snapshot.IPAddress = chunk.IPAddress
			snapshot.OSType = chunk.OSType
			snapshot.OSVersion = chunk.OSVersion
			snapshot.Arch = chunk.Arch
			snapshot.CollectedAt = chunk.CollectedAt
		}
		snapshot.Processes = append(snapshot.Processes, chunk.Processes...)

		s.logger.Info("Collected process snapshot chunk",
			zap.String("host_id", hostID),
			zap.Int("offset", chunk.ProcessOffset),
			zap.Int("count", len(chunk.Processes)),
			zap.Bool("has_more", chunk.HasMore))

		if !chunk.HasMore || len(chunk.Processes) == 0 || len(snapshot.Processes) >= processSnapshotMaxCount {
			break
		}
		offset += len(chunk.Processes)
	}

	sort.Slice(snapshot.Processes, func(i, j int) bool {
		return snapshot.Processes[i].PID < snapshot.Processes[j].PID
	})

	return snapshot, nil
}

func (s *AssetCollectionService) updateTaskStage(taskID uuid.UUID, status, stage string) error {
	task, err := s.repo.GetTask(taskID)
	if err != nil {
		return err
	}
	task.Status = status
	task.CurrentStage = stage
	return s.repo.UpdateTask(task)
}

func (s *AssetCollectionService) failTaskHost(taskHost *model.AssetCollectionTaskHost, message string) error {
	return s.failTaskHostWithStatus(taskHost, assetCollectionStatusFailed, message)
}

func (s *AssetCollectionService) failTaskHostWithStatus(taskHost *model.AssetCollectionTaskHost, status string, message string) error {
	taskHost.Status = assetCollectionStatusFailed
	if status != "" {
		taskHost.Status = status
	}
	taskHost.ErrorMessage = message
	taskHost.CollectFinishedAt = ptrTime(time.Now())
	if err := s.repo.UpdateTaskHost(taskHost); err != nil {
		s.logger.Error("Failed to update failed task host status", zap.Error(err))
	}
	return fmt.Errorf("%s", message)
}

// convertPackageToAsset 转换包数据为资产模型
func (s *AssetCollectionService) convertPackageToAsset(hostID uuid.UUID, snapshot HostAssetSnapshot, pkg PackageAsset) *model.HostSoftwareAsset {
	return &model.HostSoftwareAsset{
		ID:              uuid.New(),
		HostID:          hostID,
		Hostname:        snapshot.Hostname,
		IPAddress:       snapshot.IPAddress,
		OSType:          snapshot.OSType,
		PackageManager:  pkg.PackageManager,
		Name:            pkg.Name,
		Version:         pkg.Version,
		Release:         pkg.Release,
		Epoch:           pkg.Epoch,
		Architecture:    pkg.Architecture,
		SourceName:      pkg.SourceName,
		Vendor:          pkg.Vendor,
		License:         pkg.License,
		InstallPaths:    mustMarshalJSON(pkg.InstallPaths),
		FileCount:       pkg.FileCount,
		PackageMetadata: mustMarshalJSON(pkg.Metadata),
		Fingerprint:     generateFingerprint(hostID.String(), pkg.PackageManager, pkg.Name, pkg.Version, pkg.Release, pkg.Architecture),
		Status:          "active",
		LastModifiedAt:  &pkg.InstallTime,
		LastSeenAt:      time.Now(),
		CollectedAt:     time.Now(),
	}
}

// RetryFailed 重试失败的主机
func (s *AssetCollectionService) RetryFailed(ctx context.Context, taskID uuid.UUID) error {
	task, err := s.repo.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("task not found: %w", err)
	}

	hosts, err := s.repo.GetTaskHosts(taskID)
	if err != nil {
		return fmt.Errorf("failed to get task hosts: %w", err)
	}

	var failedHostIDs []string
	for _, h := range hosts {
		if h.Status == assetCollectionStatusFailed || h.Status == "agent_offline" {
			failedHostIDs = append(failedHostIDs, h.HostID.String())
		}
	}

	if len(failedHostIDs) == 0 {
		return fmt.Errorf("no failed hosts to retry")
	}

	// 更新任务状态
	task.Status = assetCollectionStatusCollecting
	task.CurrentStage = "process_snapshot"
	if err := s.repo.UpdateTask(task); err != nil {
		return err
	}

	// 重新执行采集
	go s.executeCollection(ctx, taskID, failedHostIDs, []string{"process", "application_analysis"})

	return nil
}

// Cancel 取消任务
func (s *AssetCollectionService) Cancel(ctx context.Context, taskID uuid.UUID) error {
	task, err := s.repo.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("task not found: %w", err)
	}

	if task.Status != assetCollectionStatusCollecting && task.Status != assetCollectionStatusAnalyzing {
		return fmt.Errorf("task cannot be cancelled in status: %s", task.Status)
	}

	task.Status = assetCollectionStatusCancelled
	now := time.Now()
	task.FinishedAt = &now
	return s.repo.UpdateTask(task)
}

// GetConfig 获取采集配置
func (s *AssetCollectionService) GetConfig() (*model.AssetCollectionConfig, error) {
	return s.repo.GetConfig()
}

// GetRepo 获取仓库实例
func (s *AssetCollectionService) GetRepo() *repository.AssetCollectionRepository {
	return s.repo
}

// UpdateConfig 更新采集配置
func (s *AssetCollectionService) UpdateConfig(config *model.AssetCollectionConfig) error {
	// 校验间隔范围
	if config.IntervalHours < 1 || config.IntervalHours > 168 {
		return fmt.Errorf("interval_hours must be between 1 and 168")
	}

	// 计算下一次运行时间
	if config.Enabled {
		nextRun := time.Now().Add(time.Duration(config.IntervalHours) * time.Hour)
		config.NextRunAt = &nextRun
	} else {
		config.NextRunAt = nil
	}

	return s.repo.UpdateConfig(config)
}

// 辅助函数

func mustMarshalJSON(v interface{}) []byte {
	data, _ := json.Marshal(v)
	return data
}

func generateFingerprint(hostID, packageManager, name, version, release, architecture string) string {
	// 简化的指纹生成
	return fmt.Sprintf("%x", sha256Sum(fmt.Sprintf("%s:%s:%s:%s:%s:%s", hostID, packageManager, name, version, release, architecture)))
}

func sha256Sum(data string) [32]byte {
	return sha256.Sum256([]byte(data))
}

func countListenPorts(processes []ProcessAsset) int {
	count := 0
	for _, p := range processes {
		count += len(p.ListenPorts)
	}
	return count
}

func hasCollectType(types []string, target string) bool {
	for _, t := range types {
		if t == target || t == "full" {
			return true
		}
	}
	return false
}

func normalizeCollectTypes(types []string) []string {
	includeAnalysis := false
	for _, t := range types {
		if t == "application_analysis" || t == "full" {
			includeAnalysis = true
			break
		}
	}
	normalized := []string{"process"}
	if includeAnalysis {
		normalized = append(normalized, "application_analysis")
	}
	return normalized
}

func computeSnapshotHash(data string) string {
	return fmt.Sprintf("%x", sha256Sum(data))
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

// HostAssetSnapshot 主机资产快照 (API Server 端)
type HostAssetSnapshot struct {
	HostID      string         `json:"host_id"`
	Hostname    string         `json:"hostname"`
	IPAddress   string         `json:"ip_address"`
	OSType      string         `json:"os_type"`
	OSVersion   string         `json:"os_version"`
	Arch        string         `json:"arch"`
	Packages    []PackageAsset `json:"packages,omitempty"`
	Processes   []ProcessAsset `json:"processes"`
	CollectedAt time.Time      `json:"collected_at"`
}

// ProcessSnapshotChunk 进程快照分片
type ProcessSnapshotChunk struct {
	HostID        string         `json:"host_id"`
	Hostname      string         `json:"hostname"`
	IPAddress     string         `json:"ip_address"`
	OSType        string         `json:"os_type"`
	OSVersion     string         `json:"os_version"`
	Arch          string         `json:"arch"`
	ProcessOffset int            `json:"process_offset"`
	ProcessLimit  int            `json:"process_limit"`
	ProcessTotal  int            `json:"process_total"`
	HasMore       bool           `json:"has_more"`
	Processes     []ProcessAsset `json:"processes"`
	CollectedAt   time.Time      `json:"collected_at"`
}

// PackageAsset 软件包资产
type PackageAsset struct {
	Name           string            `json:"name"`
	Version        string            `json:"version"`
	Release        string            `json:"release,omitempty"`
	Epoch          string            `json:"epoch,omitempty"`
	Architecture   string            `json:"architecture"`
	PackageManager string            `json:"package_manager"`
	SourceName     string            `json:"source_name,omitempty"`
	Vendor         string            `json:"vendor,omitempty"`
	License        string            `json:"license,omitempty"`
	InstallTime    time.Time         `json:"install_time,omitempty"`
	InstallPaths   []string          `json:"install_paths"`
	FileCount      int               `json:"file_count"`
	Metadata       map[string]string `json:"metadata"`
}

// ProcessAsset 进程资产
type ProcessAsset struct {
	PID         int       `json:"pid"`
	PPID        int       `json:"ppid"`
	Comm        string    `json:"comm"`
	Cmdline     string    `json:"cmdline"`
	ExePath     string    `json:"exe_path"`
	Cwd         string    `json:"cwd"`
	UID         int       `json:"uid"`
	Username    string    `json:"username"`
	ListenPorts []int     `json:"listen_ports"`
	StartTime   time.Time `json:"start_time,omitempty"`
	ContainerID string    `json:"container_id,omitempty"`
}
