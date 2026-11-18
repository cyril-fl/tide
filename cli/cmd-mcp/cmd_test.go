package cmdmcp

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestAddTools catches a schema the SDK cannot infer from the arguments: the tools would be
// announced broken, and only an agent calling them would find out.
func TestAddTools(t *testing.T) {
	agent := connect(t)
	result, err := agent.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
		if tool.InputSchema == nil || tool.Description == "" {
			t.Errorf("tool %q: schema %v, description %q", tool.Name, tool.InputSchema, tool.Description)
		}
		// The client asks before a destructive tool and runs a read-only one freely: a tool without hints is
		// destructive by default, and one marked wrong deletes without asking.
		a := tool.Annotations
		switch {
		case a == nil:
			t.Errorf("tool %q: no annotations", tool.Name)
		case strings.HasPrefix(tool.Name, "delete_") || tool.Name == "edit_note" || tool.Name == "replace_notes" || tool.Name == "save_snippet" || tool.Name == "save_search":
			if a.ReadOnlyHint || a.DestructiveHint == nil || !*a.DestructiveHint {
				t.Errorf("tool %q: %+v, want destructive", tool.Name, a)
			}
		case slices.Contains([]string{"search_notes", "read_note", "read_notes", "list_folder", "export_notes", "read_file", "snippets", "saved_searches"}, tool.Name):
			if !a.ReadOnlyHint {
				t.Errorf("tool %q: %+v, want read-only", tool.Name, a)
			}
		}
	}
	for _, want := range []string{"search_notes", "recent_notes", "duplicate_notes", "read_note", "read_notes", "list_folder", "create_note", "edit_note", "create_folder", "rename_folder", "move_item", "delete_note", "delete_folder", "list_trash", "restore_item", "tags", "tag_notes", "rename_tag", "replace_notes", "delete_tag", "note_history", "note_links", "read_file", "attach_file", "export_notes", "import_notes", "snippets", "save_snippet", "delete_snippet", "saved_searches", "save_search", "delete_search"} {
		if !slices.Contains(names, want) {
			t.Errorf("tools = %v, missing %q", names, want)
		}
	}
}

// TestInstructions catches instructions naming a tool or an argument the server does not announce, renamed or only
// in the writing tools: the agent would look for it in vain.
func TestInstructions(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		ctx := context.Background()
		client, err := apiclient.NewClientWithResponses("http://unused") // listing the tools calls no route
		if err != nil {
			t.Fatal(err)
		}
		server := mcp.NewServer(&mcp.Implementation{Name: "notes"}, &mcp.ServerOptions{Instructions: Instructions(readOnly)})
		AddTools(server, client, readOnly)
		serverTransport, clientTransport := mcp.NewInMemoryTransports()
		session, err := server.Connect(ctx, serverTransport, nil)
		if err != nil {
			t.Fatal(err)
		}
		agent, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, clientTransport, nil)
		if err != nil {
			t.Fatal(err)
		}
		result, err := agent.ListTools(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		known := map[string]bool{}
		for _, tool := range result.Tools {
			known[tool.Name] = true
			properties, _ := tool.InputSchema.(map[string]any)["properties"].(map[string]any) // none without arguments
			for name := range properties {
				known[name] = true
			}
		}
		instructions := agent.InitializeResult().Instructions
		for _, name := range regexp.MustCompile(`\b[a-z]+(?:_[a-z]+)+\b`).FindAllString(instructions, -1) {
			if !known[name] {
				t.Errorf("readOnly %v: the instructions name %q, which no tool announces", readOnly, name)
			}
		}
		agent.Close()
		session.Close()
	}
}
