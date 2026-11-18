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
	Name    string `json:"name" jsonschema:"the snippet's name; one of that name already is replaced"`
	Content string `json:"content" jsonschema:"the markdown the snippet inserts; its #hashtags become tags of the note it starts"`
}

type snippetNameArgs struct {
	Name string `json:"name" jsonschema:"the name of the snippet to remove"`
}

var errNoSnippet = errors.New("no snippet of that name")

func addSnippetTools(server *mcp.Server, client *apiclient.ClientWithResponses, readOnly bool) {
	addTool(server, readOnly, &mcp.Tool{
		Name:        "snippets",
		Annotations: reads(),
		Description: "List the account's snippets, the pieces of markdown the app inserts while typing or starts a new note from: their name and content.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.DomainSnippet, error) {
		snippets, err := readSnippets(ctx, client)
		return nil, snippets, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "save_snippet",
		Annotations: writes(true, true),
		Description: "Create a snippet, or replace the one of the same name, the others kept. Answers only the snippet saved.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args snippetArgs) (*mcp.CallToolResult, apiclient.DomainSnippet, error) {
		saved := apiclient.DomainSnippet{Name: args.Name, Content: args.Content}
		_, err := editMe(ctx, client, func(me apiclient.DomainUser) (apiclient.EdituserInput, error) {
			snippets := slices.Clone(me.Snippets)
			if i := slices.IndexFunc(snippets, func(s apiclient.DomainSnippet) bool { return s.Name == args.Name }); i >= 0 {
				snippets[i] = saved
			} else {
				snippets = append(snippets, saved)
			}
			return apiclient.EdituserInput{Snippets: &snippets}, nil
		})
		return nil, saved, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "delete_snippet",
		Annotations: writes(true, true),
		Description: "Remove a snippet by its name, the others kept. Answers the account's snippets.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args snippetNameArgs) (*mcp.CallToolResult, []apiclient.DomainSnippet, error) {
		me, err := editMe(ctx, client, func(me apiclient.DomainUser) (apiclient.EdituserInput, error) {
			kept := slices.DeleteFunc(slices.Clone(me.Snippets), func(s apiclient.DomainSnippet) bool { return s.Name == args.Name })
			if len(kept) == len(me.Snippets) {
				return apiclient.EdituserInput{}, fmt.Errorf("%w: %q", errNoSnippet, args.Name)
			}
			return apiclient.EdituserInput{Snippets: &kept}, nil
		})
		if err != nil {
			return nil, nil, err
		}
		return nil, append([]apiclient.DomainSnippet{}, me.Snippets...), nil
	})
}

// readSnippets reads the account's snippets, [] when it has none.
func readSnippets(ctx context.Context, client *apiclient.ClientWithResponses) ([]apiclient.DomainSnippet, error) {
	me, err := decode[apiclient.DomainUser](client.GetApiMe(ctx))
	return append([]apiclient.DomainSnippet{}, me.Snippets...), err
}
