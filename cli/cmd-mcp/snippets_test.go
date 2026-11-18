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

	// delete answers the snippet removed, its content with it.
	var removed apiclient.DomainSnippet
	call(t, agent, "save_snippet", map[string]any{"name": "Book", "delete": true}, &removed)
	call(t, agent, "snippets", map[string]any{}, &snippets)
	if removed != want[0] || !slices.Equal(snippets, want[1:]) {
		t.Errorf("save_snippet delete = %+v, then snippets = %+v, want %+v", removed, snippets, want[1:])
	}

	// An unknown name and a nameless snippet (refused by the API) are errors, the list left as it was.
	for _, args := range []map[string]any{{"name": "Book", "delete": true}, {"name": " ", "content": "x"}} {
		if result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: "save_snippet", Arguments: args}); err != nil || !result.IsError {
			t.Errorf("save_snippet %v = %+v, %v; want an error", args, result, err)
		}
	}
	call(t, agent, "snippets", map[string]any{}, &snippets)
	if !slices.Equal(snippets, want[1:]) {
		t.Errorf("snippets after the errors = %+v", snippets)
	}
}
