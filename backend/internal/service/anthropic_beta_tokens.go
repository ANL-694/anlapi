package service

import "strings"

func anthropicBetaTokensContains(header, token string) bool {
	if header == "" || token == "" {
		return false
	}
	for _, part := range strings.Split(header, ",") {
		if strings.TrimSpace(part) == token {
			return true
		}
	}
	return false
}
