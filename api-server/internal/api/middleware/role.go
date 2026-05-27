package middleware

import (
	"net/http"

	"api-server/internal/repository"

	"github.com/gin-gonic/gin"
)

func RoleMiddleware(roleRepo *repository.RoleRepo, requiredOperation string) gin.HandlerFunc {
	return func(c *gin.Context) {
		username, exists := c.Get("auth_username")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "unauthorized"})
			return
		}

		role, err := roleRepo.GetRole(username.(string))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to get role"})
			return
		}

		if !roleRepo.HasPermission(role, requiredOperation) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "message": "insufficient permissions"})
			return
		}

		c.Set("role", role)
		c.Next()
	}
}
