package handler

import (
	"net/http"

	"aegis-system/internal/service"
	"aegis-system/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // TODO: restrict in production
	},
}

// WebSocketHandler handles WebSocket connections
type WebSocketHandler struct {
	wsService *service.WebSocketService
	logger    *zap.Logger
}

// NewWebSocketHandler creates a new WebSocket handler
func NewWebSocketHandler(wsService *service.WebSocketService) *WebSocketHandler {
	return &WebSocketHandler{
		wsService: wsService,
		logger:    logger.Get(),
	}
}

// HandleConnection upgrades HTTP to WebSocket and manages the connection
func (h *WebSocketHandler) HandleConnection(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("failed to upgrade to websocket", zap.Error(err))
		return
	}
	defer conn.Close()

	h.wsService.AddClient(conn)
	defer h.wsService.RemoveClient(conn)

	// Keep connection alive, read messages (heartbeats)
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				h.logger.Error("websocket read error", zap.Error(err))
			}
			break
		}
	}
}
