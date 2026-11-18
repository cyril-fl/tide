package cmdmcp

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/cli/services/apiclient/apitest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connect serves the tools on the API of apitest to an MCP client in memory.
func connect(t *testing.T) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	apitest.Serve(t)
	client, err := apiclient.Read().Client()
	if err != nil {
		t.Fatal(err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "notes"}, nil)
	AddTools(server, client, false)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	session, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	agent, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { agent.Close() })
	return agent
}

// call runs a tool and decodes its structured answer into out, failing the test on an error.
func call(t *testing.T, agent *mcp.ClientSession, name string, args map[string]any, out any) *mcp.CallToolResult {
	t.Helper()
	result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s: %v", name, result.Content[0].(*mcp.TextContent).Text)
	}
	if out != nil {
		data, _ := json.Marshal(result.StructuredContent)
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("%s: %v: %s", name, err, data)
		}
	}
	return result
}

// readOne reads one note through read_notes, failing the test unless it answers that note alone.
func readOne(t *testing.T, agent *mcp.ClientSession, ref string) batchNote {
	t.Helper()
	var read batch
	call(t, agent, "read_notes", map[string]any{"notes": []string{ref}}, &read)
	if len(read.Notes) != 1 {
		t.Fatalf("read_notes %s = %+v", ref, read)
	}
	return read.Notes[0]
}

func TestPage(t *testing.T) {
	items := []int{0, 1, 2, 3, 4}
	tests := map[string]struct {
		limit, offset int
		want          []int
	}{
		"every item":   {0, 0, items},
		"first page":   {2, 0, []int{0, 1}},
		"last page":    {2, 4, []int{4}},
		"past the end": {2, 9, []int{}},
		"all after":    {0, 3, []int{3, 4}},
	}
	for name, test := range tests {
		if got := page(items, test.limit, test.offset); !slices.Equal(got, test.want) {
			t.Errorf("%s: %v, want %v", name, got, test.want)
		}
	}
}

// TestEditMeRetries reads the account again and edits over it when another write lands between its read and its save.
func TestEditMeRetries(t *testing.T) {
	ctx := context.Background()
	apitest.Serve(t)
	client, err := apiclient.Read().Client()
	if err != nil {
		t.Fatal(err)
	}

	runs := 0
	me, err := editMe(ctx, client, func(me apiclient.DomainUser) (apiclient.EdituserInput, error) {
		runs++
		if runs == 1 {
			// The app saves a snippet after the read: the first save must be refused, not overwrite it.
			app := []apiclient.DomainSnippet{{Name: "App"}}
			if _, err := decode[apiclient.DomainUser](client.PatchApiMe(ctx, apiclient.EdituserInput{Snippets: &app})); err != nil {
				t.Fatal(err)
			}
		}
		snippets := append(slices.Clone(me.Snippets), apiclient.DomainSnippet{Name: "Agent"})
		return apiclient.EdituserInput{Snippets: &snippets}, nil
	})
	want := []apiclient.DomainSnippet{{Name: "App"}, {Name: "Agent"}}
	if err != nil || runs != 2 || !slices.Equal(me.Snippets, want) {
		t.Errorf("editMe = %+v, %v after %d runs; want %+v after 2", me.Snippets, err, runs, want)
	}
}
