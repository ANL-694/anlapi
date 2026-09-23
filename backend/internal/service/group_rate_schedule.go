package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

const (
	defaultGroupRateScheduleInterval = time.Minute
	groupRateScheduleApplyTimeout    = 30 * time.Second
)

// GroupRateSchedule is a daily interval in the configured application timezone.
// A nil TargetUserID updates the group default; a non-nil target updates only that
// user's existing group-rate override for the active interval.
type GroupRateSchedule struct {
	ID              int64     `json:"id"`
	GroupID         int64     `json:"group_id"`
	TargetUserID    *int64    `json:"target_user_id,omitempty"`
	TargetUserName  string    `json:"target_user_name,omitempty"`
	TargetUserEmail string    `json:"target_user_email,omitempty"`
	StartMinute     int       `json:"start_minute"`
	EndMinute       int       `json:"end_minute"`
	RateMultiplier  float64   `json:"rate_multiplier"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type GroupRateScheduleInput struct {
	TargetUserID   *int64
	StartMinute    int
	EndMinute      int
	RateMultiplier float64
	Enabled        bool
}

type GroupRateScheduleRepository interface {
	ListByGroupID(ctx context.Context, groupID int64) ([]GroupRateSchedule, error)
	ReplaceForGroup(ctx context.Context, groupID int64, schedules []GroupRateScheduleInput) ([]GroupRateSchedule, error)
	ListEnabled(ctx context.Context) ([]GroupRateSchedule, error)
	ListManagedGroupIDs(ctx context.Context) ([]int64, error)
	ListManagedTargetUserIDs(ctx context.Context, groupID int64) ([]int64, error)
	ApplyScheduledMultiplier(ctx context.Context, groupID int64, scheduleID int64, rateMultiplier float64) (bool, error)
	RestoreBaseMultiplier(ctx context.Context, groupID int64) (bool, error)
	ApplyScheduledUserMultiplier(ctx context.Context, groupID int64, userID int64, scheduleID int64, rateMultiplier float64) (bool, error)
	RestoreBaseUserMultiplier(ctx context.Context, groupID int64, userID int64) (bool, error)
}

type GroupRateScheduleService struct {
	repo                 GroupRateScheduleRepository
	groupRepo            GroupRepository
	authCacheInvalidator APIKeyAuthCacheInvalidator
	interval             time.Duration
	applyMu              sync.Mutex
	startOnce            sync.Once
	stopOnce             sync.Once
	stopCh               chan struct{}
	doneCh               chan struct{}
}

func NewGroupRateScheduleService(repo GroupRateScheduleRepository, groupRepo GroupRepository, authCacheInvalidator APIKeyAuthCacheInvalidator, interval time.Duration) *GroupRateScheduleService {
	if interval <= 0 {
		interval = defaultGroupRateScheduleInterval
	}
	return &GroupRateScheduleService{
		repo: repo, groupRepo: groupRepo, authCacheInvalidator: authCacheInvalidator, interval: interval,
		stopCh: make(chan struct{}), doneCh: make(chan struct{}),
	}
}

func (s *GroupRateScheduleService) Start() {
	if s != nil {
		s.startOnce.Do(func() { go s.run() })
	}
}

func (s *GroupRateScheduleService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
		s.startOnce.Do(func() { close(s.doneCh) })
		<-s.doneCh
	})
}

func (s *GroupRateScheduleService) run() {
	defer close(s.doneCh)
	s.applyWithTimeout()
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.applyWithTimeout()
		case <-s.stopCh:
			return
		}
	}
}

func (s *GroupRateScheduleService) applyWithTimeout() {
	ctx, cancel := context.WithTimeout(context.Background(), groupRateScheduleApplyTimeout)
	defer cancel()
	if err := s.ApplyOnce(ctx); err != nil {
		logger.LegacyPrintf("service.group_rate_schedule", "apply schedules failed: %v", err)
	}
}

func (s *GroupRateScheduleService) List(ctx context.Context, groupID int64) ([]GroupRateSchedule, error) {
	if groupID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_GROUP_ID", "group_id must be greater than 0")
	}
	if s == nil || s.repo == nil || s.groupRepo == nil {
		return nil, infraerrors.InternalServer("RATE_SCHEDULE_UNAVAILABLE", "group rate schedule service is not configured")
	}
	if _, err := s.groupRepo.GetByIDLite(ctx, groupID); err != nil {
		return nil, err
	}
	return s.repo.ListByGroupID(ctx, groupID)
}

func (s *GroupRateScheduleService) Replace(ctx context.Context, groupID int64, schedules []GroupRateScheduleInput) ([]GroupRateSchedule, error) {
	if groupID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_GROUP_ID", "group_id must be greater than 0")
	}
	if s == nil || s.repo == nil || s.groupRepo == nil {
		return nil, infraerrors.InternalServer("RATE_SCHEDULE_UNAVAILABLE", "group rate schedule service is not configured")
	}
	if _, err := s.groupRepo.GetByIDLite(ctx, groupID); err != nil {
		return nil, err
	}
	if err := validateGroupRateSchedules(schedules); err != nil {
		return nil, err
	}
	updated, err := s.repo.ReplaceForGroup(ctx, groupID, schedules)
	if err != nil {
		return nil, err
	}
	if err := s.ApplyGroup(ctx, groupID); err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *GroupRateScheduleService) ApplyOnce(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return nil
	}
	s.applyMu.Lock()
	defer s.applyMu.Unlock()
	enabled, err := s.repo.ListEnabled(ctx)
	if err != nil {
		return err
	}
	managedIDs, err := s.repo.ListManagedGroupIDs(ctx)
	if err != nil {
		return err
	}
	byGroup := make(map[int64][]GroupRateSchedule)
	for _, schedule := range enabled {
		byGroup[schedule.GroupID] = append(byGroup[schedule.GroupID], schedule)
	}
	minute := currentGroupRateScheduleMinute()
	for _, groupID := range managedIDs {
		if err := s.applyGroupLocked(ctx, groupID, byGroup[groupID], minute); err != nil {
			logger.LegacyPrintf("service.group_rate_schedule", "apply group schedule failed: group=%d err=%v", groupID, err)
		}
	}
	return nil
}

func (s *GroupRateScheduleService) ApplyGroup(ctx context.Context, groupID int64) error {
	if s == nil || s.repo == nil {
		return nil
	}
	s.applyMu.Lock()
	defer s.applyMu.Unlock()
	schedules, err := s.repo.ListByGroupID(ctx, groupID)
	if err != nil {
		return err
	}
	enabled := make([]GroupRateSchedule, 0, len(schedules))
	for _, schedule := range schedules {
		if schedule.Enabled {
			enabled = append(enabled, schedule)
		}
	}
	return s.applyGroupLocked(ctx, groupID, enabled, currentGroupRateScheduleMinute())
}

func (s *GroupRateScheduleService) applyGroupLocked(ctx context.Context, groupID int64, schedules []GroupRateSchedule, minute int) error {
	var groupSchedules []GroupRateSchedule
	byUser := make(map[int64][]GroupRateSchedule)
	for _, schedule := range schedules {
		if schedule.TargetUserID == nil {
			groupSchedules = append(groupSchedules, schedule)
			continue
		}
		if *schedule.TargetUserID > 0 {
			byUser[*schedule.TargetUserID] = append(byUser[*schedule.TargetUserID], schedule)
		}
	}
	active := findActiveGroupRateSchedule(groupSchedules, minute)
	var changed bool
	var err error
	if active == nil {
		changed, err = s.repo.RestoreBaseMultiplier(ctx, groupID)
	} else {
		changed, err = s.repo.ApplyScheduledMultiplier(ctx, groupID, active.ID, active.RateMultiplier)
	}
	if err != nil {
		return err
	}
	if changed && s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByGroupID(ctx, groupID)
	}
	userIDs, err := s.repo.ListManagedTargetUserIDs(ctx, groupID)
	if err != nil {
		return err
	}
	for _, userID := range userIDs {
		if err := s.applyUserLocked(ctx, groupID, userID, byUser[userID], minute); err != nil {
			logger.LegacyPrintf("service.group_rate_schedule", "apply user schedule failed: group=%d user=%d err=%v", groupID, userID, err)
		}
	}
	return nil
}

func (s *GroupRateScheduleService) applyUserLocked(ctx context.Context, groupID, userID int64, schedules []GroupRateSchedule, minute int) error {
	active := findActiveGroupRateSchedule(schedules, minute)
	var changed bool
	var err error
	if active == nil {
		changed, err = s.repo.RestoreBaseUserMultiplier(ctx, groupID, userID)
	} else {
		changed, err = s.repo.ApplyScheduledUserMultiplier(ctx, groupID, userID, active.ID, active.RateMultiplier)
	}
	if err != nil {
		return err
	}
	if changed && s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByGroupID(ctx, groupID)
	}
	return nil
}

func validateGroupRateSchedules(schedules []GroupRateScheduleInput) error {
	byTarget := make(map[int64][]GroupRateScheduleInput)
	for i, schedule := range schedules {
		if schedule.StartMinute < 0 || schedule.StartMinute >= 1440 || schedule.EndMinute <= schedule.StartMinute || schedule.EndMinute > 1440 {
			return infraerrors.BadRequest("INVALID_RATE_SCHEDULE", fmt.Sprintf("invalid time range (index=%d)", i))
		}
		if schedule.RateMultiplier <= 0 || math.IsNaN(schedule.RateMultiplier) || math.IsInf(schedule.RateMultiplier, 0) {
			return infraerrors.BadRequest("INVALID_RATE_SCHEDULE", fmt.Sprintf("rate_multiplier must be greater than 0 (index=%d)", i))
		}
		key := int64(0)
		if schedule.TargetUserID != nil {
			if *schedule.TargetUserID <= 0 {
				return infraerrors.BadRequest("INVALID_RATE_SCHEDULE", fmt.Sprintf("target_user_id must be greater than 0 (index=%d)", i))
			}
			key = *schedule.TargetUserID
		}
		byTarget[key] = append(byTarget[key], schedule)
	}
	for _, targetSchedules := range byTarget {
		sort.Slice(targetSchedules, func(i, j int) bool {
			if targetSchedules[i].StartMinute == targetSchedules[j].StartMinute {
				return targetSchedules[i].EndMinute < targetSchedules[j].EndMinute
			}
			return targetSchedules[i].StartMinute < targetSchedules[j].StartMinute
		})
		for i := 1; i < len(targetSchedules); i++ {
			if targetSchedules[i].StartMinute < targetSchedules[i-1].EndMinute {
				return infraerrors.Conflict("RATE_SCHEDULE_OVERLAP", "schedule time ranges cannot overlap for the same target")
			}
		}
	}
	return nil
}

func findActiveGroupRateSchedule(schedules []GroupRateSchedule, minute int) *GroupRateSchedule {
	for i := range schedules {
		if schedules[i].Enabled && minute >= schedules[i].StartMinute && minute < schedules[i].EndMinute {
			return &schedules[i]
		}
	}
	return nil
}

func currentGroupRateScheduleMinute() int {
	now := timezone.Now()
	return now.Hour()*60 + now.Minute()
}
