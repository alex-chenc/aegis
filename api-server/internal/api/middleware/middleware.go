package middleware

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"api-server/internal/service"
	"api-server/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// CORS 跨域中间件
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func AuthRequired(authService *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isAuthPublicPath(c.Request.URL.Path) || c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		token := extractBearerToken(c.GetHeader("Authorization"))
		authCtx, err := authService.ValidateToken(token)
		if err != nil {
			status := http.StatusInternalServerError
			message := "认证服务异常"
			if errors.Is(err, service.ErrInvalidToken) {
				status = http.StatusUnauthorized
				message = "未认证"
			}
			c.AbortWithStatusJSON(status, gin.H{"message": message})
			return
		}
		if !authCtx.CanAccessBusinessAPI() && !isForcedPasswordAllowedPath(c.Request.URL.Path) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "必须先修改账号密码"})
			return
		}

		c.Set("auth_user_id", authCtx.UserID)
		c.Set("auth_username", authCtx.Username)
		c.Next()
	}
}

func isAuthPublicPath(path string) bool {
	if path == "/health" || strings.HasPrefix(path, "/api/v1/auth/") {
		return true
	}

	switch path {
	case "/api/v1/agent/install-command",
		"/api/v1/agent/install.sh",
		"/api/v1/agent/uninstall.sh",
		"/api/v1/agent/download":
		return true
	default:
		return false
	}
}

func isForcedPasswordAllowedPath(path string) bool {
	switch path {
	case "/api/v1/auth/me", "/api/v1/auth/change-credentials", "/api/v1/auth/logout":
		return true
	default:
		return false
	}
}

func extractBearerToken(header string) string {
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
}

// RequestLogger 请求日志中间件
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()

		logger.Info("http request",
			zap.Int("status", statusCode),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.String("client_ip", c.ClientIP()),
			zap.Duration("latency", latency),
		)
	}
}

// Recovery Panic 恢复中间件
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("panic recovered",
					zap.Any("error", err),
					zap.String("path", c.Request.URL.Path),
					zap.String("method", c.Request.Method),
				)

				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"code":    500,
					"message": "internal server error",
					"data":    nil,
				})
			}
		}()

		c.Next()
	}
}
