package service

import "testing"

func TestAccountShareDefaultsAndVisibility(t *testing.T) {
	owner := int64(7)
	account := &Account{OwnerUserID: &owner}

	if got := NormalizeAccountShareMode(""); got != AccountShareModePrivate {
		t.Fatalf("empty share mode = %q, want %q", got, AccountShareModePrivate)
	}
	if got := NormalizeAccountShareStatus(""); got != AccountShareStatusApproved {
		t.Fatalf("empty share status = %q, want %q", got, AccountShareStatusApproved)
	}
	if account.IsPublicShareApproved() {
		t.Fatal("private account must not be treated as public")
	}
	if !account.IsVisibleToConsumer(owner) {
		t.Fatal("owner must see own account")
	}
	if account.IsVisibleToConsumer(8) {
		t.Fatal("private account must not be visible to another user")
	}

	account.ShareMode = AccountShareModePublic
	account.ShareStatus = AccountShareStatusApproved
	if !account.IsPublicShareApproved() || !account.IsVisibleToConsumer(8) {
		t.Fatal("approved public account must be visible to another user")
	}

	account.ShareStatus = AccountShareStatusSuspended
	if account.IsPublicShareApproved() || account.IsVisibleToConsumer(8) {
		t.Fatal("suspended public account must not be visible to another user")
	}
}
