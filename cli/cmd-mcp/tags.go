package cmdmcp

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/core/services/hashtag"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var errNotesOrFilters = errors.New("give either notes or the search filters, not both")

type tagArgs struct {
	Tag string `json:"tag"`
}

type tagsArgs struct {
	Tag      string `json:"tag,omitempty" jsonschema:"the account's tags when empty"`
	Prefix   string `json:"prefix,omitempty" jsonschema:"only the tags starting with it, any case"`
	MinNotes int    `json:"min_notes,omitempty" jsonschema:"only the tags of that many notes at least"`
	Limit    *int   `json:"limit,omitempty" jsonschema:"notes of the tag, 50 when left out, 0 for all"`
	Offset   int    `json:"offset,omitempty"`
	Dates    bool   `json:"dates,omitempty" jsonschema:"add created_at and updated_at"`
}

// tags is the account's tags with their count, or one tag with a page of the notes carrying it and their total.
type tags struct {
	Tags  []apiclient.CounttagsTagCount `json:"tags,omitempty"`
	Tag   string                        `json:"tag,omitempty"`
	Notes []carrier                     `json:"notes,omitempty"`
	Total int                           `json:"total,omitempty"`
}

// carrier is a note carrying a tag, its dates left out unless asked.
type carrier struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Path      string `json:"path"`
	Readonly  bool   `json:"readonly,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type renameTagArgs struct {
	Tag    string `json:"tag"`
	To     string `json:"to"`
	Merge  bool   `json:"merge,omitempty" jsonschema:"fold it into to when to exists"`
	DryRun bool   `json:"dry_run,omitempty" jsonschema:"write nothing, answer each note's diff"`
}

// renamedTag is the tag renamed; with dry_run, the notes it would rewrite, nothing being written.
type renamedTag struct {
	apiclient.DomainTag
	Notes []apiclient.EdittagChanged `json:"notes,omitempty"`
}

type tagNotesArgs struct {
	Notes []string `json:"notes,omitempty" jsonschema:"or the filters of search_notes"`
	// The filters of search_notes, described there.
	Text          string   `json:"text,omitempty"`
	NotText       string   `json:"not_text,omitempty"`
	TagsAll       []string `json:"tags_all,omitempty"`
	TagsAny       []string `json:"tags_any,omitempty"`
	TagsNone      []string `json:"tags_none,omitempty"`
	Folder        string   `json:"folder,omitempty"`
	MaxLength     *int     `json:"max_length,omitempty"`
	Untagged      bool     `json:"untagged,omitempty"`
	UpdatedBefore string   `json:"updated_before,omitempty"`
	DryRun        bool     `json:"dry_run,omitempty" jsonschema:"write nothing, answer each note's diff"`
	Add           []string `json:"add,omitempty" jsonschema:"without the #"`
	Remove        []string `json:"remove,omitempty" jsonschema:"without the #"`
}

// tagged is a note after tag_notes, its content left out: what happened to the tags asked, or why it was left as it was.
type tagged struct {
	ID             string   `json:"id,omitempty"`
	Title          string   `json:"title,omitempty"`
	Note           string   `json:"note,omitempty"` // as asked, only for a note that failed: it has no id then
	Added          []string `json:"added,omitempty"`
	AlreadyPresent []string `json:"already_present,omitempty"`
	Removed        []string `json:"removed,omitempty"`
	NotPresent     []string `json:"not_present,omitempty"`
	Diff           []string `json:"diff,omitempty"` // with dry_run: the lines of the text that change (- old, + new)
	Error          string   `json:"error,omitempty"`
}

// tagResult counts the notes changed, left as they were and failed, then gives the notes with their diff.
type tagResult struct {
	Changed   int      `json:"changed"`
	Unchanged int      `json:"unchanged"`
	Failed    int      `json:"failed"`
	Notes     []tagged `json:"notes"`
}

// tagCounts counts the notes: failed ones, changed ones (a tag added or removed) and the rest.
func tagCounts(notes []tagged) tagResult {
	out := tagResult{Notes: notes}
	for _, n := range notes {
		switch {
		case n.Error != "":
			out.Failed++
		case len(n.Added)+len(n.Removed) > 0:
			out.Changed++
		default:
			out.Unchanged++
		}
	}
	return out
}

func addTagTools(server *mcp.Server, client *apiclient.ClientWithResponses, readOnly bool) {
	addTool(server, readOnly, &mcp.Tool{
		Name:        "tag_notes",
		Annotations: writes(false, true),
		Description: "Add or remove tags on the notes given or found by the filters of search_notes, only their #tags rewritten; answers counts and each note's diff.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args tagNotesArgs) (*mcp.CallToolResult, tagResult, error) {
		for _, tag := range args.Add {
			if !hashtag.Valid(tag) {
				return nil, tagResult{}, fmt.Errorf("%w: %q", hashtag.ErrInvalid, tag)
			}
		}
		search := searchArgs{Text: args.Text, NotText: args.NotText, TagsAll: args.TagsAll, TagsAny: args.TagsAny,
			TagsNone: args.TagsNone, Folder: args.Folder, MaxLength: args.MaxLength, Untagged: args.Untagged, UpdatedBefore: args.UpdatedBefore}
		if search.filtered() {
			if len(args.Notes) > 0 {
				return nil, tagResult{}, errNotesOrFilters
			}
			params, err := searchParams(ctx, client, search, 0)
			if err != nil {
				return nil, tagResult{}, err
			}
			matches, _, err := paged[[]apiclient.SearchnotesMatch](client.GetApiSearch(ctx, params))
			if err != nil {
				return nil, tagResult{}, err
			}
			for _, match := range matches {
				args.Notes = append(args.Notes, match.Id)
			}
		}
		if len(args.Notes) == 0 {
			return nil, tagCounts([]tagged{}), nil
		}
		// The API's answer carries the diff of each note. Hundreds of notes take longer than Timeout.
		ctx, cancel := apiclient.Long(ctx)
		defer cancel()
		out, err := decode[[]tagged](client.PatchApiNotesTags(ctx, apiclient.TagnotesInput{Notes: args.Notes, Add: &args.Add, Remove: &args.Remove, DryRun: &args.DryRun}))
		for i := range out {
			if out[i].Error == "" {
				out[i].Note = "" // the id says which note
			}
		}
		return nil, tagCounts(out), err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "tags",
		Annotations: reads(),
		Description: "List the notes carrying a tag, with the total; without a tag, the account's tags, most used first.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args tagsArgs) (*mcp.CallToolResult, tags, error) {
		if args.Tag == "" {
			counts, err := decode[[]apiclient.CounttagsTagCount](client.GetApiGraph(ctx))
			prefix := strings.ToLower(strings.TrimPrefix(args.Prefix, "#"))
			counts = slices.DeleteFunc(counts, func(count apiclient.CounttagsTagCount) bool {
				return count.Notes < args.MinNotes || !strings.HasPrefix(strings.ToLower(count.Value), prefix)
			})
			return nil, tags{Tags: counts}, err
		}
		limit, offset := limitOf(args.Limit), max(args.Offset, 0)
		result, total, err := paged[apiclient.ReadtagResult](client.GetApiTagsValue(ctx, args.Tag, &apiclient.GetApiTagsValueParams{Limit: &limit, Offset: &offset}))
		if err != nil {
			return nil, tags{}, err
		}
		notes := make([]carrier, 0, len(result.Notes))
		for _, found := range result.Notes {
			read := carrier{ID: found.Id, Title: found.Title, Path: found.Path, Readonly: apiclient.Deref(found.Readonly)}
			if args.Dates {
				read.CreatedAt, read.UpdatedAt = found.CreatedAt, editedAt(found.UpdatedAt, found.CreatedAt)
			}
			notes = append(notes, read)
		}
		return nil, tags{Tag: result.Tag.Value, Notes: notes, Total: total}, nil
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "rename_tag",
		Annotations: writes(false, true),
		Description: "Rename a tag in the text of every note carrying it; a name taken refuses unless merge.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args renameTagArgs) (*mcp.CallToolResult, renamedTag, error) {
		renamed, err := decode[renamedTag](client.PatchApiTagsValue(ctx, args.Tag, apiclient.EdittagBody{Value: args.To, Merge: &args.Merge, DryRun: &args.DryRun}))
		return nil, renamed, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "delete_tag",
		Annotations: writes(true, true),
		Description: "Remove a tag from the text of every note carrying it; a note holding nothing else goes to the trash.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args tagArgs) (*mcp.CallToolResult, []apiclient.DomainTag, error) {
		deleted, err := decode[[]apiclient.DomainTag](client.DeleteApiTagsValue(ctx, args.Tag))
		return nil, deleted, err
	})
}
