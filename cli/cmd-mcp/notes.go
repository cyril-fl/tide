package cmdmcp

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/core/services/hashtag"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	errEditMode = errors.New("give one of content, title, old or section")
	errEditTags = errors.New("tags go with content: tag_notes adds or removes them alone")
)

// note is what the read and the writes answer, the content left out when the agent did not ask for it.
type note struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Path      string   `json:"path"`
	Content   string   `json:"content,omitempty"`
	Length    *int     `json:"length,omitempty"`    // the whole content's characters, given by read_notes
	Truncated *bool    `json:"truncated,omitempty"` // a pointer: false is said too, by read_notes
	Tags      []string `json:"tags"`                // [] rather than left out: the agent sees the note has none
	Readonly  bool     `json:"readonly,omitempty"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at,omitempty"`
}

// batchNote is a note of read_notes: what all the notes share is left to batch.Common.
type batchNote struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Path      string   `json:"path,omitempty"` // left out when batch.Common.Folder holds it
	Name      string   `json:"name,omitempty"` // the last part of the path, when Common.Folder is given and it is not the title
	Content   string   `json:"content,omitempty"`
	Length    *int     `json:"length,omitempty"`
	Truncated *bool    `json:"truncated,omitempty"`
	Tags      []string `json:"tags"`
	Readonly  bool     `json:"readonly,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
}

// batchCommon is what every note of a batch of several has alike, said once.
type batchCommon struct {
	Folder    string `json:"folder,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// batch is the notes of read_notes, with the fields they share factored.
type batch struct {
	Common batchCommon `json:"common"`
	Notes  []batchNote `json:"notes"`
}

// factor builds the batch: a field every note of several gives alike moves to Common, the notes keep the others.
func factor(read []note) batch {
	out := batch{Notes: make([]batchNote, 0, len(read))}
	for _, n := range read {
		out.Notes = append(out.Notes, batchNote{ID: n.ID, Title: n.Title, Path: n.Path, Content: n.Content, Length: n.Length, Truncated: n.Truncated,
			Tags: n.Tags, Readonly: n.Readonly, CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt})
	}
	if len(read) < 2 {
		return out
	}
	folder := func(path string) string { return path[:max(strings.LastIndex(path, "/"), 0)] + "/" }
	same := func(of func(note) string) bool {
		return !slices.ContainsFunc(read, func(n note) bool { return of(n) != of(read[0]) })
	}
	if same(func(n note) string { return folder(n.Path) }) {
		out.Common.Folder = folder(read[0].Path)
		for i, n := range read {
			out.Notes[i].Path = ""
			if name := n.Path[strings.LastIndex(n.Path, "/")+1:]; name != n.Title {
				out.Notes[i].Name = name
			}
		}
	}
	if same(func(n note) string { return n.CreatedAt }) {
		out.Common.CreatedAt = read[0].CreatedAt
		for i := range out.Notes {
			out.Notes[i].CreatedAt = ""
		}
	}
	if same(func(n note) string { return n.UpdatedAt }) {
		out.Common.UpdatedAt = read[0].UpdatedAt
		for i := range out.Notes {
			out.Notes[i].UpdatedAt = ""
		}
	}
	return out
}

type notesArgs struct {
	Notes    []string `json:"notes" jsonschema:"50 at most"`
	MaxChars int      `json:"max_chars,omitempty" jsonschema:"only the start of each, cut at a line or word end; truncated says so"`
}

type editArgs struct {
	Note    string   `json:"note"`
	Content string   `json:"content,omitempty" jsonschema:"the new markdown, replacing all of it"`
	Tags    []string `json:"tags,omitempty" jsonschema:"tags added to content"`
	Title   string   `json:"title,omitempty" jsonschema:"its new title"`
	Old     string   `json:"old,omitempty" jsonschema:"an exact text, found once unless all"`
	New     string   `json:"new,omitempty" jsonschema:"replaces old or the section's body; empty deletes it"`
	All     bool     `json:"all,omitempty" jsonschema:"replace every occurrence of old"`
	Section string   `json:"section,omitempty" jsonschema:"a heading whose body becomes new"`
}

// partEdit is a note changed, answered without its content: the agent knows what it sent. Replaced counts the
// occurrences of old or the section changed.
type partEdit struct {
	note
	Replaced int `json:"replaced,omitempty"`
}

type replaceArgs struct {
	Find    string `json:"find" jsonschema:"plain text, case kept"`
	Replace string `json:"replace" jsonschema:"empty to delete it"`
	Folder  string `json:"folder,omitempty" jsonschema:"with its subfolders; every note when empty"`
	DryRun  bool   `json:"dry_run,omitempty" jsonschema:"only count"`
}

type historyArgs struct {
	Note    string `json:"note"`
	Version string `json:"version,omitempty" jsonschema:"a version's uuid; the list when empty"`
}

// history is the list of versions, their texts left out, or the one version asked with its text.
type history struct {
	Versions []apiclient.DomainNoteVersion `json:"versions,omitempty"`
	Version  *apiclient.DomainNoteVersion  `json:"version,omitempty"`
}

type linksArgs struct {
	Note string `json:"note,omitempty" jsonschema:"every broken link when empty"`
}

// links joins a note to the others, the note itself left out (read_notes reads it), or lists the broken links of every note.
type links struct {
	Links     []apiclient.ReadneighborsLinked   `json:"links,omitempty"`
	Backlinks []apiclient.ReadneighborsLinked   `json:"backlinks,omitempty"`
	Neighbors []apiclient.ReadneighborsNeighbor `json:"neighbors,omitempty"`
	Broken    []apiclient.ListbrokenlinksLink   `json:"broken,omitempty"`
}

func addNoteTools(server *mcp.Server, client *apiclient.ClientWithResponses, readOnly bool) {
	addTool(server, readOnly, &mcp.Tool{
		Name:        "read_notes",
		Annotations: reads(),
		Description: "Read notes in the order asked, with tags and length, what they share said once.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args notesArgs) (*mcp.CallToolResult, batch, error) {
		// One request for the batch, the preview cut on the server: the agent may be far over Tailscale (#210).
		found, err := decode[[]apiclient.ReadnotesNote](client.PostApiNotesRead(ctx, apiclient.ReadnotesBody{Notes: args.Notes, MaxChars: &args.MaxChars}))
		if err != nil {
			return nil, batch{}, err
		}
		read := make([]note, 0, len(found))
		for _, f := range found {
			n, _ := readNote(apiclient.ReaditemResult{Item: f.Item, Path: f.Path, Content: f.Content, Readonly: f.Readonly, Tags: f.Tags}, nil)
			n.Length, n.Truncated = &f.Length, &f.Truncated
			read = append(read, n)
		}
		return nil, factor(read), nil
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "edit_note",
		Annotations: writes(true, false),
		Description: "Change a note's whole content, a passage (old or section) or its title; answers it without content.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args editArgs) (*mcp.CallToolResult, partEdit, error) {
		if args.Content == "" {
			if args.Title+args.Old+args.Section == "" {
				return nil, partEdit{}, errEditMode
			}
			if len(args.Tags) > 0 {
				return nil, partEdit{}, errEditTags
			}
			// The server refuses more than one of title, old and section.
			given := func(value string) *string {
				if value == "" {
					return nil
				}
				return &value
			}
			input := apiclient.EditnotetextInput{Title: given(args.Title), Old: given(args.Old), Section: given(args.Section), New: given(args.New), All: &args.All}
			edited, err := editText(ctx, client, args.Note, input)
			return nil, edited, err
		}
		if args.Title+args.Old+args.Section != "" {
			return nil, partEdit{}, errEditMode
		}
		id, err := apiclient.Resolve(ctx, client, apiclient.NOTE, args.Note)
		if err != nil {
			return nil, partEdit{}, err
		}
		input := apiclient.EditnoteBody{Content: &args.Content}
		// PATCH makes the tags exactly those given, the content's #tags kept with them: tags add, as in create_item.
		if len(args.Tags) > 0 {
			tags := append(hashtag.Parse(args.Content), args.Tags...)
			input.Tags = &tags
		}
		edited, err := readNote(decode[apiclient.ReaditemResult](client.PatchApiNotesId(ctx, id, input)))
		edited.Content = "" // the agent has just sent it
		return nil, partEdit{note: edited}, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "replace_notes",
		Annotations: writes(true, false),
		Description: "Replace a text in every note, or a folder's, in one transaction; Ultra plan.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args replaceArgs) (*mcp.CallToolResult, apiclient.ReplacenotesResult, error) {
		body := apiclient.ReplacenotesInput{Find: args.Find, Replace: args.Replace, DryRun: &args.DryRun}
		if args.Folder != "" {
			id, err := apiclient.Resolve(ctx, client, apiclient.FOLDER, args.Folder)
			if err != nil {
				return nil, apiclient.ReplacenotesResult{}, err
			}
			body.Folder = &id
		}
		result, err := decode[apiclient.ReplacenotesResult](client.PostApiReplace(ctx, body))
		result.Matches = nil // the excerpts are for the app's list: the counts are enough here
		return nil, result, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "note_history",
		Annotations: reads(),
		Description: "List a note's earlier texts, newest first, or read one.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args historyArgs) (*mcp.CallToolResult, history, error) {
		id, err := apiclient.Resolve(ctx, client, apiclient.NOTE, args.Note)
		if err != nil {
			return nil, history{}, err
		}
		if args.Version == "" {
			versions, err := decode[[]apiclient.DomainNoteVersion](client.GetApiNotesIdVersions(ctx, id))
			return nil, history{Versions: versions}, err
		}
		version, err := decode[apiclient.DomainNoteVersion](client.GetApiNotesIdVersionsVersion(ctx, id, args.Version))
		if err != nil {
			return nil, history{}, err
		}
		return nil, history{Version: &version}, nil
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "note_links",
		Annotations: reads(),
		Description: "List a note's links, backlinks and the notes sharing its tags; without a note, the broken links.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args linksArgs) (*mcp.CallToolResult, links, error) {
		if args.Note == "" {
			broken, err := decode[[]apiclient.ListbrokenlinksLink](client.GetApiLinksBroken(ctx))
			return nil, links{Broken: broken}, err
		}
		id, err := apiclient.Resolve(ctx, client, apiclient.NOTE, args.Note)
		if err != nil {
			return nil, links{}, err
		}
		result, err := decode[apiclient.ReadneighborsResult](client.GetApiNotesIdNeighbors(ctx, id))
		if err != nil {
			return nil, links{}, err
		}
		return nil, links{Links: result.Links, Backlinks: result.Backlinks, Neighbors: result.Neighbors}, nil
	})
}

// editText changes a part of the text of a note, answered without its content.
func editText(ctx context.Context, client *apiclient.ClientWithResponses, ref string, input apiclient.EditnotetextInput) (partEdit, error) {
	id, err := apiclient.Resolve(ctx, client, apiclient.NOTE, ref)
	if err != nil {
		return partEdit{}, err
	}
	result, err := decode[apiclient.EditnotetextResult](client.PatchApiNotesIdText(ctx, id, input))
	if err != nil {
		return partEdit{}, err
	}
	edited, err := readNote(result.Note, nil)
	edited.Content = ""
	return partEdit{note: edited, Replaced: result.Replaced}, err
}

// readNote turns the note the API answers into what the agent reads.
func readNote(result apiclient.ReaditemResult, err error) (note, error) {
	if err != nil {
		return note{}, err
	}
	return note{
		ID:        result.Item.Id,
		Title:     result.Item.Title,
		Path:      result.Path,
		Content:   apiclient.Deref(result.Content),
		Tags:      append([]string{}, apiclient.Deref(result.Tags)...),
		Readonly:  apiclient.Deref(result.Readonly),
		CreatedAt: result.Item.CreatedAt,
		UpdatedAt: editedAt(result.Item.UpdatedAt, result.Item.CreatedAt),
	}, nil
}
