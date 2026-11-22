package cmdmcp

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type snippetArgs struct {
	Name    string `json:"name" jsonschema:"one of that name is replaced"`
	Content string `json:"content,omitempty" jsonschema:"the markdown it inserts"`
	Delete  bool   `json:"delete,omitempty" jsonschema:"remove the snippet of that name instead"`
}

var errNoSnippet = errors.New("no snippet of that name")

func addSnippetTools(server *mcp.Server, client *apiclient.ClientWithResponses, readOnly bool) {
	addTool(server, readOnly, &mcp.Tool{
		Name:        "snippets",
		Annotations: reads(),
		Description: "List the snippets: markdown the app inserts or starts a note from.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.DomainSnippet, error) {
		snippets, err := readSnippets(ctx, client)
		return nil, snippets, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "save_snippet",
		Annotations: writes(true, true),
		Description: "Create, replace or delete a snippet by name, the others kept.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args snippetArgs) (*mcp.CallToolResult, apiclient.DomainSnippet, error) {
		answer := apiclient.DomainSnippet{Name: args.Name, Content: args.Content}
		_, err := editMe(ctx, client, func(me apiclient.DomainUser) (apiclient.EdituserInput, error) {
			snippets := slices.Clone(me.Snippets)
			i := slices.IndexFunc(snippets, func(s apiclient.DomainSnippet) bool { return s.Name == args.Name })
			switch {
			case args.Delete && i < 0:
				return apiclient.EdituserInput{}, fmt.Errorf("%w: %q", errNoSnippet, args.Name)
			case args.Delete:
				answer = snippets[i]
				snippets = slices.Delete(snippets, i, i+1)
			case i >= 0:
				snippets[i] = answer
			default:
				snippets = append(snippets, answer)
			}
			return apiclient.EdituserInput{Snippets: &snippets}, nil
		})
		return nil, answer, err
	})
}

// readSnippets reads the account's snippets, [] when it has none.
func readSnippets(ctx context.Context, client *apiclient.ClientWithResponses) ([]apiclient.DomainSnippet, error) {
	me, err := decode[apiclient.DomainUser](client.GetApiMe(ctx))
	return append([]apiclient.DomainSnippet{}, me.Snippets...), err
}
