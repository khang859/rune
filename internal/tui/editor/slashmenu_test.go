package editor

import "testing"

func TestSlashMenu_FiltersOnQuery(t *testing.T) {
	cmds := []string{"/model", "/tree", "/resume", "/login"}
	m := NewSlashMenu(cmds)
	m.SetQuery("re")
	items := m.Items()
	found := map[string]bool{}
	for _, it := range items {
		found[it] = true
	}
	if !found["/resume"] || !found["/tree"] {
		t.Fatalf("expected /resume and /tree, got %v", items)
	}
}

func TestSlashMenu_ExactMatchFirst(t *testing.T) {
	m := NewSlashMenu([]string{"/model", "/modes"})
	m.SetQuery("model")
	if items := m.Items(); len(items) == 0 || items[0] != "/model" {
		t.Fatalf("/model not first: %v", items)
	}
}
