package handler

import (
	"log/slog"

	"anlapi/internal/pkg/response"
	"anlapi/internal/server/middleware"
	"anlapi/internal/service"

	"github.com/gin-gonic/gin"
)

type ModelPlazaHandler struct {
	channelService *service.ChannelService
	apiKeyService  *service.APIKeyService
	settingService *service.SettingService
}

func NewModelPlazaHandler(
	channelService *service.ChannelService,
	apiKeyService *service.APIKeyService,
	settingService *service.SettingService,
) *ModelPlazaHandler {
	return &ModelPlazaHandler{
		channelService: channelService,
		apiKeyService:  apiKeyService,
		settingService: settingService,
	}
}

type modelPlazaOfficialPricing struct {
	InputPrice        *float64 `json:"input_price"`
	OutputPrice       *float64 `json:"output_price"`
	CacheWritePrice   *float64 `json:"cache_write_price"`
	CacheWrite1hPrice *float64 `json:"cache_write_1h_price,omitempty"`
	CacheReadPrice    *float64 `json:"cache_read_price"`
}

type modelPlazaModel struct {
	Name            string                     `json:"name"`
	Platform        string                     `json:"platform"`
	Pricing         *userSupportedModelPricing `json:"pricing"`
	OfficialPricing *modelPlazaOfficialPricing `json:"official_pricing"`
}

type modelPlazaGroup struct {
	ID                 int64             `json:"id"`
	Name               string            `json:"name"`
	Description        string            `json:"description"`
	Platform           string            `json:"platform"`
	SubscriptionType   string            `json:"subscription_type"`
	RateMultiplier     float64           `json:"rate_multiplier"`
	UserRateMultiplier *float64          `json:"user_rate_multiplier,omitempty"`
	PeakRateEnabled    bool              `json:"peak_rate_enabled"`
	PeakStart          string            `json:"peak_start"`
	PeakEnd            string            `json:"peak_end"`
	PeakRateMultiplier float64           `json:"peak_rate_multiplier"`
	IsExclusive        bool              `json:"is_exclusive"`
	Models             []modelPlazaModel `json:"models"`
}

type modelPlazaResponse struct {
	Description string            `json:"description"`
	Groups      []modelPlazaGroup `json:"groups"`
}

func (h *ModelPlazaHandler) Get(c *gin.Context) {
	if h.settingService == nil {
		response.NotFound(c, "Model plaza is not enabled")
		return
	}
	rt := h.settingService.GetModelPlazaRuntime(c.Request.Context())
	if !rt.Enabled {
		response.NotFound(c, "Model plaza is not enabled")
		return
	}

	subject, authed := middleware.GetAuthSubjectFromContext(c)
	if rt.RequireAuth && !authed {
		response.Unauthorized(c, "Authentication required")
		return
	}

	groups, err := h.channelService.ListPlazaGroups(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	var allowedExclusive map[int64]struct{}
	var userRates map[int64]float64
	if authed {
		allowedExclusive, err = h.apiKeyService.GetUserAllowedGroupIDSet(c.Request.Context(), subject.UserID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		userRates, err = h.apiKeyService.GetUserGroupRates(c.Request.Context(), subject.UserID)
		if err != nil {
			slog.Warn("model_plaza_user_rates_failed", "error", err, "user_id", subject.UserID)
			userRates = nil
		}
	}

	visible := filterPlazaVisibleGroups(groups, allowedExclusive)

	out := make([]modelPlazaGroup, 0, len(visible))
	for i := range visible {
		out = append(out, toModelPlazaGroupDTO(&visible[i], userRates))
	}
	response.Success(c, modelPlazaResponse{
		Description: rt.Description,
		Groups:      out,
	})
}

func filterPlazaVisibleGroups(
	groups []service.PlazaGroup,
	allowedExclusive map[int64]struct{},
) []service.PlazaGroup {
	visible := make([]service.PlazaGroup, 0, len(groups))
	for _, g := range groups {
		if g.IsExclusive {
			if allowedExclusive == nil {
				continue
			}
			if _, ok := allowedExclusive[g.ID]; !ok {
				continue
			}
		}
		visible = append(visible, g)
	}
	return visible
}

func toModelPlazaGroupDTO(g *service.PlazaGroup, userRates map[int64]float64) modelPlazaGroup {
	models := make([]modelPlazaModel, 0, len(g.Models))
	for i := range g.Models {
		m := &g.Models[i]
		models = append(models, modelPlazaModel{
			Name:            m.Name,
			Platform:        m.Platform,
			Pricing:         toUserPricing(m.Pricing),
			OfficialPricing: toModelPlazaOfficialPricing(m.OfficialPricing),
		})
	}
	dto := modelPlazaGroup{
		ID:                 g.ID,
		Name:               g.Name,
		Description:        g.Description,
		Platform:           g.Platform,
		SubscriptionType:   g.SubscriptionType,
		RateMultiplier:     g.RateMultiplier,
		PeakRateEnabled:    g.PeakRateEnabled,
		PeakStart:          g.PeakStart,
		PeakEnd:            g.PeakEnd,
		PeakRateMultiplier: g.PeakRateMultiplier,
		IsExclusive:        g.IsExclusive,
		Models:             models,
	}
	if rate, ok := userRates[g.ID]; ok {
		dto.UserRateMultiplier = &rate
	}
	return dto
}

func toModelPlazaOfficialPricing(p *service.PlazaOfficialPricing) *modelPlazaOfficialPricing {
	if p == nil {
		return nil
	}
	return &modelPlazaOfficialPricing{
		InputPrice:        p.InputPrice,
		OutputPrice:       p.OutputPrice,
		CacheWritePrice:   p.CacheWritePrice,
		CacheWrite1hPrice: p.CacheWrite1hPrice,
		CacheReadPrice:    p.CacheReadPrice,
	}
}
