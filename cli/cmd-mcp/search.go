package cmdmcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	errListAlone = errors.New("list takes no text nor filter")
	errBadList   = errors.New("list is recent, duplicates or same_path")
)

type searchArgs struct {
	Text          string   `json:"text,omitempty" jsonschema:"in the title or content"`
	NotText       string   `json:"not_text,omitempty" jsonschema:"a text the title and content lack, any case"`
	TagsAll       []string `json:"tags_all,omitempty" jsonschema:"tags the notes all carry"`
	TagsAny       []string `json:"tags_any,omitempty" jsonschema:"tags the notes carry one of at least"`
	TagsNone      []string `json:"tags_none,omitempty" jsonschema:"tags the notes carry none of"`
	Folder        string   `json:"folder,omitempty" jsonschema:"a folder, its subfolders included"`
	MaxLength     *int     `json:"max_length,omitempty" jsonschema:"at most N characters, title line included"`
	Untagged      bool     `json:"untagged,omitempty"`
	UpdatedBefore string   `json:"updated_before,omitempty" jsonschema:"changed before this day or RFC 3339 instant, oldest first"`
	EmptySections bool     `json:"empty_sections,omitempty" jsonschema:"a heading with nothing before the next of its level or the end, listed"`
	List          string   `json:"list,omitempty" jsonschema:"instead of a search: recent (20 last changed), duplicates (same title), same_path (same title and folder)"`
	Limit         *int     `json:"limit,omitempty" jsonschema:"default 50, 0 for all"`
	Offset        int      `json:"offset,omitempty"`
}

// filter is the args without list and paging.
func (args searchArgs) filter() apiclient.SearchFilter {
	return apiclient.SearchFilter{Text: args.Text, NotText: args.NotText, TagsAll: args.TagsAll, TagsAny: args.TagsAny,
		TagsNone: args.TagsNone, Folder: args.Folder, MaxLength: args.MaxLength, Untagged: args.Untagged, UpdatedBefore: args.UpdatedBefore,
		EmptySections: args.EmptySections}
}

// searchPage is a page of matches and their total: those read, and the notes after them that may match. With list,
// the recent notes or the groups of duplicates instead, and how many.
type searchPage struct {
	Matches []apiclient.SearchnotesMatch    `json:"matches,omitempty"`
	Recent  []apiclient.ListrecentRecent    `json:"recent,omitempty"`
	Groups  []apiclient.ListduplicatesGroup `json:"groups,omitempty"`
	Total   int                             `json:"total"`
}

func addSearchTools(server *mcp.Server, client *apiclient.ClientWithResponses, readOnly bool) {
	addTool(server, readOnly, &mcp.Tool{
		Name:        "search_notes",
		Annotations: reads(),
		Description: "Find notes by text and filters, all combined (folder with tags), with the line found and total; or list recent or duplicate titles.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args searchArgs) (*mcp.CallToolResult, searchPage, error) {
		if args.List != "" && args.filter().Filtered() {
			return nil, searchPage{}, errListAlone
		}
		switch args.List {
		case "":
		case "recent":
			recent, err := apiclient.Decode[[]apiclient.ListrecentRecent](client.GetApiRecent(ctx))
			return nil, searchPage{Recent: recent, Total: len(recent)}, err
		case "duplicates", "same_path":
			groups, err := apiclient.Duplicates(ctx, client, args.List == "same_path")
			return nil, searchPage{Groups: groups, Total: len(groups)}, err
		default:
			return nil, searchPage{}, fmt.Errorf("%w: %q", errBadList, args.List)
		}
		params, err := apiclient.SearchParams(ctx, client, args.filter(), limitOf(args.Limit), args.Offset)
		if err != nil {
			return nil, searchPage{}, err
		}
		matches, total, err := apiclient.Paged[[]apiclient.SearchnotesMatch](client.GetApiSearch(ctx, params))
		return nil, searchPage{Matches: matches, Total: total}, err
	})
}
