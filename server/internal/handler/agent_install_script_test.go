package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAgentInstallScriptEnablesGuardMonitoringByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/agent/install.sh", nil)

	handler := NewAgentHandler(nil, nil, "127.0.0.1", 8082, 19090)
	handler.GetInstallScript(context)

	body := recorder.Body.String()
	for _, setting := range []string{
		"AgentGuardEnabled = true",
		"AgentGuardBehaviorMonitorEnabled = true",
	} {
		if !strings.Contains(body, setting) {
			t.Fatalf("install script missing safe monitoring default %q", setting)
		}
	}
	if strings.Contains(body, "AgentGuardEnabled = false") ||
		strings.Contains(body, "AgentGuardBehaviorMonitorEnabled = false") {
		t.Fatal("install script still disables Agent Guard monitoring")
	}
}
