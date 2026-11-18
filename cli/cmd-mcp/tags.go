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

type tagArgs struct {
	Tag string `json:"tag" jsonschema:"the tag, without the #"`
}

type tagsArgs struct {
	Tag      string `json:"tag,omitempty" jsonschema:"the tag, without the #; the list of tags when empty"`
	Prefix   string `json:"prefix,omitempty" jsonschema:"without a tag, only the tags starting with this text, whatever the case"`
	MinNotes int    `json:"min_notes,omitempty" jsonschema:"without a tag, only the tags carried by at least that many notes"`
	Limit    *int   `json:"limit,omitempty" jsonschema:"how many notes of the tag at most, 50 when left out, 0 for all of them"`
	Offset   int    `json:"offset,omitempty" jsonschema:"how many notes of the tag to skip: the next page starts at offset + limit"`
	Dates    bool   `json:"dates,omitempty" jsonschema:"also the created_at and updated_at of each note of the tag, to judge whether it is still current"`
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
	Tag    string `json:"tag" jsonschema:"the tag to rename, without the #"`
	To     string `json:"to" jsonschema:"its new name, without the #"`
	Merge  bool   `json:"merge,omitempty" jsonschema:"when to is another tag already, fold the tag into it instead of refusing"`
	DryRun bool   `json:"dry_run,omitempty" jsonschema:"change nothing: answer with each note it would rewrite and the lines of its text that change (- old, + new); do it first"`
}

// renamedTag is the tag renamed; with dry_run, the notes it would rewrite, nothing being written.
type renamedTag struct {
	apiclient.DomainTag
	Notes []apiclient.EdittagChanged `json:"notes,omitempty"`
}

type tagNotesArgs struct {
	Notes         []string `json:"notes,omitempty" jsonschema:"the notes: their uuid, or their path from the root (/Folder/Note); or leave out for the notes the search filters find"`
	Text          string   `json:"text,omitempty" jsonschema:"instead of notes, every note whose title or content contains this text, as search_notes finds it"`
	MaxLength     *int     `json:"max_length,omitempty" jsonschema:"instead of notes, every note whose content, title line included, has at most that many characters"`
	Untagged      bool     `json:"untagged,omitempty" jsonschema:"instead of notes, every note with no tag"`
	UpdatedBefore string   `json:"updated_before,omitempty" jsonschema:"instead of notes, every note last changed before this day (2026-01-31) or RFC 3339 instant"`
	NotText       string   `json:"not_text,omitempty" jsonschema:"instead of notes, every note whose title and content do not hold this text, whatever its case"`
	TagsAll       []string `json:"tags_all,omitempty" jsonschema:"instead of notes, every note carrying every one of these tags, without the #"`
	TagsAny       []string `json:"tags_any,omitempty" jsonschema:"instead of notes, every note carrying at least one of these tags"`
	TagsNone      []string `json:"tags_none,omitempty" jsonschema:"instead of notes, every note carrying none of these tags"`
	Folder        string   `json:"folder,omitempty" jsonschema:"instead of notes, every note of this folder and its subfolders: its uuid, or its path from the root (/Folder)"`
	DryRun        bool     `json:"dry_run,omitempty" jsonschema:"change nothing: answer with each note it would change, what happens to the tags asked and the lines of its text that change (- old, + new); do it first"`
	Add           []string `json:"add,omitempty" jsonschema:"tags to add, without the #"`
	Remove        []string `json:"remove,omitempty" jsonschema:"tags to remove, without the #"`
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
		Description: "Add or remove tags on notes without sending their content: only their #tags change, the rest of the text stays as it is, and each note changed keeps its earlier text in note_history. Missing tags are written at the end of the note, on its last line of tags or a new one. A #tag is read anywhere but in code (a closed ``` block, lines indented 4 spaces or a tab after a blank line, `inline`), a link or an address. A locked note, or a tag that would not be read where it is written, leaves the note as it was, with the error. Instead of naming the notes, the filters of search_notes (text, not_text, tags_all, tags_any, tags_none, folder, max_length, untagged, updated_before) tag every note it would find, without reading them. With dry_run, nothing is written: the answer lists each note it would change, what happens to the tags asked and the lines of its text that change (- old, + new), as the real call would, so do it first on a bulk change.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args tagNotesArgs) (*mcp.CallToolResult, tagResult, error) {
		for _, tag := range args.Add {
			if !hashtag.Valid(tag) {
				return nil, tagResult{}, fmt.Errorf("%w: %q", hashtag.ErrInvalid, tag)
			}
		}
		if searched := args.Text != "" || args.NotText != "" || len(args.TagsAll)+len(args.TagsAny)+len(args.TagsNone) > 0 || args.Folder != "" ||
			args.MaxLength != nil || args.Untagged || args.UpdatedBefore != ""; searched {
			if len(args.Notes) > 0 {
				return nil, tagResult{}, errors.New("give either notes or the search filters, not both")
			}
			params, err := searchParams(ctx, client, searchArgs{Text: args.Text, NotText: args.NotText, TagsAll: args.TagsAll, TagsAny: args.TagsAny,
				TagsNone: args.TagsNone, Folder: args.Folder, MaxLength: args.MaxLength, Untagged: args.Untagged, UpdatedBefore: args.UpdatedBefore}, 0)
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
		Description: "List the notes carrying a tag, with their path: the first 50 and the total, unless limit says otherwise; dates adds when each was created and changed. Without a tag, the account's tags, most used first, prefix and min_notes keeping only some. Tags of a note are set by tag_notes.",
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
		Description: "Rename a tag on every note carrying it, in their text too, each note changed keeping its earlier text in note_history. A name taken by another tag refuses, unless merge folds the tag into it. With dry_run, nothing is written: the answer lists each note it would rewrite with the lines of its text that change (- old, + new): do it first.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args renameTagArgs) (*mcp.CallToolResult, renamedTag, error) {
		renamed, err := decode[renamedTag](client.PatchApiTagsValue(ctx, args.Tag, apiclient.EdittagBody{Value: args.To, Merge: &args.Merge, DryRun: &args.DryRun}))
		return nil, renamed, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "delete_tag",
		Annotations: writes(true, true),
		Description: "Remove a tag from every note carrying it, in their text too, each note changed keeping its earlier text in note_history. A note holding nothing but the tag goes to the trash.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args tagArgs) (*mcp.CallToolResult, []apiclient.DomainTag, error) {
		deleted, err := decode[[]apiclient.DomainTag](client.DeleteApiTagsValue(ctx, args.Tag))
		return nil, deleted, err
	})
}
