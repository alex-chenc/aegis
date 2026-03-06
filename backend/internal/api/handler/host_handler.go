package handler

import (
	"net/http"

	"baseline-system/internal/repository"
	"baseline-system/internal/storage"
	"baseline-system/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// HostHandler 主机 Handler
type HostHandler struct {
	hostRepo    *repository.HostRepository
	redisClient *storage.RedisClient
}

// NewHostHandler 创建主机 Handler
func NewHostHandler(hostRepo *repository.HostRepository, redisClient *storage.RedisClient) *HostHandler {
	return &HostHandler{
		hostRepo:    hostRepo,
		redisClient: redisClient,
	}
}

// HostResponse 主机响应
type HostResponse struct {
	ID              string `json:"id"`
	IPAddress       string `json:"ip_address"`
	Hostname        string `json:"hostname"`
	OSType          string `json:"os_type"`
	AgentVersion    string `json:"agent_version"`
	LastHeartbeatAt string `json:"last_heartbeat_at"`
	Online          bool   `json:"online"`
}

// ListHosts 获取主机列表
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

	// 批量检查在线状态
	hostIDs := make([]string, len(hosts))
	for i, host := range hosts {
		hostIDs[i] = host.ID.String()
	}

	onlineMap, _ := h.redisClient.BatchCheckOnline(hostIDs)

	// 构建响应
	result := make([]HostResponse, len(hosts))
	for i, host := range hosts {
		result[i] = HostResponse{
			ID:              host.ID.String(),
			IPAddress:       host.IPAddress,
			Hostname:        host.Hostname,
			OSType:          host.OSType,
			AgentVersion:    host.AgentVersion,
			LastHeartbeatAt: host.LastHeartbeatAt.Format("2006-01-02 15:04:05"),
			Online:          onlineMap[host.ID.String()],
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    result,
	})
}

// GetHost 获取主机详情
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

	online, _ := h.redisClient.IsOnline(id)

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
