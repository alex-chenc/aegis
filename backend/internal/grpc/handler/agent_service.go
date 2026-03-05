package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"ai-benchmark/backend/internal/models"
	"ai-benchmark/backend/pkg/config"
	pb "ai-benchmark/backend/proto/agent_comm"
)

type StatusChangeListener interface {
	OnHostStatusChange(hostID string, isOnline bool)
	OnTaskComplete(hostID, taskID, status string, stdout, stderr string)
}

type AgentConnection struct {
	HostID string
	Stream pb.AgentService_RegisterServer
}

type AgentServiceHandler struct {
	pb.UnimplementedAgentServiceServer
	mu          sync.RWMutex
	connections map[string]*AgentConnection
	db          *gorm.DB
	redis       *config.RedisClient
	listener    StatusChangeListener
}

func NewAgentServiceHandler(db *gorm.DB, redis *config.RedisClient) *AgentServiceHandler {
	return &AgentServiceHandler{
		connections: make(map[string]*AgentConnection),
		db:          db,
		redis:       redis,
	}
}

func (h *AgentServiceHandler) SetListener(listener StatusChangeListener) {
	h.listener = listener
}

func (h *AgentServiceHandler) Register(stream pb.AgentService_RegisterServer) error {
	ctx := stream.Context()
	var hostID string

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			log.Printf("[Agent] Disconnected: %s", hostID)
			h.removeConnection(hostID)
			h.markHostOffline(hostID)
			return nil
		}
		if err != nil {
			log.Printf("[Agent] Error receiving from %s: %v", hostID, err)
			h.removeConnection(hostID)
			h.markHostOffline(hostID)
			return err
		}

		if hostID == "" {
			hostID = req.HostId
			log.Printf("[Agent] Registered: %s", hostID)
			h.addConnection(hostID, stream)
			h.markHostOnline(hostID)
		}

		h.handleMessage(ctx, req, stream)
	}
}

func (h *AgentServiceHandler) handleMessage(ctx context.Context, msg *pb.AgentMessage, stream pb.AgentService_RegisterServer) {
	switch payload := msg.Payload.(type) {
	case *pb.AgentMessage_HeartbeatRequest:
		h.handleHeartbeat(msg.HostId, payload.HeartbeatRequest)
	case *pb.AgentMessage_AssetInfo:
		h.handleAssetInfo(msg.HostId, payload.AssetInfo)
	case *pb.AgentMessage_CommandResult:
		h.handleCommandResult(msg.HostId, payload.CommandResult)
	default:
		log.Printf("[Agent] Unknown message type from %s", msg.HostId)
	}
}

func (h *AgentServiceHandler) handleHeartbeat(hostID string, heartbeat *pb.HeartbeatRequest) {
	log.Printf("[Agent] Heartbeat from %s: cpu=%.2f, mem=%.2f, version=%s",
		hostID, heartbeat.CpuLoad_1Min, heartbeat.MemUsagePercent, heartbeat.AgentVersion)

	if h.redis != nil {
		if err := h.redis.SetHostOnline(hostID); err != nil {
			log.Printf("[Agent] Failed to update Redis status for %s: %v", hostID, err)
		}
		if err := h.redis.UpdateHostMetrics(hostID, heartbeat.CpuLoad_1Min, heartbeat.MemUsagePercent); err != nil {
			log.Printf("[Agent] Failed to update Redis metrics for %s: %v", hostID, err)
		}
	}

	if h.db != nil {
		now := time.Now()
		result := h.db.Model(&models.Host{}).Where("id = ?", hostID).Updates(map[string]interface{}{
			"cpu_load_1min":     heartbeat.CpuLoad_1Min,
			"mem_usage_percent": heartbeat.MemUsagePercent,
			"agent_version":     heartbeat.AgentVersion,
			"last_heartbeat_at": now,
			"is_online":         true,
		})
		if result.Error != nil {
			log.Printf("[Agent] Failed to update host %s in DB: %v", hostID, result.Error)
		}
	}
}

func (h *AgentServiceHandler) handleAssetInfo(hostID string, assetInfo *pb.AssetInfo) {
	log.Printf("[Agent] AssetInfo from %s: hostname=%s, os=%s, ip=%s",
		hostID, assetInfo.Hostname, assetInfo.OsName, assetInfo.IpAddress)

	if h.db == nil {
		return
	}

	var cpuInfoJSON datatypes.JSON
	if assetInfo.CpuInfo != nil {
		cpuData := map[string]interface{}{
			"model_name": assetInfo.CpuInfo.ModelName,
			"cores":      assetInfo.CpuInfo.Cores,
			"threads":    assetInfo.CpuInfo.Threads,
			"frequency":  assetInfo.CpuInfo.FrequencyMhz,
		}
		if data, err := json.Marshal(cpuData); err == nil {
			cpuInfoJSON = data
		}
	}

	var networkJSON datatypes.JSON
	if len(assetInfo.NetworkInterfaces) > 0 {
		interfaces := make([]map[string]interface{}, len(assetInfo.NetworkInterfaces))
		for i, ni := range assetInfo.NetworkInterfaces {
			interfaces[i] = map[string]interface{}{
				"name":         ni.Name,
				"mac_address":  ni.MacAddress,
				"ip_addresses": ni.IpAddresses,
				"is_up":        ni.IsUp,
			}
		}
		if data, err := json.Marshal(map[string]interface{}{"interfaces": interfaces}); err == nil {
			networkJSON = data
		}
	}

	var totalMemoryMB int64
	if assetInfo.MemoryInfo != nil {
		totalMemoryMB = assetInfo.MemoryInfo.TotalBytes / (1024 * 1024)
	}

	now := time.Now()
	host := models.Host{
		ID:                parseUUID(hostID),
		IPAddress:         assetInfo.IpAddress,
		Hostname:          assetInfo.Hostname,
		OsType:            assetInfo.OsName,
		OsVersion:         assetInfo.OsVersion,
		KernelVersion:     assetInfo.KernelVersion,
		Architecture:      assetInfo.Arch,
		CpuInfo:           cpuInfoJSON,
		TotalMemoryMB:     totalMemoryMB,
		NetworkInterfaces: networkJSON,
		LastHeartbeatAt:   now,
		IsOnline:          true,
	}

	result := h.db.Where("id = ?", hostID).Assign(host).FirstOrCreate(&host)
	if result.Error != nil {
		log.Printf("[Agent] Failed to save host %s: %v", hostID, result.Error)
	}
}

func (h *AgentServiceHandler) handleCommandResult(hostID string, result *pb.CommandResult) {
	log.Printf("[Agent] CommandResult from %s: command_id=%s, status=%v, exit_code=%d",
		hostID, result.CommandId, result.Status, result.ExitCode)

	if h.db == nil {
		return
	}

	status := "FAILED"
	if result.Status == pb.CommandStatus_SUCCESS {
		status = "SUCCESS"
	} else if result.Status == pb.CommandStatus_TIMEOUT {
		status = "TIMEOUT"
	}

	var stdout, stderr string
	for _, entry := range result.LogEntries {
		if entry.Stream == pb.LogStream_STDOUT {
			stdout += entry.Line + "\n"
		} else {
			stderr += entry.Line + "\n"
		}
	}

	now := time.Now()
	h.db.Model(&models.TaskLog{}).Where("id = ?", result.CommandId).Updates(map[string]interface{}{
		"status":      status,
		"stdout":      stdout,
		"stderr":      stderr,
		"exit_code":   result.ExitCode,
		"finished_at": now,
	})

	if h.listener != nil {
		h.listener.OnTaskComplete(hostID, result.CommandId, status, stdout, stderr)
	}
}

func (h *AgentServiceHandler) SendCommand(hostID string, cmd *pb.ServerCommand) error {
	h.mu.RLock()
	conn, ok := h.connections[hostID]
	h.mu.RUnlock()

	if !ok {
		return errors.New("agent not connected: " + hostID)
	}

	msg := &pb.ServerMessage{
		MessageId: cmd.CommandId,
		Timestamp: timestamppb.Now(),
		Payload: &pb.ServerMessage_Command{
			Command: cmd,
		},
	}

	return conn.Stream.Send(msg)
}

func (h *AgentServiceHandler) GetConnectedAgents() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	agents := make([]string, 0, len(h.connections))
	for id := range h.connections {
		agents = append(agents, id)
	}
	return agents
}

func (h *AgentServiceHandler) IsAgentConnected(hostID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.connections[hostID]
	return ok
}

func (h *AgentServiceHandler) addConnection(hostID string, stream pb.AgentService_RegisterServer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.connections[hostID] = &AgentConnection{
		HostID: hostID,
		Stream: stream,
	}
}

func (h *AgentServiceHandler) removeConnection(hostID string) {
	if hostID == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.connections, hostID)
}

func (h *AgentServiceHandler) markHostOnline(hostID string) {
	if hostID == "" {
		return
	}

	if h.redis != nil {
		h.redis.SetHostOnline(hostID)
	}

	if h.db != nil {
		h.db.Model(&models.Host{}).Where("id = ?", hostID).Updates(map[string]interface{}{
			"is_online":         true,
			"last_heartbeat_at": time.Now(),
		})
	}

	if h.listener != nil {
		h.listener.OnHostStatusChange(hostID, true)
	}

	log.Printf("[Agent] Host %s marked as ONLINE", hostID)
}

func (h *AgentServiceHandler) markHostOffline(hostID string) {
	if hostID == "" {
		return
	}

	if h.redis != nil {
		h.redis.SetHostOffline(hostID)
	}

	if h.db != nil {
		h.db.Model(&models.Host{}).Where("id = ?", hostID).Updates(map[string]interface{}{
			"is_online": false,
		})
	}

	if h.listener != nil {
		h.listener.OnHostStatusChange(hostID, false)
	}

	log.Printf("[Agent] Host %s marked as OFFLINE", hostID)
}

func parseUUID(s string) uuid.UUID {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.New()
	}
	return id
}
