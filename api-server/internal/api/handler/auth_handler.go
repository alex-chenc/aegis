package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"api-server/internal/model"
	"api-server/internal/repository"
	"api-server/internal/service"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authService *service.AuthService
	roleRepo    *repository.RoleRepo
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

type ResetPasswordRequest struct {
	ResetKey        string `json:"reset_key"`
	NewPassword     string `json:"new_password"`
	ConfirmPassword string `json:"confirm_password"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
	ConfirmPassword string `json:"confirm_password"`
}

func NewAuthHandler(authService *service.AuthService, roleRepo *repository.RoleRepo) *AuthHandler {
	return &AuthHandler{authService: authService, roleRepo: roleRepo}
}

func (h *AuthHandler) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/status", h.GetStatus)
	group.POST("/bootstrap-login", h.BootstrapLogin)
	group.POST("/login", h.Login)
	group.GET("/me", h.Me)
	group.POST("/change-credentials", h.ChangeCredentials)
	group.POST("/reset-password", h.ResetPassword)
	group.POST("/change-password", h.ChangePassword)
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
	role := h.resolveUserRole(session.Username)
	c.JSON(http.StatusOK, gin.H{
		"token":                 session.Token,
		"username":              session.Username,
		"force_password_change": session.ForcePasswordChange,
		"role":                  role,
		"capabilities":          h.capabilitiesForRole(role),
		"capability_version":    "role-policy-v1",
		"capability_expires_at": time.Now().UTC().Add(15 * time.Minute),
	})
}

func (h *AuthHandler) Me(c *gin.Context) {
	authCtx, err := h.authService.ValidateToken(bearerToken(c))
	if err != nil {
		writeAuthError(c, err)
		return
	}
	role := h.resolveUserRole(authCtx.Username)
	c.JSON(http.StatusOK, gin.H{
		"username":              authCtx.Username,
		"force_password_change": authCtx.ForcePasswordChange,
		"role":                  role,
		"capabilities":          h.capabilitiesForRole(role),
		"capability_version":    "role-policy-v1",
		"capability_expires_at": time.Now().UTC().Add(15 * time.Minute),
	})
}

// resolveUserRole returns the user's role, auto-assigning admin only on
// fresh installs where no role records exist yet.
func (h *AuthHandler) resolveUserRole(username string) string {
	if h.roleRepo == nil {
		return model.RoleSecurityAnalyst
	}
	has, err := h.roleRepo.HasRoleRecord(username)
	if err != nil {
		return model.RoleSecurityAnalyst
	}
	if has {
		role, _ := h.roleRepo.GetRole(username)
		return role
	}
	// No role record for this user. Only auto-assign admin if the entire
	// role_permissions table is empty (fresh install). Otherwise assign
	// the lowest-privilege role to avoid privilege escalation.
	anyRoles, err := h.roleRepo.HasAnyRoles()
	if err != nil {
		return model.RoleSecurityAnalyst
	}
	if !anyRoles {
		_ = h.roleRepo.SetRole(username, model.RoleAdmin)
		return model.RoleAdmin
	}
	_ = h.roleRepo.SetRole(username, model.RoleSecurityAnalyst)
	return model.RoleSecurityAnalyst
}

func (h *AuthHandler) capabilitiesForRole(role string) []string {
	if h.roleRepo == nil {
		return nil
	}
	return h.roleRepo.ListPermissions(role)
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

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request"})
		return
	}

	session, err := h.authService.ResetPassword(req.ResetKey, req.NewPassword, req.ConfirmPassword)
	if err != nil {
		writeAuthError(c, err)
		return
	}
	c.JSON(http.StatusOK, session)
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request"})
		return
	}

	session, err := h.authService.ChangePassword(bearerToken(c), service.ChangePasswordInput{
		CurrentPassword: req.CurrentPassword,
		NewPassword:     req.NewPassword,
		ConfirmPassword: req.ConfirmPassword,
	})
	if err != nil {
		writeAuthError(c, err)
		return
	}
	c.JSON(http.StatusOK, session)
}

func bearerToken(c *gin.Context) string {
	header := c.GetHeader("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
}

func writeAuthError(c *gin.Context, err error) {
	var rateLimitErr *service.LoginRateLimitError
	switch {
	case errors.As(err, &rateLimitErr):
		minutes := int(rateLimitErr.Remaining.Minutes())
		if minutes < 1 {
			minutes = 1
		}
		c.JSON(http.StatusTooManyRequests, gin.H{"message": fmt.Sprintf("密码错误次数过多，请 %d 分钟后再试", minutes)})
	case errors.Is(err, service.ErrInvalidCredentials), errors.Is(err, service.ErrInvalidToken):
		c.JSON(http.StatusUnauthorized, gin.H{"message": "密码错误"})
	case errors.Is(err, service.ErrBootstrapUnavailable):
		c.JSON(http.StatusForbidden, gin.H{"message": "首次进入已关闭"})
	case errors.Is(err, service.ErrValidation):
		c.JSON(http.StatusBadRequest, gin.H{"message": "账号或密码不符合要求"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"message": "认证服务异常"})
	}
}
