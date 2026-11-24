package cmdmcp

import (
	"context"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type savedSearchesArgs struct {
	Search string `json:"search,omitempty" jsonschema:"its id or name; the list when empty"`
	Limit  *int   `json:"limit,omitempty" jsonschema:"default 50, 0 for all"`
	Offset int    `json:"offset,omitempty"`
}

type saveSearchArgs struct {
	Search      string   `json:"search,omitempty" jsonschema:"the one to replace or delete, by id or name; a new one when empty"`
	Delete      bool     `json:"delete,omitempty" jsonschema:"remove it instead, its notes kept"`
	Name        string   `json:"name,omitempty"`
	TagsAll     []string `json:"tags_all,omitempty"`
	TagsAny     []string `json:"tags_any,omitempty"`
	TagsNone    []string `json:"tags_none,omitempty"`
	Folders     []string `json:"folders,omitempty" jsonschema:"with their subfolders"`
	FolderMode  string   `json:"folder_mode,omitempty" jsonschema:"include (default) or exclude the folders' notes"`
	CreatedFrom string   `json:"created_from,omitempty" jsonschema:"a day or RFC 3339 instant, as the other dates"`
	CreatedTo   string   `json:"created_to,omitempty"`
	EditedFrom  string   `json:"edited_from,omitempty"`
	EditedTo    string   `json:"edited_to,omitempty"`
	Text        string   `json:"text,omitempty"`
	NotText     string   `json:"not_text,omitempty"`
}

// searches is the account's saved searches, or one run with a page of its notes and their total.
type searches struct {
	Searches []apiclient.SavedSearch         `json:"searches,omitempty"`
	Search   *apiclient.SavedSearch          `json:"search,omitempty"`
	Notes    []apiclient.RunsavedsearchMatch `json:"notes,omitempty"`
	Total    int                             `json:"total,omitempty"`
}

func addSavedSearchTools(server *mcp.Server, client *apiclient.ClientWithResponses, readOnly bool) {
	addTool(server, readOnly, &mcp.Tool{
		Name:        "saved_searches",
		Annotations: reads(),
		Description: "List the saved searches (the app's smart folders), or run one: its notes and total; Pro plan.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args savedSearchesArgs) (*mcp.CallToolResult, searches, error) {
		if args.Search == "" {
			list, err := apiclient.SavedSearches(ctx, client)
			return nil, searches{Searches: list}, err
		}
		search, notes, total, err := apiclient.RunSearch(ctx, client, args.Search, limitOf(args.Limit), max(args.Offset, 0))
		if err != nil {
			return nil, searches{}, err
		}
		return nil, searches{Search: &search, Notes: notes, Total: total}, nil
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "save_search",
		Annotations: writes(true, false),
		Description: "Create, replace or delete a saved search (its notes match every criterion), the others kept; Pro plan.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args saveSearchArgs) (*mcp.CallToolResult, apiclient.SavedSearch, error) {
		var search *apiclient.SavedSearch // nil deletes
		if !args.Delete {
			search = &apiclient.SavedSearch{
				Name: args.Name, TagsAll: args.TagsAll, TagsAny: args.TagsAny, TagsNone: args.TagsNone, Folders: args.Folders,
				FolderMode: args.FolderMode, CreatedFrom: args.CreatedFrom, CreatedTo: args.CreatedTo, EditedFrom: args.EditedFrom,
				EditedTo: args.EditedTo, Text: args.Text, NotText: args.NotText,
			}
		}
		saved, err := apiclient.SaveSearch(ctx, client, args.Search, search)
		return nil, saved, err
	})
}
