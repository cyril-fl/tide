package cmdmcp

import (
	"context"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type snippetArgs struct {
	Name    string `json:"name" jsonschema:"one of that name is replaced"`
	Content string `json:"content,omitempty" jsonschema:"the markdown it inserts"`
	Delete  bool   `json:"delete,omitempty" jsonschema:"remove the snippet of that name instead"`
}

// snippetList is the snippets in an object, as MCP wants of a tool's output.
type snippetList struct {
	Snippets []apiclient.DomainSnippet `json:"snippets"`
}

func addSnippetTools(server *mcp.Server, client *apiclient.ClientWithResponses, readOnly bool) {
	addTool(server, readOnly, &mcp.Tool{
		Name:        "snippets",
		Annotations: reads(),
		Description: "List the snippets: markdown the app inserts or starts a note from.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, snippetList, error) {
		snippets, err := apiclient.Snippets(ctx, client)
		return nil, snippetList{snippets}, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "save_snippet",
		Annotations: writes(true, true),
		Description: "Create, replace or delete a snippet by name, the others kept.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args snippetArgs) (*mcp.CallToolResult, apiclient.DomainSnippet, error) {
		snippet, err := apiclient.SaveSnippet(ctx, client, args.Name, args.Content, args.Delete)
		return nil, snippet, err
	})
}
