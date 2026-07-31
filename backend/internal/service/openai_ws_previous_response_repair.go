package service

import (
	"context"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func (s *OpenAIGatewayService) RepairOpenAIWSPreviousResponseIDForSession(ctx context.Context, groupID int64, sessionHash string, payload []byte, enabled bool) ([]byte, string, bool) {
	if s == nil || !enabled || groupID <= 0 || strings.TrimSpace(sessionHash) == "" || len(payload) == 0 {
		return payload, "", false
	}
	previousResponseID := strings.TrimSpace(gjson.GetBytes(payload, "previous_response_id").String())
	if previousResponseID == "" {
		return payload, "", false
	}
	store := s.getOpenAIWSStateStore()
	ownerSession, err := store.GetResponseSession(ctx, groupID, previousResponseID)
	if err != nil || strings.TrimSpace(ownerSession) == "" {
		return payload, previousResponseID, false
	}
	latestResponseID, err := store.GetSessionLatestResponse(ctx, groupID, ownerSession)
	latestResponseID = strings.TrimSpace(latestResponseID)
	if err != nil || latestResponseID == "" || latestResponseID == previousResponseID {
		return payload, previousResponseID, false
	}
	repairedPayload, err := sjson.SetBytes(payload, "previous_response_id", latestResponseID)
	if err != nil {
		return payload, previousResponseID, false
	}
	return repairedPayload, latestResponseID, true
}
