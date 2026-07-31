package middleware

import (
	"strings"

	"anlapi/internal/service"

	"github.com/gin-gonic/gin"
)

func NewOptionalJWTAuthMiddleware(
	authService *service.AuthService,
	userService *service.UserService,
	settingService *service.SettingService,
	auditService *service.AuditLogService,
) OptionalJWTAuthMiddleware {
	strict := jwtAuth(authService, userService, userService, settingService, auditService)
	return OptionalJWTAuthMiddleware(func(c *gin.Context) {
		if strings.TrimSpace(c.GetHeader("Authorization")) == "" {
			c.Next()
			return
		}
		strict(c)
	})
}
