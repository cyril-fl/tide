package cmdmcp

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	errNoTag     = errors.New("no tag of that name")
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
	MaxLength     *int     `json:"max_length,omitempty" jsonschema:"at most that many characters, title line included"`
	Untagged      bool     `json:"untagged,omitempty"`
	UpdatedBefore string   `json:"updated_before,omitempty" jsonschema:"changed before this day or RFC 3339 instant, oldest first"`
	List          string   `json:"list,omitempty" jsonschema:"instead of a search: recent (the 20 changed last), duplicates (notes sharing a title) or same_path (sharing it in one folder)"`
	Limit         *int     `json:"limit,omitempty" jsonschema:"50 when left out, 0 for all"`
	Offset        int      `json:"offset,omitempty"`
}

// filtered tells whether the args hold a text or a filter.
func (args searchArgs) filtered() bool {
	return args.Text != "" || args.NotText != "" || len(args.TagsAll)+len(args.TagsAny)+len(args.TagsNone) > 0 || args.Folder != "" ||
		args.MaxLength != nil || args.Untagged || args.UpdatedBefore != ""
}

// searchParams turns the filters of search_notes into the API's query, the tags and the folder by their ids: the one
// path search_notes and tag_notes share.
func searchParams(ctx context.Context, client *apiclient.ClientWithResponses, args searchArgs, limit int) (*apiclient.GetApiSearchParams, error) {
	params := &apiclient.GetApiSearchParams{Q: &args.Text, MaxLength: args.MaxLength, Untagged: &args.Untagged, Limit: &limit, Offset: &args.Offset}
	if args.UpdatedBefore != "" {
		params.UpdatedBefore = &args.UpdatedBefore
	}
	if args.NotText != "" {
		params.NotText = &args.NotText
	}
	if args.Folder != "" {
		folder, err := apiclient.Resolve(ctx, client, apiclient.FOLDER, args.Folder)
		if err != nil {
			return nil, fmt.Errorf("folder %s: %w", args.Folder, err)
		}
		params.Folder = &folder
	}
	var err error
	params.TagsAll, params.TagsAny, params.TagsNone, err = tagIDs(ctx, client, args.TagsAll, args.TagsAny, args.TagsNone)
	return params, err
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
		Description: "Find the notes matching a text and all the filters given, with the line found and the total; with list, the recent notes or the groups of duplicate titles instead.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args searchArgs) (*mcp.CallToolResult, searchPage, error) {
		if args.List != "" && args.filtered() {
			return nil, searchPage{}, errListAlone
		}
		switch args.List {
		case "":
		case "recent":
			recent, err := decode[[]apiclient.ListrecentRecent](client.GetApiRecent(ctx))
			return nil, searchPage{Recent: recent, Total: len(recent)}, err
		case "duplicates", "same_path":
			samePath := args.List == "same_path"
			groups, err := decode[[]apiclient.ListduplicatesGroup](client.GetApiNotesDuplicates(ctx, &apiclient.GetApiNotesDuplicatesParams{SamePath: &samePath}))
			return nil, searchPage{Groups: groups, Total: len(groups)}, err
		default:
			return nil, searchPage{}, fmt.Errorf("%w: %q", errBadList, args.List)
		}
		params, err := searchParams(ctx, client, args, limitOf(args.Limit))
		if err != nil {
			return nil, searchPage{}, err
		}
		matches, total, err := paged[[]apiclient.SearchnotesMatch](client.GetApiSearch(ctx, params))
		return nil, searchPage{Matches: matches, Total: total}, err
	})
}

// tagIDs gives the ids of the tags named by each of the three lists, nil for an empty one: a name unknown to the
// account is an error.
func tagIDs(ctx context.Context, client *apiclient.ClientWithResponses, lists ...[]string) (*[]string, *[]string, *[]string, error) {
	out := make([]*[]string, 3)
	if len(slices.Concat(lists...)) == 0 {
		return nil, nil, nil, nil
	}
	all, err := decode[[]apiclient.DomainTag](client.GetApiTags(ctx))
	if err != nil {
		return nil, nil, nil, err
	}
	for l, names := range lists {
		if len(names) == 0 {
			continue
		}
		ids := make([]string, 0, len(names))
		for _, name := range names {
			name = strings.TrimPrefix(name, "#")
			i := slices.IndexFunc(all, func(tag apiclient.DomainTag) bool { return strings.EqualFold(tag.Value, name) })
			if i < 0 {
				return nil, nil, nil, fmt.Errorf("%w: %q", errNoTag, name)
			}
			ids = append(ids, all[i].Id)
		}
		out[l] = &ids
	}
	return out[0], out[1], out[2], nil
}
