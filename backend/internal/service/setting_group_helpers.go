package service

func isAdministratorPublicGroup(group *Group) bool {
	return group != nil && group.OwnerUserID == nil && NormalizeGroupScope(group.Scope) == GroupScopePublic
}
