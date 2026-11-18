package cmdmcp

import (
	"context"
	"slices"
	"strings"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/core/services/hashtag"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// note is what the read and the writes answer, the content left out when the agent did not ask for it.
type note struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Path      string   `json:"path"`
	Content   string   `json:"content,omitempty"`
	Length    *int     `json:"length,omitempty"`    // the whole content's characters, given by read_notes and with max_chars
	Truncated *bool    `json:"truncated,omitempty"` // a pointer: false is said too, by read_notes and with max_chars
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
	Notes    []string `json:"notes" jsonschema:"the notes: their uuid, or their path from the root (/Folder/Note); 50 at most"`
	MaxChars int      `json:"max_chars,omitempty" jsonschema:"only the first that many characters of each content, with its whole length and truncated: a preview to sort notes before reading in full only those that need it"`
}

type noteArgs struct {
	Note     string `json:"note" jsonschema:"the note: its uuid, or its path from the root (/Folder/Note)"`
	MaxChars int    `json:"max_chars,omitempty" jsonschema:"only the first that many characters of the content, with its whole length and truncated: a preview before reading it in full"`
}

type createArgs struct {
	Content string   `json:"content" jsonschema:"the markdown content; its first heading becomes the title and its #hashtags become tags"`
	Folder  string   `json:"folder,omitempty" jsonschema:"the folder to create the note in: its uuid, or its path from the root (/Folder); the root itself when empty"`
	Tags    []string `json:"tags,omitempty" jsonschema:"tags to add to the content, without the #"`
}

type editArgs struct {
	Note    string   `json:"note" jsonschema:"the note to rewrite: its uuid, or its path from the root (/Folder/Note)"`
	Content string   `json:"content" jsonschema:"the new markdown content; it replaces the whole note"`
	Tags    []string `json:"tags,omitempty" jsonschema:"tags to add to the content, without the #"`
}

type renameNoteArgs struct {
	Note  string `json:"note" jsonschema:"the note to rename: its uuid, or its path from the root (/Folder/Note)"`
	Title string `json:"title" jsonschema:"its new title, on one line"`
}

type replaceInNoteArgs struct {
	Note    string `json:"note" jsonschema:"the note to change: its uuid, or its path from the root (/Folder/Note)"`
	Old     string `json:"old,omitempty" jsonschema:"the exact text to replace, with its case, as read_note gives it: found more than once refuses, unless all"`
	New     string `json:"new,omitempty" jsonschema:"the text that takes the place of old or of the section's body, empty to delete it"`
	All     bool   `json:"all,omitempty" jsonschema:"replace every occurrence of old instead of refusing more than one"`
	Section string `json:"section,omitempty" jsonschema:"instead of old, the words of a heading: what follows it, down to the next heading of its level or higher, becomes new, the heading kept"`
}

// partEdit is a note changed in part: the agent knows what it sent, so the content is left out.
type partEdit struct {
	note
	Replaced int `json:"replaced"`
}

type replaceArgs struct {
	Find    string `json:"find" jsonschema:"the exact text to find, with its case: plain text, not a regex"`
	Replace string `json:"replace" jsonschema:"the text that takes its place, empty to delete it"`
	Folder  string `json:"folder,omitempty" jsonschema:"only the notes of this folder and its subfolders, as a path or an uuid; empty for every note"`
	DryRun  bool   `json:"dry_run,omitempty" jsonschema:"only count the notes and occurrences, change nothing: do it first"`
}

type historyArgs struct {
	Note    string `json:"note" jsonschema:"the note: its uuid, or its path from the root (/Folder/Note)"`
	Version string `json:"version,omitempty" jsonschema:"the uuid of a version to read, as the list gives it; the list of versions when empty"`
}

// history is the list of versions, their texts left out, or the one version asked with its text.
type history struct {
	Versions []apiclient.DomainNoteVersion `json:"versions,omitempty"`
	Version  *apiclient.DomainNoteVersion  `json:"version,omitempty"`
}

type linksArgs struct {
	Note string `json:"note,omitempty" jsonschema:"the note: its uuid, or its path from the root (/Folder/Note); the broken links of every note when empty"`
}

// links joins a note to the others, the note itself left out (read_note reads it), or lists the broken links of every note.
type links struct {
	Links     []apiclient.ReadneighborsLinked   `json:"links,omitempty"`
	Backlinks []apiclient.ReadneighborsLinked   `json:"backlinks,omitempty"`
	Neighbors []apiclient.ReadneighborsNeighbor `json:"neighbors,omitempty"`
	Broken    []apiclient.ListbrokenlinksLink   `json:"broken,omitempty"`
}

func addNoteTools(server *mcp.Server, client *apiclient.ClientWithResponses, readOnly bool) {
	addTool(server, readOnly, &mcp.Tool{
		Name:        "read_note",
		Annotations: reads(),
		Description: "Read a note: its content, its path and its tags. max_chars gives only the start of the content, cut at the end of a line or a word, with its length and truncated.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args noteArgs) (*mcp.CallToolResult, note, error) {
		if args.MaxChars > 0 {
			// The server cuts at a word or a line, as for read_notes (#222).
			found, err := decode[[]apiclient.ReadnotesNote](client.PostApiNotesRead(ctx, apiclient.ReadnotesBody{Notes: []string{args.Note}, MaxChars: &args.MaxChars}))
			if err != nil || len(found) != 1 {
				return nil, note{}, err
			}
			f := found[0]
			read, _ := readNote(apiclient.ReaditemResult{Item: f.Item, Path: f.Path, Content: f.Content, Readonly: f.Readonly, Tags: f.Tags}, nil)
			read.Length, read.Truncated = &f.Length, &f.Truncated
			return nil, read, nil
		}
		id, err := apiclient.Resolve(ctx, client, apiclient.NOTE, args.Note)
		if err != nil {
			return nil, note{}, err
		}
		read, err := readNote(decode[apiclient.ReaditemResult](client.GetApiNotesId(ctx, id)))
		return nil, read, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "read_notes",
		Annotations: reads(),
		Description: "Read several notes at once, up to 50, each as read_note gives it plus its length and truncated (true or false), in the order asked: one missing fails the call. Answers {common, notes}: what every note shares (folder, created_at, updated_at) is said once in common and left out of the notes, whose path is then common.folder plus name (name left out when it is the title). To sort many notes, read them first with max_chars (the start of each, cut at the end of a line or a word; its tags stay whole in their own field, those written at the end of the text may be cut out of the content), then in full only those that need it.",
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
		Name:        "create_note",
		Annotations: writes(false, false),
		Description: "Create a note from markdown content.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args createArgs) (*mcp.CallToolResult, note, error) {
		parentID, err := resolveFolder(ctx, client, args.Folder)
		if err != nil {
			return nil, note{}, err
		}
		created, err := decode[apiclient.DomainItem](client.PostApiNotes(ctx, apiclient.CreatenoteInput{Content: args.Content, ParentId: &parentID, Tags: &args.Tags}))
		if err != nil {
			return nil, note{}, err
		}
		// Read back: the API answers the item alone, without the path nor the tags it parsed.
		read, err := readNote(decode[apiclient.ReaditemResult](client.GetApiNotesId(ctx, created.Id)))
		return nil, read, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "edit_note",
		Annotations: writes(true, true),
		Description: "Replace the content of a note, answered without it. Read it first: nothing is kept of the old content, and its first heading becomes the title. To only add or remove tags, use tag_notes. A locked note refuses.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args editArgs) (*mcp.CallToolResult, note, error) {
		id, err := apiclient.Resolve(ctx, client, apiclient.NOTE, args.Note)
		if err != nil {
			return nil, note{}, err
		}
		input := apiclient.EditnoteBody{Content: &args.Content}
		// PATCH makes the tags exactly those given, the content's #tags kept with them: tags add, as in create_note.
		if len(args.Tags) > 0 {
			tags := append(hashtag.Parse(args.Content), args.Tags...)
			input.Tags = &tags
		}
		edited, err := readNote(decode[apiclient.ReaditemResult](client.PatchApiNotesId(ctx, id, input)))
		edited.Content = "" // the agent has just sent it
		return nil, edited, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "rename_note",
		Annotations: writes(false, true),
		Description: "Change the title of a note without sending its content: its first heading takes the new title, or a heading is added on top, as the app renames a note; the [[links]] to it follow. The earlier text goes to note_history like an edit_note's. A locked note refuses.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args renameNoteArgs) (*mcp.CallToolResult, note, error) {
		edited, err := editText(ctx, client, args.Note, apiclient.EditnotetextInput{Title: &args.Title})
		return nil, edited.note, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "replace_in_note",
		Annotations: writes(true, false),
		Description: "Correct a part of one note without rewriting it: old becomes new, old found exactly once (more refuses with the count, unless all; none refuses), or the body of a section, given by its heading, becomes new. The rest of the text stays as it is; the title and the tags follow the new text, the earlier text goes to note_history like an edit_note's. A locked note refuses. Answers the note without its content and replaced, the occurrences changed. To change a text in many notes, replace_notes.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args replaceInNoteArgs) (*mcp.CallToolResult, partEdit, error) {
		input := apiclient.EditnotetextInput{All: &args.All}
		if args.Section != "" {
			input.Section = &args.Section
		} else {
			input.Old = &args.Old
		}
		if args.New != "" {
			input.New = &args.New
		}
		edited, err := editText(ctx, client, args.Note, input)
		return nil, edited, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "delete_note",
		Annotations: writes(true, true),
		Description: "Move a note to the trash, where restore_item can take it back.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args noteArgs) (*mcp.CallToolResult, []item, error) {
		id, err := apiclient.Resolve(ctx, client, apiclient.NOTE, args.Note)
		if err != nil {
			return nil, nil, err
		}
		return made(decode[[]apiclient.DomainItem](client.DeleteApiNotesId(ctx, id)))
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "replace_notes",
		Annotations: writes(true, false),
		Description: "Replace a plain text (with its case, not a regex) at every occurrence in every note of the account out of the trash, or of folder and its subfolders, in one transaction, without rewriting them: the hidden targets (a note link's ](note:<id>), a file's address) are left as they are, locked notes are left and counted, and each note changed keeps its earlier text in note_history. Answers the notes and occurrences changed; with dry_run, only counts them. Needs the Ultra plan. For one note, and a text that must be there once, replace_in_note.",
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
		return nil, result, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "note_history",
		Annotations: reads(),
		Description: "List the earlier texts of a note, newest first, or read one: what edit_note replaced.",
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
		Description: "List the notes a note links to, those linking to it, and those sharing its tags. Without a note, the broken links of every note.",
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
