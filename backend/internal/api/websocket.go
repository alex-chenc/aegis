package api

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type WebSocketMessage struct {
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

type WebSocketHandler struct {
	clients   map[*websocket.Conn]bool
	broadcast chan *WebSocketMessage
	mu        sync.RWMutex
}

func NewWebSocketHandler() *WebSocketHandler {
	h := &WebSocketHandler{
		clients:   make(map[*websocket.Conn]bool),
		broadcast: make(chan *WebSocketMessage, 100),
	}

	go h.runBroadcast()

	return h
}

func (h *WebSocketHandler) HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WebSocket] Failed to upgrade connection: %v", err)
		return
	}

	h.mu.Lock()
	h.clients[conn] = true
	clientCount := len(h.clients)
	h.mu.Unlock()

	log.Printf("[WebSocket] Client connected. Total clients: %d", clientCount)

	defer func() {
		h.mu.Lock()
		delete(h.clients, conn)
		h.mu.Unlock()
		conn.Close()
		log.Printf("[WebSocket] Client disconnected. Total clients: %d", len(h.clients))
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (h *WebSocketHandler) runBroadcast() {
	for msg := range h.broadcast {
		h.mu.RLock()
		clients := make([]*websocket.Conn, 0, len(h.clients))
		for client := range h.clients {
			clients = append(clients, client)
		}
		h.mu.RUnlock()

		for _, client := range clients {
			err := client.WriteJSON(msg)
			if err != nil {
				log.Printf("[WebSocket] Error writing to client: %v", err)
				client.Close()
				h.mu.Lock()
				delete(h.clients, client)
				h.mu.Unlock()
			}
		}
	}
}

func (h *WebSocketHandler) BroadcastHostUpdate(hostID, status string, data interface{}) {
	msg := &WebSocketMessage{
		Type:      "host_update",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"host_id": hostID,
			"status":  status,
			"data":    data,
		},
	}
	h.broadcast <- msg
}

func (h *WebSocketHandler) BroadcastTaskUpdate(taskID, status string, stdout, stderr string) {
	msg := &WebSocketMessage{
		Type:      "task_update",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"task_id": taskID,
			"status":  status,
			"stdout":  stdout,
			"stderr":  stderr,
		},
	}
	h.broadcast <- msg
}

func (h *WebSocketHandler) BroadcastLogEntry(taskID, stream, line string) {
	msg := &WebSocketMessage{
		Type:      "log_entry",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"task_id": taskID,
			"stream":  stream,
			"line":    line,
		},
	}
	h.broadcast <- msg
}

func (h *WebSocketHandler) BroadcastAgentStatus(hostID string, online bool) {
	msg := &WebSocketMessage{
		Type:      "agent_status",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"host_id": hostID,
			"online":  online,
		},
	}
	h.broadcast <- msg
}

func (h *WebSocketHandler) HandleHostStatusWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, conn)
		h.mu.Unlock()
		conn.Close()
	}()

	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			return
		}

		if messageType == websocket.TextMessage {
			var msg map[string]interface{}
			if err := json.Unmarshal(p, &msg); err == nil {
				if msg["type"] == "ping" {
					conn.WriteJSON(map[string]string{"type": "pong"})
				}
			}
		}
	}
}
