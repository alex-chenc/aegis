package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"ai-benchmark/backend/internal/models"
	pb "ai-benchmark/backend/proto/agent_comm"
)

type HostHandler struct {
	DB           *gorm.DB
	AgentService CommandSender
}

type CommandSender interface {
	SendCommand(hostID string, cmd *pb.ServerCommand) error
	IsAgentConnected(hostID string) bool
}

func NewHostHandler(db *gorm.DB, agentService CommandSender) *HostHandler {
	return &HostHandler{DB: db, AgentService: agentService}
}

func (h *HostHandler) GetHosts(c *gin.Context) {
	if h.DB == nil {
		c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{"items": []models.Host{}, "total": 0}})
		return
	}

	var hosts []models.Host
	if err := h.DB.Find(&hosts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{"items": hosts, "total": len(hosts)}})
}

func (h *HostHandler) GetHostDetails(c *gin.Context) {
	if h.DB == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "Host not found"})
		return
	}

	id := c.Param("id")
	var host models.Host
	if err := h.DB.First(&host, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "Host not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": host})
}

type CommandRequest struct {
	Command string `json:"command" binding:"required"`
	Timeout int32  `json:"timeout"`
}

func (h *HostHandler) SendCommand(c *gin.Context) {
	hostID := c.Param("id")

	if h.AgentService == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "Agent service not available"})
		return
	}

	if !h.AgentService.IsAgentConnected(hostID) {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "Agent not connected"})
		return
	}

	var req CommandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "Invalid request: " + err.Error()})
		return
	}

	commandID := uuid.New().String()
	cmd := &pb.ServerCommand{
		CommandId:      commandID,
		Type:           pb.CommandType_EXEC_SCRIPT,
		ScriptContent:  req.Command,
		TimeoutSeconds: int64(req.Timeout),
	}

	if err := h.AgentService.SendCommand(hostID, cmd); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "Failed to send command: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"command_id": commandID,
			"status":     "sent",
		},
	})
}
