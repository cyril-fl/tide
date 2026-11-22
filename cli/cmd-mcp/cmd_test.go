package cmdmcp

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"testing"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The ceiling of the tool list, 24 tools of 12.4 KB at #225: a tool or a long description more is paid at every
// request. Raise it knowingly.
const maxTools, maxToolBytes = 24, 13500

// TestAddTools catches a schema the SDK cannot infer from the arguments: the tools would be
// announced broken, and only an agent calling them would find out. It names every tool: one more is a choice.
func TestAddTools(t *testing.T) {
	tools, _ := announced(t, false)
	var names []string
	for _, tool := range tools {
		names = append(names, tool.Name)
		if tool.InputSchema == nil || tool.Description == "" {
			t.Errorf("tool %q: schema %v, description %q", tool.Name, tool.InputSchema, tool.Description)
		}
		// MCP wants an object: a client on the Python SDK (Vibe) rejects the whole tool list otherwise.
		if schema, ok := tool.OutputSchema.(map[string]any); tool.OutputSchema != nil && (!ok || schema["type"] != "object") {
			t.Errorf("tool %q: output schema %v, want an object", tool.Name, tool.OutputSchema)
		}
		// The client asks before a destructive tool and runs a read-only one freely: a tool without hints is
		// destructive by default, and one marked wrong deletes without asking.
		a := tool.Annotations
		switch {
		case a == nil:
			t.Errorf("tool %q: no annotations", tool.Name)
		case slices.Contains([]string{"delete_item", "delete_tag", "edit_note", "replace_notes", "save_snippet", "save_search"}, tool.Name):
			if a.ReadOnlyHint || a.DestructiveHint == nil || !*a.DestructiveHint {
				t.Errorf("tool %q: %+v, want destructive", tool.Name, a)
			}
		case slices.Contains([]string{"search_notes", "read_notes", "list_folder", "tags", "note_history", "note_links", "export_notes", "read_file", "snippets", "saved_searches"}, tool.Name):
			if !a.ReadOnlyHint {
				t.Errorf("tool %q: %+v, want read-only", tool.Name, a)
			}
		default:
			if a.ReadOnlyHint || a.DestructiveHint == nil || *a.DestructiveHint {
				t.Errorf("tool %q: %+v, want a write that is not destructive", tool.Name, a)
			}
		}
	}
	want := []string{"search_notes", "read_notes", "list_folder", "create_item", "edit_note", "rename_folder", "move_item", "delete_item", "restore_item",
		"tags", "tag_notes", "rename_tag", "delete_tag", "replace_notes", "note_history", "note_links", "read_file", "attach_file", "export_notes",
		"import_notes", "snippets", "save_snippet", "saved_searches", "save_search"}
	if slices.Sort(names); !slices.Equal(names, slices.Sorted(slices.Values(want))) {
		t.Errorf("tools = %v, want %v", names, want)
	}
}

// announced lists the tools and the instructions as a client gets them at the connection; listing calls no route.
func announced(t *testing.T, readOnly bool) ([]*mcp.Tool, string) {
	t.Helper()
	ctx := context.Background()
	client, err := apiclient.NewClientWithResponses("http://unused")
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
	defer session.Close()
	agent, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer agent.Close()
	result, err := agent.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	return result.Tools, agent.InitializeResult().Instructions
}

// TestInstructions catches instructions naming a tool or an argument the server does not announce, renamed or only
// in the writing tools: the agent would look for it in vain.
func TestInstructions(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		tools, instructions := announced(t, readOnly)
		known := map[string]bool{}
		for _, tool := range tools {
			known[tool.Name] = true
			properties, _ := tool.InputSchema.(map[string]any)["properties"].(map[string]any) // none without arguments
			for name := range properties {
				known[name] = true
			}
		}
		for _, name := range regexp.MustCompile(`\b[a-z]+(?:_[a-z]+)+\b`).FindAllString(instructions, -1) {
			if !known[name] {
				t.Errorf("readOnly %v: the instructions name %q, which no tool announces", readOnly, name)
			}
		}
	}
}

// TestToolListSize measures what the tool list costs the model at every request: the name, description and input
// schema of each tool (clients do not give it the output schemas), and the instructions. go test -v -run Size prints it.
func TestToolListSize(t *testing.T) {
	tools, instructions := announced(t, false)
	size := 0
	for _, tool := range tools {
		data, err := json.Marshal(map[string]any{"name": tool.Name, "description": tool.Description, "input_schema": tool.InputSchema})
		if err != nil {
			t.Fatal(err)
		}
		size += len(data)
	}
	t.Logf("%d tools, %d bytes; instructions %d bytes", len(tools), size, len(instructions))
	if len(tools) > maxTools || size > maxToolBytes {
		t.Errorf("%d tools of %d bytes, over %d tools or %d bytes: optimise what exists before adding", len(tools), size, maxTools, maxToolBytes)
	}
}
