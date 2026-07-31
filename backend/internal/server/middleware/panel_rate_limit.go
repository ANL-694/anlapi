package middleware

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"anlapi/internal/middleware"
	"anlapi/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const panelRateLimitWindow = time.Minute

type panelRateLimitAllower interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (middleware.AllowResult, error)
}

type PanelRateLimiter struct {
	limiter        panelRateLimitAllower
	settingService *service.SettingService
}

func NewPanelRateLimiter(redisClient *redis.Client, settingService *service.SettingService) *PanelRateLimiter {
	return &PanelRateLimiter{
		limiter:        middleware.NewRateLimiter(redisClient),
		settingService: settingService,
	}
}

func (p *PanelRateLimiter) Global() gin.HandlerFunc {
	return p.userScoped("global", func(s service.PanelRateLimitSettings) int { return s.UserRPM })
}

func (p *PanelRateLimiter) Heavy() gin.HandlerFunc {
	return p.userScoped("heavy", func(s service.PanelRateLimitSettings) int { return s.HeavyRPM })
}

func (p *PanelRateLimiter) userScoped(scope string, limitOf func(service.PanelRateLimitSettings) int) gin.HandlerFunc {
	return func(c *gin.Context) {
		if p == nil || p.limiter == nil || p.settingService == nil {
			c.Next()
			return
		}
		settings := p.settingService.GetPanelRateLimitSettingsCached(c.Request.Context())
		if !settings.Enabled {
			c.Next()
			return
		}
		limit := limitOf(settings)
		if limit <= 0 {
			c.Next()
			return
		}
		subject, ok := GetAuthSubjectFromContext(c)
		if !ok || subject.UserID <= 0 {
			c.Next()
			return
		}
		if settings.ExemptAdmin {
			if role, hasRole := GetUserRoleFromContext(c); hasRole && role == service.RoleAdmin {
				c.Next()
				return
			}
		}

		key := "panel:" + scope + ":user:" + strconv.FormatInt(subject.UserID, 10)
		result, err := p.limiter.Allow(c.Request.Context(), key, limit, panelRateLimitWindow)
		if err != nil {
			slog.Warn("panel rate limit check failed, allowing request", "scope", scope, "error", err)
			c.Next()
			return
		}
		if !result.Allowed {
			abortPanelRateLimited(c, result.RetryAfter)
			return
		}
		c.Next()
	}
}

func (p *PanelRateLimiter) PublicIP() gin.HandlerFunc {
	return func(c *gin.Context) {
		if p == nil || p.limiter == nil || p.settingService == nil {
			c.Next()
			return
		}
		settings := p.settingService.GetPanelRateLimitSettingsCached(c.Request.Context())
		if !settings.Enabled || settings.PublicIPRPM <= 0 {
			c.Next()
			return
		}
		clientIP := SecurityClientIP(c)
		if !isPubliclyRoutableClientIP(clientIP) {
			c.Next()
			return
		}

		result, err := p.limiter.Allow(c.Request.Context(), "panel:public:ip:"+clientIP, settings.PublicIPRPM, panelRateLimitWindow)
		if err != nil {
			slog.Warn("panel public rate limit check failed, allowing request", "error", err)
			c.Next()
			return
		}
		if !result.Allowed {
			abortPanelRateLimited(c, result.RetryAfter)
			return
		}
		c.Next()
	}
}

func isPubliclyRoutableClientIP(clientIP string) bool {
	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	return ip.IsGlobalUnicast()
}

func abortPanelRateLimited(c *gin.Context, retryAfter time.Duration) {
	if retryAfter <= 0 {
		retryAfter = panelRateLimitWindow
	}
	seconds := int64(retryAfter / time.Second)
	if retryAfter%time.Second > 0 {
		seconds++
	}
	c.Header("Retry-After", strconv.FormatInt(seconds, 10))
	AbortWithError(c, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests, please slow down and try again later")
}
