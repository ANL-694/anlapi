//go:build unit

package service

import (
	"context"
	"time"
)

type outboxCleanupDeleteCall struct {
	watermark int64
	limit     int
}

type outboxCleanupRepo struct {
	events              []SchedulerOutboxEvent
	rows                []int64
	maxIDCalls          int
	maxIDErr            error
	lockAcquired        bool
	lockAttempts        int
	releaseCount        int
	deleteCalls         []outboxCleanupDeleteCall
	firstCreatedAfterID []int64
}

func (r *outboxCleanupRepo) ListAfterAndReleaseDedup(_ context.Context, afterID int64, limit int) ([]SchedulerOutboxEvent, error) {
	events := make([]SchedulerOutboxEvent, 0, len(r.events))
	for _, event := range r.events {
		if event.ID <= afterID {
			continue
		}
		events = append(events, event)
		if limit > 0 && len(events) >= limit {
			break
		}
	}
	return events, nil
}

func (r *outboxCleanupRepo) FirstCreatedAtAfter(_ context.Context, afterID int64) (time.Time, bool, error) {
	r.firstCreatedAfterID = append(r.firstCreatedAfterID, afterID)
	for _, event := range r.events {
		if event.ID > afterID {
			return event.CreatedAt, true, nil
		}
	}
	return time.Time{}, false, nil
}

func (r *outboxCleanupRepo) MaxID(context.Context) (int64, error) {
	r.maxIDCalls++
	if r.maxIDErr != nil {
		return 0, r.maxIDErr
	}
	var maxID int64
	for _, id := range r.rows {
		if id > maxID {
			maxID = id
		}
	}
	return maxID, nil
}

func (r *outboxCleanupRepo) DeleteConsumedUpTo(_ context.Context, watermark int64, limit int) (int64, error) {
	r.deleteCalls = append(r.deleteCalls, outboxCleanupDeleteCall{watermark: watermark, limit: limit})
	if watermark <= 0 || limit <= 0 {
		return 0, nil
	}
	var deleted int64
	kept := make([]int64, 0, len(r.rows))
	for _, id := range r.rows {
		if id <= watermark && deleted < int64(limit) {
			deleted++
			continue
		}
		kept = append(kept, id)
	}
	r.rows = kept
	return deleted, nil
}

func (r *outboxCleanupRepo) TryAcquireCleanupLock(context.Context) (SchedulerOutboxCleanupLease, bool, error) {
	r.lockAttempts++
	if !r.lockAcquired {
		return nil, false, nil
	}
	return outboxCleanupLease{release: func() { r.releaseCount++ }}, true, nil
}

type outboxCleanupLease struct {
	release func()
}

func (l outboxCleanupLease) Release() {
	if l.release != nil {
		l.release()
	}
}
