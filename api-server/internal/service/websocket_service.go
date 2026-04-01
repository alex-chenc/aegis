package service

import (
	"sync"

	"api-server/internal/model"
	"api-server/pkg/logger"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// WSMessage represents a WebSocket message
type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// WebSocketService manages WebSocket connections and broadcasts
type WebSocketService struct {
	clients map[*websocket.Conn]bool
	mu      sync.RWMutex
	logger  *zap.Logger
}

// NewWebSocketService creates a new WebSocket service
func NewWebSocketService() *WebSocketService {
	return &WebSocketService{
		clients: make(map[*websocket.Conn]bool),
		logger:  logger.Get(),
	}
}

// AddClient registers a new WebSocket connection
func (s *WebSocketService) AddClient(conn *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[conn] = true
	s.logger.Info("WebSocket client connected", zap.Int("total_clients", len(s.clients)))
}

// RemoveClient unregisters a WebSocket connection
func (s *WebSocketService) RemoveClient(conn *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, conn)
	s.logger.Info("WebSocket client disconnected", zap.Int("total_clients", len(s.clients)))
}

// Broadcast sends a message to all connected clients
func (s *WebSocketService) Broadcast(msg WSMessage) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for conn := range s.clients {
		if err := conn.WriteJSON(msg); err != nil {
			s.logger.Error("failed to broadcast message", zap.Error(err))
			conn.Close()
			delete(s.clients, conn)
		}
	}
}

// BroadcastAlert broadcasts an alert to all clients
func (s *WebSocketService) BroadcastAlert(alert *model.Alert) {
	s.Broadcast(WSMessage{
		Type: "alert",
		Data: alert,
	})
}

// BroadcastBlockStatus broadcasts a block record to all clients
func (s *WebSocketService) BroadcastBlockStatus(record *model.BlockRecord) {
	s.Broadcast(WSMessage{
		Type: "block_status",
		Data: record,
	})
}

// BroadcastRuleUpdate broadcasts a rule update to all clients
func (s *WebSocketService) BroadcastRuleUpdate(rule *model.SigmaRule) {
	s.Broadcast(WSMessage{
		Type: "rule_update",
		Data: rule,
	})
}

// BroadcastPolicyUpdate broadcasts a block policy update to all clients
func (s *WebSocketService) BroadcastPolicyUpdate(policy *model.BlockPolicy) {
	s.Broadcast(WSMessage{
		Type: "policy_update",
		Data: policy,
	})
}

// GetClientCount returns the number of connected clients
func (s *WebSocketService) GetClientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}
