package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"aegis-system/internal/service"
	"aegis-system/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func TestNewWebSocketHandler(t *testing.T) {
	if logger.Logger == nil {
		err := logger.Init(&logger.Config{
			Level:      "error",
			MaxSize:    10,
			MaxBackups: 1,
			MaxAge:     1,
			Compress:   false,
		})
		if err != nil {
			t.Fatalf("failed to init logger: %v", err)
		}
	}

	wsService := service.NewWebSocketService()
	handler := NewWebSocketHandler(wsService)
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
	if handler.wsService != wsService {
		t.Error("expected handler to use provided service")
	}
	if handler.logger == nil {
		t.Error("expected handler to have logger")
	}
}

func TestWebSocketHandler_HandleConnection(t *testing.T) {
	gin.SetMode(gin.TestMode)

	if logger.Logger == nil {
		err := logger.Init(&logger.Config{
			Level:      "error",
			MaxSize:    10,
			MaxBackups: 1,
			MaxAge:     1,
			Compress:   false,
		})
		if err != nil {
			t.Fatalf("failed to init logger: %v", err)
		}
	}

	wsService := service.NewWebSocketService()
	handler := NewWebSocketHandler(wsService)

	// Create a test server that upgrades the connection
	router := gin.Default()
	router.GET("/ws", handler.HandleConnection)

	// Create test server
	server := httptest.NewServer(router)
	defer server.Close()

	// Convert http:// to ws://
	wsURL := "ws" + server.URL[4:] + "/ws"

	// Connect with WebSocket client
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer wsConn.Close()

	// Verify client was registered
	if wsService.GetClientCount() != 1 {
		t.Errorf("expected 1 client, got %d", wsService.GetClientCount())
	}

	// Send a message to verify connection is working
	testMessage := map[string]string{"type": "ping"}
	if err := wsConn.WriteJSON(testMessage); err != nil {
		t.Fatalf("failed to write message: %v", err)
	}

	// Clean up
	wsConn.Close()
}

func TestWebSocketHandler_UpgraderConfiguration(t *testing.T) {
	// Verify upgrader is configured with correct buffer sizes
	if upgrader.ReadBufferSize != 1024 {
		t.Errorf("expected ReadBufferSize 1024, got %d", upgrader.ReadBufferSize)
	}
	if upgrader.WriteBufferSize != 1024 {
		t.Errorf("expected WriteBufferSize 1024, got %d", upgrader.WriteBufferSize)
	}

	// Verify CheckOrigin allows all origins (TODO: should be restricted in production)
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	if !upgrader.CheckOrigin(req) {
		t.Error("expected CheckOrigin to return true for any origin")
	}
}
