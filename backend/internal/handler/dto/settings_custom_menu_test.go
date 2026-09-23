package dto

import (
	"encoding/json"
	"testing"
)

func TestCustomMenuItemPreservesHideOpenButton(t *testing.T) {
	raw := `[{"id":"docs","label":"Docs","icon_svg":"","url":"https://docs.example.test","visibility":"user","sort_order":1,"hide_open_button":true}]`

	items := ParseCustomMenuItems(raw)
	if len(items) != 1 {
		t.Fatalf("parsed %d custom menu items, want 1", len(items))
	}
	if !items[0].HideOpenButton {
		t.Fatal("hide_open_button was dropped while parsing public settings")
	}

	encoded, err := json.Marshal(items[0])
	if err != nil {
		t.Fatalf("marshal custom menu item: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("decode marshaled custom menu item: %v", err)
	}
	if value, ok := got["hide_open_button"].(bool); !ok || !value {
		t.Fatalf("hide_open_button = %#v, want true", got["hide_open_button"])
	}
}
