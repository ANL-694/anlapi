package service

import "time"

const subscriptionDayDuration = 24 * time.Hour

func subscriptionDaysRemainingAt(expiresAt, now time.Time) int {
	remaining := expiresAt.Sub(now)
	if remaining <= 0 {
		return 0
	}

	days := int(remaining / subscriptionDayDuration)
	if remaining%subscriptionDayDuration != 0 {
		days++
	}
	return days
}
