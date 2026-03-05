package api

import (
	"net"
	"net/http"
	"os"
	"sync"

	"github.com/gin-gonic/gin"

	"ai-benchmark/backend/pkg/llm"
)

type SettingsHandler struct {
	llmClient *llm.Client
	config    *SystemConfig
	mu        sync.RWMutex
}

type SystemConfig struct {
	LLMAPIKey  string `json:"llm_api_key"`
	LLMBaseURL string `json:"llm_base_url"`
	LLMModel   string `json:"llm_model"`
}

func NewSettingsHandler(llmClient *llm.Client) *SettingsHandler {
	return &SettingsHandler{
		llmClient: llmClient,
		config: &SystemConfig{
			LLMAPIKey:  os.Getenv("LLM_API_KEY"),
			LLMBaseURL: os.Getenv("LLM_BASE_URL"),
			LLMModel:   "qwen-plus",
		},
	}
}

func (h *SettingsHandler) GetSettings(c *gin.Context) {
	h.mu.RLock()
	config := h.config
	h.mu.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"llm_configured": config.LLMAPIKey != "",
			"llm_base_url":   config.LLMBaseURL,
			"llm_model":      config.LLMModel,
		},
	})
}

type UpdateLLMConfigRequest struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

func (h *SettingsHandler) UpdateLLMConfig(c *gin.Context) {
	var req UpdateLLMConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	h.mu.Lock()
	if req.APIKey != "" {
		h.config.LLMAPIKey = req.APIKey
	}
	if req.BaseURL != "" {
		h.config.LLMBaseURL = req.BaseURL
	}
	if req.Model != "" {
		h.config.LLMModel = req.Model
	}
	h.mu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "LLM configuration updated",
	})
}

func (h *SettingsHandler) TestLLMConnection(c *gin.Context) {
	h.mu.RLock()
	apiKey := h.config.LLMAPIKey
	baseURL := h.config.LLMBaseURL
	h.mu.RUnlock()

	if apiKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "LLM API key not configured",
		})
		return
	}

	testClient := llm.NewClient(apiKey, baseURL)

	if err := testClient.TestConnection(); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    500,
			"message": "LLM connection test failed: " + err.Error(),
			"data": gin.H{
				"connected": false,
				"error":     err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "LLM connection test successful",
		"data": gin.H{
			"connected": true,
		},
	})
}

type GetInstallCommandRequest struct {
	ServerIP string `json:"server_ip"`
}

func (h *SettingsHandler) GetInstallCommand(c *gin.Context) {
	serverIP := c.Query("server_ip")
	if serverIP == "" {
		serverIP = os.Getenv("SERVER_PUBLIC_IP")
	}
	if serverIP == "" {
		serverIP = "localhost"
	}

	grpcPort := os.Getenv("BACKEND_GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "9090"
	}

	authToken := os.Getenv("AGENT_AUTH_TOKEN")
	if authToken == "" {
		authToken = "your-auth-token"
	}

	installCommand := `curl -sSL http://` + serverIP + `/agent/install.sh | bash -s -- --server=` + serverIP + `:` + grpcPort + ` --token=` + authToken

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"install_command": installCommand,
			"server_address":  serverIP + ":" + grpcPort,
		},
	})
}

func (h *SettingsHandler) GetServerInfo(c *gin.Context) {
	serverIP := os.Getenv("SERVER_PUBLIC_IP")
	if serverIP == "" {
		serverIP = getLocalIP()
	}

	grpcPort := os.Getenv("BACKEND_GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "9090"
	}

	httpPort := os.Getenv("BACKEND_HTTP_PORT")
	if httpPort == "" {
		httpPort = "8080"
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"server_ip":      serverIP,
			"grpc_port":      grpcPort,
			"http_port":      httpPort,
			"server_address": serverIP + ":" + grpcPort,
		},
	})
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}
