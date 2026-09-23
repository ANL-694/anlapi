package dto

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestGroupFromServiceIncludesScopeAndOwner(t *testing.T) {
	ownerID := int64(91)
	group := &service.Group{
		ID:          12,
		Name:        "private-u91-openai",
		Platform:    service.PlatformOpenAI,
		Status:      service.StatusActive,
		Scope:       service.GroupScopeUserPrivate,
		OwnerUserID: &ownerID,
	}

	admin := GroupFromServiceAdmin(group)
	regular := GroupFromService(group)
	for name, got := range map[string]*Group{
		"admin":   &admin.Group,
		"regular": regular,
	} {
		if got.Scope != service.GroupScopeUserPrivate {
			t.Errorf("%s: scope = %q, want %q", name, got.Scope, service.GroupScopeUserPrivate)
		}
		if got.OwnerUserID == nil || *got.OwnerUserID != ownerID {
			t.Errorf("%s: owner_user_id = %v, want %d", name, got.OwnerUserID, ownerID)
		}
	}
}
