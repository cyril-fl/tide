package cmdmcp

import (
	"context"
	"slices"
	"testing"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestSnippets creates, replaces and removes snippets one at a time, the others kept, the server's checks surfacing.
func TestSnippets(t *testing.T) {
	agent := connect(t)

	var snippets []apiclient.DomainSnippet
	call(t, agent, "snippets", map[string]any{}, &snippets)
	if len(snippets) != 0 {
		t.Errorf("snippets of a new account = %+v", snippets)
	}
	call(t, agent, "save_snippet", map[string]any{"name": "Book", "content": "# Title\n#reading"}, nil)
	call(t, agent, "save_snippet", map[string]any{"name": "Trip", "content": "# Trip"}, nil)
	var saved apiclient.DomainSnippet
	call(t, agent, "save_snippet", map[string]any{"name": "Book", "content": "# Book"}, &saved)
	want := []apiclient.DomainSnippet{{Name: "Book", Content: "# Book"}, {Name: "Trip", Content: "# Trip"}}
	if saved != want[0] {
		t.Errorf("save_snippet = %+v, want only %+v", saved, want[0])
	}
	call(t, agent, "snippets", map[string]any{}, &snippets)
	if !slices.Equal(snippets, want) {
		t.Errorf("snippets = %+v, want %+v", snippets, want)
	}

	call(t, agent, "delete_snippet", map[string]any{"name": "Book"}, nil)
	call(t, agent, "snippets", map[string]any{}, &snippets)
	if !slices.Equal(snippets, want[1:]) {
		t.Errorf("snippets after delete_snippet = %+v, want %+v", snippets, want[1:])
	}

	// An unknown name and a nameless snippet (refused by the API) are errors, the list left as it was.
	for name, args := range map[string]map[string]any{"delete_snippet": {"name": "Book"}, "save_snippet": {"name": " ", "content": "x"}} {
		if result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args}); err != nil || !result.IsError {
			t.Errorf("%s %v = %+v, %v; want an error", name, args, result, err)
		}
	}
	call(t, agent, "snippets", map[string]any{}, &snippets)
	if !slices.Equal(snippets, want[1:]) {
		t.Errorf("snippets after the errors = %+v", snippets)
	}
}
