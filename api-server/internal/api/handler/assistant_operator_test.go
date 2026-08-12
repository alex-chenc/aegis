package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAssistantOperatorPrefersAuthenticatedUsername(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("auth_username", "security-admin")
	ctx.Set("username", "stale-compatibility-value")

	if got := assistantOperator(ctx); got != "security-admin" {
		t.Fatalf("assistantOperator() = %q, want authenticated username", got)
	}
}

func TestAssistantOperatorFallsBackForCompatibilityCallers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("username", "legacy-admin")

	if got := assistantOperator(ctx); got != "legacy-admin" {
		t.Fatalf("assistantOperator() = %q, want compatibility username", got)
	}
}
