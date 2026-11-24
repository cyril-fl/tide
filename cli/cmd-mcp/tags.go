package cmdmcp

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/core/services/hashtag"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type deleteTagArgs struct {
	Tags      []string `json:"tags"`
	DryRun    bool     `json:"dry_run,omitempty"`
	KeepEmpty bool     `json:"keep_empty,omitempty"`
	Summary   bool     `json:"summary,omitempty" jsonschema:"counts only"`
}

// deletedTag counts the notes changed and trashed, then lists those trashed unless summary.
type deletedTag struct {
	Tags    []apiclient.DomainTag        `json:"tags"`
	Changed int                          `json:"changed"`
	Trashed int                          `json:"trashed"`
	Notes   []apiclient.DeletetagTrashed `json:"notes,omitempty"`
}

type tagsArgs struct {
	Tag      string `json:"tag,omitempty" jsonschema:"the account's tags when empty"`
	Prefix   string `json:"prefix,omitempty" jsonschema:"tags starting with it, any case"`
	MinNotes int    `json:"min_notes,omitempty" jsonschema:"tags of that many notes at least"`
	Limit    *int   `json:"limit,omitempty" jsonschema:"default 50, 0 for all"`
	Offset   int    `json:"offset,omitempty"`
	Dates    bool   `json:"dates,omitempty" jsonschema:"add created_at and updated_at"`
	Similar  bool   `json:"similar,omitempty" jsonschema:"groups of close tags: typo, case, accent, plural"`
}

// tags is the account's tags with their count, or one tag with a page of the notes carrying it and their total.
type tags struct {
	Tags    []apiclient.CounttagsTagCount    `json:"tags,omitempty"`
	Similar *[][]apiclient.CounttagsTagCount `json:"similar,omitempty"` // a pointer: no group is [], not left out
	Tag     string                           `json:"tag,omitempty"`
	Notes   []carrier                        `json:"notes,omitempty"`
	Total   int                              `json:"total,omitempty"`
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
	Tags    []string `json:"tags"`
	To      string   `json:"to"`
	Merge   bool     `json:"merge,omitempty" jsonschema:"fold it into to when to exists"`
	DryRun  bool     `json:"dry_run,omitempty" jsonschema:"write nothing, answer the diffs"`
	Summary bool     `json:"summary,omitempty" jsonschema:"count only"`
}

// renamedTag is the tag renamed; with dry_run, the count of the notes it would rewrite and, unless summary, those
// notes, nothing being written.
type renamedTag struct {
	apiclient.DomainTag
	Changed *int                       `json:"changed,omitempty"`
	Notes   []apiclient.EdittagChanged `json:"notes,omitempty"`
}

// renamedTags is every tag renamed, in the order asked.
type renamedTags struct {
	Tags []renamedTag `json:"tags"`
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
	EmptySections bool     `json:"empty_sections,omitempty"`
	DryRun        bool     `json:"dry_run,omitempty" jsonschema:"write nothing, answer the diffs"`
	Summary       bool     `json:"summary,omitempty" jsonschema:"counts and failed notes only"`
	Add           []string `json:"add,omitempty" jsonschema:"without the #"`
	Remove        []string `json:"remove,omitempty" jsonschema:"without the #"`
	Ops           []tagOp  `json:"ops,omitempty" jsonschema:"more add/remove on other notes, counted apart"`
}

type tagOp struct {
	Notes  []string `json:"notes"`
	Add    []string `json:"add,omitempty"`
	Remove []string `json:"remove,omitempty"`
}

// tagged is a note after tag_notes, its content left out: what happened to the tags asked, or why it was left as it was.
type tagged struct {
	ID             string   `json:"id,omitempty"`
	Title          string   `json:"title,omitempty"`
	Note           string   `json:"note,omitempty"` // as asked, only for a note that failed: it has no id then
	Op             *int     `json:"op,omitempty"`   // the index in ops of the operation, absent for the top level
	Added          []string `json:"added,omitempty"`
	AlreadyPresent []string `json:"already_present,omitempty"`
	Removed        []string `json:"removed,omitempty"`
	NotPresent     []string `json:"not_present,omitempty"`
	Diff           []string `json:"diff,omitempty"` // with dry_run: the lines of the text that change (- old, + new)
	Error          string   `json:"error,omitempty"`
}

// tagCount counts the notes changed, left as they were and failed.
type tagCount struct {
	Changed   int `json:"changed"`
	Unchanged int `json:"unchanged"`
	Failed    int `json:"failed"`
}

// tagResult counts every note, then those of each op, then gives the notes with their diff.
type tagResult struct {
	tagCount
	Ops   []tagCount `json:"ops,omitempty"`
	Notes []tagged   `json:"notes"`
}

// tagCounts counts the notes: failed ones, changed ones (a tag added or removed) and the rest, in all and by op.
func tagCounts(notes []tagged, ops int) tagResult {
	out := tagResult{Notes: notes, Ops: make([]tagCount, ops)}
	for _, n := range notes {
		counts := []*tagCount{&out.tagCount}
		if n.Op != nil && *n.Op < ops {
			counts = append(counts, &out.Ops[*n.Op])
		}
		for _, c := range counts {
			switch {
			case n.Error != "":
				c.Failed++
			case len(n.Added)+len(n.Removed) > 0:
				c.Changed++
			default:
				c.Unchanged++
			}
		}
	}
	return out
}

func addTagTools(server *mcp.Server, client *apiclient.ClientWithResponses, readOnly bool) {
	addTool(server, readOnly, &mcp.Tool{
		Name:        "tag_notes",
		Annotations: writes(false, true),
		Description: "Add or remove #tags in the text of notes given, found by search_notes filters or in ops; answers counts and diffs.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args tagNotesArgs) (*mcp.CallToolResult, tagResult, error) {
		adds := args.Add
		for _, op := range args.Ops {
			adds = append(adds[:len(adds):len(adds)], op.Add...)
		}
		for _, tag := range adds {
			if !hashtag.Valid(tag) {
				return nil, tagResult{}, fmt.Errorf("%w: %q", hashtag.ErrInvalid, tag)
			}
		}
		search := apiclient.SearchFilter{Text: args.Text, NotText: args.NotText, TagsAll: args.TagsAll, TagsAny: args.TagsAny,
			TagsNone: args.TagsNone, Folder: args.Folder, MaxLength: args.MaxLength, Untagged: args.Untagged, UpdatedBefore: args.UpdatedBefore,
			EmptySections: args.EmptySections}
		notes, err := apiclient.Targets(ctx, client, args.Notes, search)
		if err != nil {
			return nil, tagResult{}, err
		}
		args.Notes = notes
		if len(args.Notes) == 0 && len(args.Ops) == 0 {
			return nil, tagCounts([]tagged{}, 0), nil
		}
		ops := make([]apiclient.TagnotesOp, len(args.Ops))
		for i, op := range args.Ops {
			ops[i] = apiclient.TagnotesOp{Notes: op.Notes, Add: &op.Add, Remove: &op.Remove}
		}
		// The API's answer carries the diff of each note. Hundreds of notes take longer than Timeout.
		ctx, cancel := apiclient.Long(ctx)
		defer cancel()
		out, err := apiclient.Decode[[]tagged](client.PatchApiNotesTags(ctx, apiclient.TagnotesInput{Notes: args.Notes, Add: &args.Add, Remove: &args.Remove, Ops: &ops, DryRun: &args.DryRun}))
		for i := range out {
			if out[i].Error == "" {
				out[i].Note = "" // the id says which note
			}
		}
		result := tagCounts(out, len(args.Ops))
		if args.Summary {
			result.Notes = slices.DeleteFunc(result.Notes, func(n tagged) bool { return n.Error == "" })
		}
		return nil, result, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "tags",
		Annotations: reads(),
		Description: "Tags are #words in note text, case-insensitive. List a tag's notes and total; without one, all tags, most used first.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args tagsArgs) (*mcp.CallToolResult, tags, error) {
		if args.Tag == "" {
			counts, err := apiclient.Decode[[]apiclient.CounttagsTagCount](client.GetApiGraph(ctx))
			prefix := strings.ToLower(strings.TrimPrefix(args.Prefix, "#"))
			counts = slices.DeleteFunc(counts, func(count apiclient.CounttagsTagCount) bool {
				return count.Notes < args.MinNotes || !strings.HasPrefix(strings.ToLower(count.Value), prefix)
			})
			if args.Similar {
				similar := hashtag.Similar(counts, func(c apiclient.CounttagsTagCount) string { return c.Value })
				return nil, tags{Similar: &similar}, err
			}
			return nil, tags{Tags: counts}, err
		}
		limit, offset := limitOf(args.Limit), max(args.Offset, 0)
		result, total, err := apiclient.Paged[apiclient.ReadtagResult](client.GetApiTagsValue(ctx, args.Tag, &apiclient.GetApiTagsValueParams{Limit: &limit, Offset: &offset}))
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
		Description: "Rename tags, to one name, in the text of every note carrying them; a name taken refuses unless merge.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args renameTagArgs) (*mcp.CallToolResult, renamedTags, error) {
		ctx, cancel := apiclient.Long(ctx) // many notes take longer than Timeout, like delete_tag
		defer cancel()
		renamed, err := apiclient.Decode[[]renamedTag](client.PatchApiTags(ctx, apiclient.EdittagManyBody{Values: args.Tags, Value: args.To, Merge: &args.Merge, DryRun: &args.DryRun}))
		for i := range renamed {
			if args.DryRun {
				changed := len(renamed[i].Notes)
				renamed[i].Changed = &changed
			}
			if args.Summary {
				renamed[i].Notes = nil
			}
		}
		return nil, renamedTags{renamed}, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "delete_tag",
		Annotations: writes(true, true),
		Description: "Remove a tag from its notes' text, trashing those left empty unless keep_empty.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args deleteTagArgs) (*mcp.CallToolResult, deletedTag, error) {
		// Many notes take longer than Timeout, like tag_notes.
		ctx, cancel := apiclient.Long(ctx)
		defer cancel()
		deleted, err := apiclient.Decode[apiclient.DeletetagResult](client.DeleteApiTags(ctx, apiclient.DeletetagBody{Values: args.Tags, DryRun: &args.DryRun, KeepEmpty: &args.KeepEmpty}))
		out := deletedTag{Tags: deleted.Tags, Changed: deleted.Changed, Trashed: len(deleted.Trashed), Notes: deleted.Trashed}
		if args.Summary {
			out.Notes = nil
		}
		return nil, out, err
	})
}
