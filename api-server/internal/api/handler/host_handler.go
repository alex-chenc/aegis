package handler

import (
	"context"
	"net/http"
	"time"

	grpcclient "api-server/internal/grpc"
	"api-server/internal/repository"
	"api-server/internal/storage"
	"api-server/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type HostHandler struct {
	hostRepo     *repository.HostRepository
	redisClient *storage.RedisClient
	serverClient *grpcclient.ServerClient
}

func NewHostHandler(hostRepo *repository.HostRepository, redisClient *storage.RedisClient, serverClient *grpcclient.ServerClient) *HostHandler {
	return &HostHandler{
		hostRepo:     hostRepo,
		redisClient:  redisClient,
		serverClient: serverClient,
	}
}

type HostResponse struct {
	ID               string `json:"id"`
	IPAddress        string `json:"ip_address"`
	Hostname         string `json:"hostname"`
	OSType           string `json:"os_type"`
	AgentVersion     string `json:"agent_version"`
	LastHeartbeatAt  string `json:"last_heartbeat_at"`
	Online           bool   `json:"online"`
}

func (h *HostHandler) ListHosts(c *gin.Context) {
	page := 1
	pageSize := 10
	query := c.DefaultQuery("query", "")

	hosts, err := h.hostRepo.FindAll(page, pageSize, query)
	if err != nil {
		logger.Error("failed to list hosts", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "failed to list hosts",
		})
		return
	}

	// Get online status from server
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	var agentStatuses map[string]bool
	if h.serverClient != nil {
		resp, err := h.serverClient.ListConnectedAgents(ctx)
		if err == nil {
			agentStatuses = make(map[string]bool)
			for _, agent := range resp.Agents {
				agentStatuses[agent.HostId] = agent.Connected
			}
		}
	}

	result := make([]HostResponse, len(hosts))
	for i, host := range hosts {
		online := false
		if agentStatuses != nil {
			online = agentStatuses[host.ID.String()]
		}

		result[i] = HostResponse{
			ID:              host.ID.String(),
			IPAddress:       host.IPAddress,
			Hostname:        host.Hostname,
			OSType:          host.OSType,
			AgentVersion:    host.AgentVersion,
			LastHeartbeatAt: host.LastHeartbeatAt.Format("2006-01-02 15:04:05"),
			Online:          online,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    result,
	})
}

func (h *HostHandler) GetHost(c *gin.Context) {
	id := c.Param("id")
	hostID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid host id",
		})
		return
	}

	host, err := h.hostRepo.FindByID(hostID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "host not found",
		})
		return
	}

	// Check online status from server
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	online := false
	if h.serverClient != nil {
		resp, err := h.serverClient.GetAgentStatus(ctx, host.ID.String())
		if err == nil {
			online = resp.Connected
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": HostResponse{
			ID:              host.ID.String(),
			IPAddress:       host.IPAddress,
			Hostname:        host.Hostname,
			OSType:          host.OSType,
			AgentVersion:    host.AgentVersion,
			LastHeartbeatAt: host.LastHeartbeatAt.Format("2006-01-02 15:04:05"),
			Online:          online,
		},
	})
}
