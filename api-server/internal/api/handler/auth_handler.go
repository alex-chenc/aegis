package handler

import (
	"errors"
	"net/http"
	"strings"

	"api-server/internal/service"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authService *service.AuthService
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type ChangeCredentialsRequest struct {
	Username        string `json:"username"`
	NewPassword     string `json:"new_password"`
	ConfirmPassword string `json:"confirm_password"`
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

func (h *AuthHandler) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/status", h.GetStatus)
	group.POST("/bootstrap-login", h.BootstrapLogin)
	group.POST("/login", h.Login)
	group.GET("/me", h.Me)
	group.POST("/change-credentials", h.ChangeCredentials)
	group.POST("/logout", h.Logout)
}

func (h *AuthHandler) GetStatus(c *gin.Context) {
	status, err := h.authService.GetStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to get auth status"})
		return
	}
	c.JSON(http.StatusOK, status)
}

func (h *AuthHandler) BootstrapLogin(c *gin.Context) {
	session, err := h.authService.BootstrapLogin()
	if err != nil {
		writeAuthError(c, err)
		return
	}
	c.JSON(http.StatusOK, session)
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request"})
		return
	}

	session, err := h.authService.Login(req.Username, req.Password)
	if err != nil {
		writeAuthError(c, err)
		return
	}
	c.JSON(http.StatusOK, session)
}

func (h *AuthHandler) Me(c *gin.Context) {
	authCtx, err := h.authService.ValidateToken(bearerToken(c))
	if err != nil {
		writeAuthError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"username":              authCtx.Username,
		"force_password_change": authCtx.ForcePasswordChange,
	})
}

func (h *AuthHandler) ChangeCredentials(c *gin.Context) {
	var req ChangeCredentialsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request"})
		return
	}

	session, err := h.authService.ChangeCredentials(bearerToken(c), service.ChangeCredentialsInput{
		Username:        req.Username,
		NewPassword:     req.NewPassword,
		ConfirmPassword: req.ConfirmPassword,
	})
	if err != nil {
		writeAuthError(c, err)
		return
	}
	c.JSON(http.StatusOK, session)
}

func (h *AuthHandler) Logout(c *gin.Context) {
	if err := h.authService.Logout(bearerToken(c)); err != nil {
		writeAuthError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func bearerToken(c *gin.Context) string {
	header := c.GetHeader("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
}

func writeAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidCredentials), errors.Is(err, service.ErrInvalidToken):
		c.JSON(http.StatusUnauthorized, gin.H{"message": "认证失败"})
	case errors.Is(err, service.ErrBootstrapUnavailable):
		c.JSON(http.StatusForbidden, gin.H{"message": "首次进入已关闭"})
	case errors.Is(err, service.ErrValidation):
		c.JSON(http.StatusBadRequest, gin.H{"message": "账号或密码不符合要求"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"message": "认证服务异常"})
	}
}
