package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type PlazaOfficialPricing struct {
	InputPrice        *float64
	OutputPrice       *float64
	CacheWritePrice   *float64 // 5m 缓存写入（= LiteLLM cache_creation）
	CacheWrite1hPrice *float64 // 1h 缓存写入（LiteLLM cache_creation_above_1hr）
	CacheReadPrice    *float64
}

type PlazaModel struct {
	Name            string
	Platform        string
	Pricing         *ChannelModelPricing
	OfficialPricing *PlazaOfficialPricing
}

type PlazaGroup struct {
	ID                 int64
	Name               string
	Description        string
	Platform           string
	SubscriptionType   string
	RateMultiplier     float64
	PeakRateEnabled    bool
	PeakStart          string
	PeakEnd            string
	PeakRateMultiplier float64
	IsExclusive        bool
	Models             []PlazaModel
}

func (s *ChannelService) ListPlazaGroups(ctx context.Context) ([]PlazaGroup, error) {
	channels, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
	}

	sort.SliceStable(channels, func(i, j int) bool {
		return strings.ToLower(channels[i].Name) < strings.ToLower(channels[j].Name)
	})

	byGroup := make(map[int64]*PlazaGroup, len(groups))
	order := make([]int64, 0, len(groups))
	for i := range groups {
		g := groups[i]
		byGroup[g.ID] = &PlazaGroup{
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
		}
		order = append(order, g.ID)
	}

	modelIdx := make(map[int64]map[string]int, len(groups))
	for i := range channels {
		ch := &channels[i]
		if ch.Status != StatusActive {
			continue
		}
		ch.normalizeBillingModelSource()
		supported := ch.SupportedModels()
		s.fillGlobalPricingFallback(supported)

		for _, gid := range ch.GroupIDs {
			pg, ok := byGroup[gid]
			if !ok {
				continue
			}
			idx := modelIdx[gid]
			if idx == nil {
				idx = make(map[string]int, len(supported))
				modelIdx[gid] = idx
			}
			for j := range supported {
				m := supported[j]
				if m.Platform != pg.Platform {
					continue
				}
				if at, seen := idx[m.Name]; seen {
					if pg.Models[at].Pricing == nil && m.Pricing != nil {
						pg.Models[at].Pricing = m.Pricing
					}
					continue
				}
				idx[m.Name] = len(pg.Models)
				pg.Models = append(pg.Models, PlazaModel{
					Name:     m.Name,
					Platform: m.Platform,
					Pricing:  m.Pricing,
				})
			}
		}
	}

	officialMemo := make(map[string]*PlazaOfficialPricing)
	out := make([]PlazaGroup, 0, len(order))
	for _, gid := range order {
		pg := byGroup[gid]
		if len(pg.Models) == 0 {
			continue
		}
		sort.SliceStable(pg.Models, func(i, j int) bool { return pg.Models[i].Name < pg.Models[j].Name })
		for j := range pg.Models {
			pg.Models[j].OfficialPricing = s.lookupOfficialPricing(pg.Models[j].Name, officialMemo)
		}
		out = append(out, *pg)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RateMultiplier != out[j].RateMultiplier {
			return out[i].RateMultiplier < out[j].RateMultiplier
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (s *ChannelService) lookupOfficialPricing(modelName string, memo map[string]*PlazaOfficialPricing) *PlazaOfficialPricing {
	if s.pricingService == nil {
		return nil
	}
	if cached, ok := memo[modelName]; ok {
		return cached
	}
	var result *PlazaOfficialPricing
	if lp := s.pricingService.GetModelPricing(modelName); lp != nil && !lp.TokenPricingAbsent {
		result = &PlazaOfficialPricing{
			InputPrice:        nonZeroPtr(lp.InputCostPerToken),
			OutputPrice:       nonZeroPtr(lp.OutputCostPerToken),
			CacheWritePrice:   nonZeroPtr(lp.CacheCreationInputTokenCost),
			CacheWrite1hPrice: nonZeroPtr(lp.CacheCreationInputTokenCostAbove1hr),
			CacheReadPrice:    nonZeroPtr(lp.CacheReadInputTokenCost),
		}
		if result.InputPrice == nil && result.OutputPrice == nil &&
			result.CacheWritePrice == nil && result.CacheWrite1hPrice == nil && result.CacheReadPrice == nil {
			result = nil
		}
	}
	memo[modelName] = result
	return result
}
