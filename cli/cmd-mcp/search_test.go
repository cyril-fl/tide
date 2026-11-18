package cmdmcp

import "testing"

func TestDuplicateNotes(t *testing.T) {
	agent := connect(t)
	call(t, agent, "create_folder", map[string]any{"title": "Work"}, nil)
	call(t, agent, "create_note", map[string]any{"content": "# Pizza\none"}, nil)
	call(t, agent, "create_note", map[string]any{"content": "# Pizza\ntwo"}, nil)
	call(t, agent, "create_note", map[string]any{"content": "# pizza\nthree", "folder": "/Work"}, nil)
	call(t, agent, "create_note", map[string]any{"content": "# Alone"}, nil)

	var page duplicatesPage
	call(t, agent, "duplicate_notes", map[string]any{}, &page)
	if len(page.Groups) != 1 || len(page.Groups[0].Notes) != 3 || page.Groups[0].Notes[0].Length == 0 {
		t.Errorf("duplicate_notes = %+v, want one group of 3 Pizza", page)
	}
	call(t, agent, "duplicate_notes", map[string]any{"same_path": true}, &page)
	if len(page.Groups) != 1 || page.Groups[0].Key != "/Pizza" || len(page.Groups[0].Notes) != 2 {
		t.Errorf("duplicate_notes same_path = %+v, want the 2 notes of /Pizza", page)
	}
}
