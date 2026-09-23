package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type groupRateScheduleRepoStub struct {
	GroupRateScheduleRepository
	schedules         []GroupRateSchedule
	managedGroups     []int64
	managedUsers      []int64
	applyGroupCalls   int
	restoreGroupCalls int
	applyUserCalls    int
	restoreUserCalls  int
	invalidGroupIDs   []int64
}

func (s *groupRateScheduleRepoStub) ListByGroupID(context.Context, int64) ([]GroupRateSchedule, error) {
	return append([]GroupRateSchedule(nil), s.schedules...), nil
}
func (s *groupRateScheduleRepoStub) ReplaceForGroup(_ context.Context, _ int64, schedules []GroupRateScheduleInput) ([]GroupRateSchedule, error) {
	s.schedules = make([]GroupRateSchedule, 0, len(schedules))
	for i, input := range schedules {
		s.schedules = append(s.schedules, GroupRateSchedule{ID: int64(i + 1), StartMinute: input.StartMinute, EndMinute: input.EndMinute, RateMultiplier: input.RateMultiplier, Enabled: input.Enabled, TargetUserID: input.TargetUserID})
	}
	return s.schedules, nil
}
func (s *groupRateScheduleRepoStub) ListEnabled(context.Context) ([]GroupRateSchedule, error) {
	return s.schedules, nil
}
func (s *groupRateScheduleRepoStub) ListManagedGroupIDs(context.Context) ([]int64, error) {
	return s.managedGroups, nil
}
func (s *groupRateScheduleRepoStub) ListManagedTargetUserIDs(context.Context, int64) ([]int64, error) {
	return s.managedUsers, nil
}
func (s *groupRateScheduleRepoStub) ApplyScheduledMultiplier(context.Context, int64, int64, float64) (bool, error) {
	s.applyGroupCalls++
	return true, nil
}
func (s *groupRateScheduleRepoStub) RestoreBaseMultiplier(context.Context, int64) (bool, error) {
	s.restoreGroupCalls++
	return true, nil
}
func (s *groupRateScheduleRepoStub) ApplyScheduledUserMultiplier(context.Context, int64, int64, int64, float64) (bool, error) {
	s.applyUserCalls++
	return true, nil
}
func (s *groupRateScheduleRepoStub) RestoreBaseUserMultiplier(context.Context, int64, int64) (bool, error) {
	s.restoreUserCalls++
	return true, nil
}

type groupRateScheduleGroupRepoStub struct{ GroupRepository }

func (groupRateScheduleGroupRepoStub) GetByIDLite(context.Context, int64) (*Group, error) {
	return &Group{ID: 1}, nil
}

type groupRateScheduleInvalidatorStub struct{ ids []int64 }

func (s *groupRateScheduleInvalidatorStub) InvalidateAuthCacheByKey(context.Context, string)   {}
func (s *groupRateScheduleInvalidatorStub) InvalidateAuthCacheByUserID(context.Context, int64) {}
func (s *groupRateScheduleInvalidatorStub) InvalidateAuthCacheByGroupID(_ context.Context, id int64) {
	s.ids = append(s.ids, id)
}

func TestValidateGroupRateSchedulesRejectsOverlapPerTarget(t *testing.T) {
	err := validateGroupRateSchedules([]GroupRateScheduleInput{
		{StartMinute: 60, EndMinute: 120, RateMultiplier: 1},
		{StartMinute: 119, EndMinute: 180, RateMultiplier: 2},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "RATE_SCHEDULE_OVERLAP")
}

func TestGroupRateScheduleApplyGroupUpdatesDefaultAndUserTargets(t *testing.T) {
	userID := int64(9)
	repo := &groupRateScheduleRepoStub{
		schedules: []GroupRateSchedule{
			{ID: 1, GroupID: 1, StartMinute: 0, EndMinute: 1440, RateMultiplier: 1.5, Enabled: true},
			{ID: 2, GroupID: 1, TargetUserID: &userID, StartMinute: 0, EndMinute: 1440, RateMultiplier: 0.5, Enabled: true},
		},
		managedUsers: []int64{userID},
	}
	invalidator := &groupRateScheduleInvalidatorStub{}
	svc := NewGroupRateScheduleService(repo, groupRateScheduleGroupRepoStub{}, invalidator, time.Hour)

	require.NoError(t, svc.ApplyGroup(context.Background(), 1))
	require.Equal(t, 1, repo.applyGroupCalls)
	require.Equal(t, 1, repo.applyUserCalls)
	require.Equal(t, []int64{1, 1}, invalidator.ids)
}

func TestGroupRateScheduleApplyGroupRestoresWhenNoWindowIsActive(t *testing.T) {
	repo := &groupRateScheduleRepoStub{managedUsers: []int64{9}}
	svc := NewGroupRateScheduleService(repo, groupRateScheduleGroupRepoStub{}, &groupRateScheduleInvalidatorStub{}, time.Hour)

	require.NoError(t, svc.ApplyGroup(context.Background(), 1))
	require.Equal(t, 1, repo.restoreGroupCalls)
	require.Equal(t, 1, repo.restoreUserCalls)
}
