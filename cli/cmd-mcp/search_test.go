package cmdmcp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestSearchLists gives the duplicate titles and the recent notes through search_notes list, alone.
func TestSearchLists(t *testing.T) {
	agent := connect(t)
	call(t, agent, "create_item", map[string]any{"title": "Work"}, nil)
	call(t, agent, "create_item", map[string]any{"content": "# Pizza\none"}, nil)
	call(t, agent, "create_item", map[string]any{"content": "# Pizza\ntwo"}, nil)
	call(t, agent, "create_item", map[string]any{"content": "# pizza\nthree", "folder": "/Work"}, nil)
	call(t, agent, "create_item", map[string]any{"content": "# Alone"}, nil)

	var page searchPage
	call(t, agent, "search_notes", map[string]any{"list": "duplicates"}, &page)
	if len(page.Groups) != 1 || len(page.Groups[0].Notes) != 3 || page.Groups[0].Notes[0].Length == 0 || len(page.Matches) != 0 {
		t.Errorf("search_notes list duplicates = %+v, want one group of 3 Pizza", page)
	}
	page = searchPage{}
	call(t, agent, "search_notes", map[string]any{"list": "same_path"}, &page)
	if len(page.Groups) != 1 || page.Groups[0].Key != "/Pizza" || len(page.Groups[0].Notes) != 2 {
		t.Errorf("search_notes list same_path = %+v, want the 2 notes of /Pizza", page)
	}
	page = searchPage{}
	call(t, agent, "search_notes", map[string]any{"list": "recent"}, &page)
	if len(page.Recent) != 4 || page.Total != 4 {
		t.Errorf("search_notes list recent = %+v, want the 4 notes", page)
	}

	// A list with a filter, or an unknown list, refuses instead of ignoring one of them.
	for _, args := range []map[string]any{{"list": "recent", "text": "pizza"}, {"list": "old"}} {
		if result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: "search_notes", Arguments: args}); err != nil || !result.IsError {
			t.Errorf("search_notes %v = %+v, %v; want an error", args, result, err)
		}
	}
}
