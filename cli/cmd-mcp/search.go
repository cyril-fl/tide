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

var errNoTag = errors.New("no tag of that name")

type searchArgs struct {
	Text          string   `json:"text,omitempty" jsonschema:"the text to find in the title or the content of the notes; may be left out with another filter"`
	NotText       string   `json:"not_text,omitempty" jsonschema:"only the notes whose title and content do not hold this text, whatever its case: the recipes without Étapes"`
	TagsAll       []string `json:"tags_all,omitempty" jsonschema:"only the notes carrying every one of these tags, without the #"`
	TagsAny       []string `json:"tags_any,omitempty" jsonschema:"only the notes carrying at least one of these tags: Idée AND (Cuisine OR Boisson) is tags_all [Idée] and tags_any [Cuisine, Boisson]"`
	TagsNone      []string `json:"tags_none,omitempty" jsonschema:"only the notes carrying none of these tags"`
	Folder        string   `json:"folder,omitempty" jsonschema:"only the notes of this folder and its subfolders: its uuid, or its path from the root (/Folder)"`
	MaxLength     *int     `json:"max_length,omitempty" jsonschema:"only the notes whose content, title line included, has at most that many characters (unicode characters, not bytes): the empty or title-only ones"`
	Untagged      bool     `json:"untagged,omitempty" jsonschema:"only the notes with no tag"`
	UpdatedBefore string   `json:"updated_before,omitempty" jsonschema:"only the notes last changed before this day (2026-01-31) or RFC 3339 instant: the stale ones, the oldest first when there is no text"`
	Limit         *int     `json:"limit,omitempty" jsonschema:"how many notes at most, 50 when left out, 0 for all of them"`
	Offset        int      `json:"offset,omitempty" jsonschema:"how many matches to skip: the next page starts at offset + limit"`
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

// searchPage is a page of matches and their total: those read, and the notes after them that may match.
type searchPage struct {
	Matches []apiclient.SearchnotesMatch `json:"matches"`
	Total   int                          `json:"total"`
}

type duplicatesArgs struct {
	SamePath bool `json:"same_path,omitempty" jsonschema:"only the notes of the same folder with the same title, which a path cannot tell apart; any folder when left out"`
}

type duplicatesPage struct {
	Groups []apiclient.ListduplicatesGroup `json:"groups"`
}

func addSearchTools(server *mcp.Server, client *apiclient.ClientWithResponses, readOnly bool) {
	addTool(server, readOnly, &mcp.Tool{
		Name:        "search_notes",
		Annotations: reads(),
		Description: "Find the notes whose title or content contains a text, with the line where it was found, and their total: the first 50, title matches first, or limit and offset. The filters, with or without a text, all hold at once: tags_all, tags_any and tags_none by tag name (Idée AND (Cuisine OR Boisson) is tags_all [Idée] and tags_any [Cuisine, Boisson]), folder with its subfolders, not_text a text the notes lack, max_length, untagged and updated_before the short, untagged or stale notes, so that sorting the account reads only those.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args searchArgs) (*mcp.CallToolResult, searchPage, error) {
		params, err := searchParams(ctx, client, args, limitOf(args.Limit))
		if err != nil {
			return nil, searchPage{}, err
		}
		matches, total, err := paged[[]apiclient.SearchnotesMatch](client.GetApiSearch(ctx, params))
		return nil, searchPage{Matches: append([]apiclient.SearchnotesMatch{}, matches...), Total: total}, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "recent_notes",
		Annotations: reads(),
		Description: "List the notes changed most recently, newest first.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.ListrecentRecent, error) {
		recent, err := decode[[]apiclient.ListrecentRecent](client.GetApiRecent(ctx))
		return nil, recent, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "duplicate_notes",
		Annotations: reads(),
		Description: "List the notes sharing a title, so that no edit lands on the wrong one and duplicates can be merged: groups of {key, notes: [{id, path, updated_at, length}]}, the title as key (any case, in any folder). same_path keeps only the notes of the same folder, whose path is ambiguous: a path names none of them, use their ids. Notes with a near-identical content under different titles are not found.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args duplicatesArgs) (*mcp.CallToolResult, duplicatesPage, error) {
		groups, err := decode[[]apiclient.ListduplicatesGroup](client.GetApiNotesDuplicates(ctx, &apiclient.GetApiNotesDuplicatesParams{SamePath: &args.SamePath}))
		return nil, duplicatesPage{Groups: append([]apiclient.ListduplicatesGroup{}, groups...)}, err
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
