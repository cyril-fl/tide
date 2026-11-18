package cmdmcp

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestSavedSearches saves searches by tag name and folder path, runs them a page at a time, replaces and removes one,
// then refuses all of it once the account leaves the Pro plan.
func TestSavedSearches(t *testing.T) {
	agent := connect(t)
	call(t, agent, "create_folder", map[string]any{"title": "Work"}, nil)
	call(t, agent, "create_note", map[string]any{"content": "# Pasta\n#cuisine #italie", "folder": "/Work"}, nil)
	call(t, agent, "create_note", map[string]any{"content": "# Soup\n#cuisine"}, nil)
	call(t, agent, "create_note", map[string]any{"content": "# Rome\n#italie"}, nil)

	// search_notes filters by tag names, folder path and a text left out, all at once.
	for want, args := range map[string]map[string]any{
		"/Soup":       {"tags_all": []string{"cuisine"}, "tags_none": []string{"#Italie"}},
		"/Work/Pasta": {"tags_any": []string{"cuisine", "italie"}, "folder": "/Work"},
		"/Rome":       {"tags_any": []string{"italie"}, "not_text": "PASTA"},
	} {
		var page searchPage
		call(t, agent, "search_notes", args, &page)
		if len(page.Matches) != 1 || page.Matches[0].Path != want || page.Total != 1 {
			t.Errorf("search_notes %v = %+v, want %s", args, page, want)
		}
	}
	for _, args := range []map[string]any{{"tags_none": []string{"nope"}}, {"folder": "/Nowhere"}} {
		if result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: "search_notes", Arguments: args}); err != nil || !result.IsError {
			t.Errorf("search_notes %v = %+v, %v; want an error", args, result, err)
		}
	}

	var saved savedSearch
	call(t, agent, "save_search", map[string]any{"name": "Italian", "tags_all": []string{"cuisine", "#Italie"}}, &saved)
	if saved.ID == "" || !slices.Equal(saved.TagsAll, []string{"cuisine", "italie"}) || len(saved.TagsAny) != 0 {
		t.Errorf("save_search = %+v", saved)
	}
	var run searches
	call(t, agent, "saved_searches", map[string]any{"search": "italian"}, &run)
	if run.Total != 1 || len(run.Notes) != 1 || run.Notes[0].Path != "/Work/Pasta" || run.Search == nil || run.Search.ID != saved.ID {
		t.Errorf("saved_searches italian = %+v", run)
	}

	call(t, agent, "save_search", map[string]any{"name": "Elsewhere", "folders": []string{"/Work"}, "folder_mode": "exclude", "created_to": "2999-12-31"}, &saved)
	if !slices.Equal(saved.Folders, []string{"/Work"}) || saved.CreatedTo != "2999-12-31T23:59:59.999Z" {
		t.Errorf("save_search Elsewhere = %+v", saved)
	}
	call(t, agent, "saved_searches", map[string]any{"search": saved.ID}, &run)
	if run.Total != 2 {
		t.Errorf("saved_searches Elsewhere = %+v", run)
	}

	// Replacing keeps the id and drops the criteria left out; a page still counts every note.
	call(t, agent, "save_search", map[string]any{"search": "Elsewhere", "name": "Cooking", "tags_any": []string{"cuisine"}}, nil)
	var replaced searches // a fresh one: decoding into run would keep the fields the answer leaves out
	call(t, agent, "saved_searches", map[string]any{"search": saved.ID, "limit": 1}, &replaced)
	if replaced.Total != 2 || len(replaced.Notes) != 1 || replaced.Search.Name != "Cooking" || len(replaced.Search.Folders) != 0 || len(replaced.Search.TagsAll) != 0 {
		t.Errorf("saved_searches Cooking = %+v, %+v", replaced, replaced.Search)
	}

	// cuisine AND (italie OR …), without the tag none nor the text left out, stored and replayed.
	var precise savedSearch
	call(t, agent, "save_search", map[string]any{"name": "Precise", "tags_all": []string{"cuisine"}, "tags_any": []string{"italie"}, "tags_none": []string{"cuisine"}, "not_text": "x"}, &precise)
	if !slices.Equal(precise.TagsAll, []string{"cuisine"}) || !slices.Equal(precise.TagsAny, []string{"italie"}) || !slices.Equal(precise.TagsNone, []string{"cuisine"}) || precise.NotText != "x" {
		t.Errorf("save_search Precise = %+v", precise)
	}
	call(t, agent, "save_search", map[string]any{"search": "Precise", "name": "Precise", "tags_all": []string{"cuisine"}, "tags_any": []string{"italie"}, "not_text": "soup"}, nil)
	var replayed searches
	call(t, agent, "saved_searches", map[string]any{"search": "Precise"}, &replayed)
	if replayed.Total != 1 || len(replayed.Notes) != 1 || replayed.Notes[0].Path != "/Work/Pasta" {
		t.Errorf("saved_searches Precise = %+v", replayed)
	}
	call(t, agent, "delete_search", map[string]any{"search": "Precise"}, nil)

	var left []savedSearch
	call(t, agent, "delete_search", map[string]any{"search": "Italian"}, &left)
	var listed searches
	call(t, agent, "saved_searches", map[string]any{}, &listed)
	if len(left) != 1 || len(listed.Searches) != 1 || listed.Searches[0].ID != saved.ID {
		t.Errorf("delete_search = %+v, saved_searches = %+v", left, listed)
	}

	// An unknown tag, folder, search or a date that is none are errors.
	for name, args := range map[string]map[string]any{
		"save_search":    {"name": "x", "tags_any": []string{"nope"}},
		"saved_searches": {"search": "Italian"},
		"delete_search":  {"search": "Italian"},
	} {
		if result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args}); err != nil || !result.IsError {
			t.Errorf("%s %v = %+v, %v; want an error", name, args, result, err)
		}
	}
	for _, args := range []map[string]any{{"name": "x", "folders": []string{"/Nowhere"}}, {"name": "x", "edited_from": "yesterday"}} {
		if result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: "save_search", Arguments: args}); err != nil || !result.IsError {
			t.Errorf("save_search %v = %+v, %v; want an error", args, result, err)
		}
	}

	// Outside the Pro plan, every saved search tool says so.
	client, err := apiclient.Read().Client()
	if err != nil {
		t.Fatal(err)
	}
	base := domain.Plans[0]
	if _, err := decode[apiclient.DomainUser](client.PatchApiMe(context.Background(), apiclient.EdituserInput{Plan: &base})); err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string]map[string]any{"saved_searches": {}, "save_search": {"name": "x"}, "delete_search": {"search": "Cooking"}} {
		result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || !result.IsError || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "Pro plan") {
			t.Errorf("%s outside Pro = %+v, %v; want the Pro plan error", name, result, err)
		}
	}
}
