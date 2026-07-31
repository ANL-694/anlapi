package service

import "testing"

func TestIsOpenAIOAuthServableModelRejectsKimiCodeBareIDs(t *testing.T) {
	for _, model := range []string{"k3", "k3-256k", "provider/k3", "provider/k3-256k"} {
		if isOpenAIOAuthServableModel(model) {
			t.Fatalf("isOpenAIOAuthServableModel(%q) = true", model)
		}
	}
	for _, model := range []string{"k3-custom", "provider/k3-custom"} {
		if !isOpenAIOAuthServableModel(model) {
			t.Fatalf("isOpenAIOAuthServableModel(%q) = false", model)
		}
	}
}
