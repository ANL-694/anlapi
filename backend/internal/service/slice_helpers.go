package service

func containsInt64(items []int64, needle int64) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}
