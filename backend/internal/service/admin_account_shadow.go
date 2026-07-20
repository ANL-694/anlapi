package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	infraerrors "anl-api/internal/pkg/errors"
)

type AccountShadowRepository interface {
	ListShadowsByParent(ctx context.Context, parentID int64) ([]*Account, error)
}

func accountShadowRepository(repo AccountRepository) (AccountShadowRepository, error) {
	shadowRepo, ok := repo.(AccountShadowRepository)
	if !ok {
		return nil, infraerrors.InternalServer("SPARK_SHADOW_REPOSITORY_UNAVAILABLE", "spark shadow repository is unavailable")
	}
	return shadowRepo, nil
}

// CreateShadow creates the single spark-dimension shadow linked to an OpenAI OAuth parent.
func (s *adminServiceImpl) CreateShadow(ctx context.Context, parentID int64, opts ShadowOptions) (*Account, error) {
	parent, err := s.accountRepo.GetByID(ctx, parentID)
	if err != nil {
		return nil, fmt.Errorf("get parent account: %w", err)
	}
	if !parent.IsOpenAIOAuth() {
		return nil, infraerrors.New(http.StatusBadRequest, "SPARK_SHADOW_INVALID_PARENT", "spark shadow requires an OpenAI OAuth parent account")
	}
	if parent.IsCredentialShadow() {
		return nil, infraerrors.New(http.StatusBadRequest, "SPARK_SHADOW_PARENT_IS_SHADOW", "spark shadow parent must be a real account, not another spark shadow")
	}

	shadowRepo, err := accountShadowRepository(s.accountRepo)
	if err != nil {
		return nil, err
	}
	shadows, err := shadowRepo.ListShadowsByParent(ctx, parentID)
	if err != nil {
		return nil, fmt.Errorf("check existing spark shadows: %w", err)
	}
	if len(shadows) > 0 {
		return nil, infraerrors.New(http.StatusConflict, "SPARK_SHADOW_ALREADY_EXISTS", "parent account already has a spark shadow account")
	}

	groupIDs := append([]int64(nil), opts.GroupIDs...)
	if len(groupIDs) > 0 {
		if s.groupRepo != nil {
			if err := s.validateGroupIDsExist(ctx, groupIDs); err != nil {
				return nil, err
			}
		}
	} else if len(parent.GroupIDs) > 0 {
		groupIDs = append(groupIDs, parent.GroupIDs...)
	} else if s.groupRepo != nil {
		groups, groupErr := s.groupRepo.ListActiveByPlatform(ctx, PlatformOpenAI)
		if groupErr == nil {
			for _, group := range groups {
				if group.Name == PlatformOpenAI+"-default" {
					groupIDs = []int64{group.ID}
					break
				}
			}
		}
	}

	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = parent.Name + " (Spark)"
	}
	if runes := []rune(name); len(runes) > 100 {
		name = string(runes[:100])
	}
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = parent.Concurrency
	}
	priority := opts.Priority
	if priority <= 0 {
		priority = parent.Priority
	}

	shadow := &Account{
		Name:            name,
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		Status:          StatusActive,
		Credentials:     map[string]any{"model_mapping": defaultSparkShadowModelMapping()},
		ParentAccountID: &parentID,
		QuotaDimension:  QuotaDimensionSpark,
		ProxyID:         parent.ProxyID,
		Priority:        priority,
		Concurrency:     concurrency,
		Schedulable:     true,
		Extra: map[string]any{
			openAILongContextBillingEnabledKey: parent.IsOpenAILongContextBillingEnabled(),
		},
	}
	if err := s.accountRepo.Create(ctx, shadow); err != nil {
		if existing, queryErr := shadowRepo.ListShadowsByParent(ctx, parentID); queryErr == nil && len(existing) > 0 {
			return nil, infraerrors.New(http.StatusConflict, "SPARK_SHADOW_ALREADY_EXISTS", "parent account already has a spark shadow account")
		}
		return nil, fmt.Errorf("create spark shadow: %w", err)
	}

	if len(groupIDs) > 0 {
		if err := s.accountRepo.BindGroups(ctx, shadow.ID, groupIDs); err != nil {
			if deleteErr := s.accountRepo.Delete(context.WithoutCancel(ctx), shadow.ID); deleteErr != nil {
				slog.Error("spark_shadow_bind_groups_rollback_failed", "shadow_id", shadow.ID, "parent_id", parentID, "delete_err", deleteErr)
			}
			return nil, fmt.Errorf("bind groups for spark shadow: %w", err)
		}
		shadow.GroupIDs = append([]int64(nil), groupIDs...)
	}
	return shadow, nil
}

func (s *adminServiceImpl) propagateProxyToShadows(ctx context.Context, parentID int64, proxyID *int64) error {
	return propagateAccountProxyToShadows(ctx, s.accountRepo, parentID, proxyID)
}

func propagateAccountProxyToShadows(ctx context.Context, repo AccountRepository, parentID int64, proxyID *int64) error {
	shadowRepo, err := accountShadowRepository(repo)
	if err != nil {
		return err
	}
	shadows, err := shadowRepo.ListShadowsByParent(ctx, parentID)
	if err != nil {
		return fmt.Errorf("list spark shadows for proxy propagation: %w", err)
	}
	for _, shadow := range shadows {
		shadow.ProxyID = proxyID
		if err := repo.Update(ctx, shadow); err != nil {
			return fmt.Errorf("update spark shadow %d proxy: %w", shadow.ID, err)
		}
	}
	return nil
}
