package routes

import (
	"anlapi/internal/handler"
	"anlapi/internal/server/middleware"
	"anlapi/internal/service"

	"github.com/gin-gonic/gin"
)

func RegisterModelPlazaRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	optionalJWT middleware.OptionalJWTAuthMiddleware,
	settingService *service.SettingService,
	panelRateLimiter *middleware.PanelRateLimiter,
) {
	plaza := v1.Group("/model-plaza")
	plaza.Use(panelRateLimiter.PublicIP())
	plaza.Use(gin.HandlerFunc(optionalJWT))
	plaza.Use(middleware.BackendModeUserGuard(settingService))
	{
		plaza.GET("", h.ModelPlaza.Get)
	}
}
