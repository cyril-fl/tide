// Package cmdmcp serves the account's notes to an agent over MCP, a client of the API like the rest of the CLI:
// the rules (titles, tags, links, trash, encryption) are those of the server.
package cmdmcp

import (
	"archive/zip"
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"slices"
	"strconv"
	"strings"
	"testing/fstest"
	"time"

	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/config"
	"github.com/cyril-fl/tide/core/services/hashtag"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

var errBadPath = errors.New("not the relative path of a .md file")

// filesDir is exportnotes.FilesDir, where an export puts the images and attachments.
const filesDir = "_files"

// fileURL is domain.FileURL, how a note cites a file.
func fileURL(id string) string {
	return "/api/files/" + id
}

// item is a note or a folder as an agent needs it, with its dates to judge whether it is still current (list_folder
// leaves them out unless asked).
type item struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// folderPage is a page of the items of a folder, and how many it holds in all.
type folderPage struct {
	Items []item `json:"items"`
	Total int    `json:"total"`
}

// note is what the read and the writes answer, the content left out when the agent did not ask for it.
type note struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Path      string   `json:"path"`
	Content   string   `json:"content,omitempty"`
	Length    *int     `json:"length,omitempty"` // the whole content's characters, given with max_chars
	Truncated bool     `json:"truncated,omitempty"`
	Tags      []string `json:"tags"` // [] rather than left out: the agent sees the note has none
	Readonly  bool     `json:"readonly,omitempty"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at,omitempty"`
}

// pageSize is how many results a list gives when the agent names no limit: a common word or a flat folder holds
// thousands of notes, megabytes an agent cannot read.
const pageSize = 50

// limitOf is the limit an agent asked, pageSize when it named none, 0 (all) for a negative one.
func limitOf(asked *int) int {
	if asked == nil {
		return pageSize
	}
	return max(*asked, 0)
}

type searchArgs struct {
	Text          string `json:"text,omitempty" jsonschema:"the text to find in the title or the content of the notes; may be left out with max_length, untagged or updated_before"`
	MaxLength     *int   `json:"max_length,omitempty" jsonschema:"only the notes whose content, title line included, has at most that many characters: the empty or title-only ones"`
	Untagged      bool   `json:"untagged,omitempty" jsonschema:"only the notes with no tag"`
	UpdatedBefore string `json:"updated_before,omitempty" jsonschema:"only the notes last changed before this day (2026-01-31) or RFC 3339 instant: the stale ones, the oldest first when there is no text"`
	Limit         *int   `json:"limit,omitempty" jsonschema:"how many notes at most, 50 when left out, 0 for all of them"`
	Offset        int    `json:"offset,omitempty" jsonschema:"how many matches to skip: the next page starts at offset + limit"`
}

// searchPage is a page of matches and their total: those read, and the notes after them that may match.
type searchPage struct {
	Matches []apiclient.SearchnotesMatch `json:"matches"`
	Total   int                          `json:"total"`
}

type notesArgs struct {
	Notes    []string `json:"notes" jsonschema:"the notes: their uuid, or their path from the root (/Folder/Note); 50 at most"`
	MaxChars int      `json:"max_chars,omitempty" jsonschema:"only the first that many characters of each content, with its whole length and truncated: a preview to sort notes before reading in full only those that need it"`
}

type listFolderArgs struct {
	Folder string `json:"folder,omitempty" jsonschema:"the folder: its uuid, or its path from the root (/Folder); the root itself when empty"`
	Limit  *int   `json:"limit,omitempty" jsonschema:"how many items at most, 50 when left out, 0 for all of them"`
	Offset int    `json:"offset,omitempty" jsonschema:"how many items to skip, folders first then by title: the next page starts at offset + limit"`
	Dates  bool   `json:"dates,omitempty" jsonschema:"also the created_at and updated_at of each item, to judge whether it is still current"`
}

type noteArgs struct {
	Note     string `json:"note" jsonschema:"the note: its uuid, or its path from the root (/Folder/Note)"`
	MaxChars int    `json:"max_chars,omitempty" jsonschema:"only the first that many characters of the content, with its whole length and truncated: a preview before reading it in full"`
}

type folderArgs struct {
	Folder string `json:"folder,omitempty" jsonschema:"the folder: its uuid, or its path from the root (/Folder); the root itself when empty"`
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

type createFolderArgs struct {
	Title  string `json:"title" jsonschema:"the new folder's title"`
	Folder string `json:"folder,omitempty" jsonschema:"the folder to create it in: its uuid, or its path from the root (/Folder); the root itself when empty"`
}

type renameFolderArgs struct {
	Folder string `json:"folder" jsonschema:"the folder to rename: its uuid, or its path from the root (/Folder)"`
	Title  string `json:"title" jsonschema:"its new title"`
}

type moveArgs struct {
	Item   string `json:"item" jsonschema:"the note or folder to move: its uuid, or its path from the root (/Folder/Note)"`
	Folder string `json:"folder,omitempty" jsonschema:"the folder to move it into: its uuid, or its path from the root (/Folder); the root itself when empty"`
}

type trashedArgs struct {
	Item string `json:"item" jsonschema:"the uuid of the trashed note or folder, as list_trash gives it"`
}

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
	Tag   string `json:"tag" jsonschema:"the tag to rename, without the #"`
	To    string `json:"to" jsonschema:"its new name, without the #"`
	Merge bool   `json:"merge,omitempty" jsonschema:"when to is another tag already, fold the tag into it instead of refusing"`
}

type tagNotesArgs struct {
	Notes         []string `json:"notes,omitempty" jsonschema:"the notes: their uuid, or their path from the root (/Folder/Note); or leave out for the notes text, max_length, untagged and updated_before find"`
	Text          string   `json:"text,omitempty" jsonschema:"instead of notes, every note whose title or content contains this text, as search_notes finds it"`
	MaxLength     *int     `json:"max_length,omitempty" jsonschema:"instead of notes, every note whose content, title line included, has at most that many characters"`
	Untagged      bool     `json:"untagged,omitempty" jsonschema:"instead of notes, every note with no tag"`
	UpdatedBefore string   `json:"updated_before,omitempty" jsonschema:"instead of notes, every note last changed before this day (2026-01-31) or RFC 3339 instant"`
	DryRun        bool     `json:"dry_run,omitempty" jsonschema:"with text, max_length, untagged or updated_before, only list the notes found, change nothing: do it first"`
	Add           []string `json:"add,omitempty" jsonschema:"tags to add, without the #"`
	Remove        []string `json:"remove,omitempty" jsonschema:"tags to remove, without the #"`
}

// tagged is a note after tag_notes, its content left out, or why it was left as it was.
type tagged struct {
	Note  string   `json:"note"`
	ID    string   `json:"id,omitempty"`
	Title string   `json:"title,omitempty"`
	Path  string   `json:"path,omitempty"`
	Tags  []string `json:"tags,omitempty"`
	Error string   `json:"error,omitempty"`
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

type fileArgs struct {
	File string `json:"file,omitempty" jsonschema:"the uuid of a file, as a note cites it (/api/files/<uuid>) or the list gives it; the list of files when empty"`
}

type attachArgs struct {
	Name string `json:"name" jsonschema:"the file name, with its extension"`
	Data string `json:"data" jsonschema:"the bytes of the file in base64: an image (PNG, JPEG, GIF, WebP), a PDF, a zip, an audio or a video file"`
}

// attached is the stored file and the markdown a note cites it with.
type attached struct {
	ID       string `json:"id"`
	Markdown string `json:"markdown"`
}

type exportArgs struct {
	Folder string   `json:"folder,omitempty" jsonschema:"the folder to export: its uuid, or its path from the root (/Folder); everything when empty"`
	Items  []string `json:"items,omitempty" jsonschema:"notes or folders to export instead of a folder: their uuid, or their path from the root"`
	Limit  *int     `json:"limit,omitempty" jsonschema:"how many files at most, 50 when left out, 0 for all of them"`
	Offset int      `json:"offset,omitempty" jsonschema:"how many files to skip: the next page starts at offset + limit"`
}

// exported is a note as a markdown file with its id and last change, or a file it cites, whose bytes read_file gives.
type exported struct {
	Path      string `json:"path"`
	ID        string `json:"id,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	Content   string `json:"content,omitempty"`
	File      string `json:"file,omitempty"`
}

// export is a page of the exported files, and how many there are in all.
type export struct {
	Files []exported `json:"files"`
	Total int        `json:"total"`
}

type importArgs struct {
	Files  []importFile `json:"files" jsonschema:"the markdown files; their directories become folders"`
	Folder string       `json:"folder,omitempty" jsonschema:"the folder to import into: its uuid, or its path from the root (/Folder); the root itself when empty"`
}

type importFile struct {
	Path    string `json:"path" jsonschema:"the file path, relative (Folder/Note.md)"`
	Content string `json:"content" jsonschema:"the markdown content"`
}

type snippetArgs struct {
	Name    string `json:"name" jsonschema:"the snippet's name; one of that name already is replaced"`
	Content string `json:"content" jsonschema:"the markdown the snippet inserts; its #hashtags become tags of the note it starts"`
}

type snippetNameArgs struct {
	Name string `json:"name" jsonschema:"the name of the snippet to remove"`
}

var errNoSnippet = errors.New("no snippet of that name")

type savedSearchesArgs struct {
	Search string `json:"search,omitempty" jsonschema:"the saved search to run: its id, or its name; the list of saved searches when empty"`
	Limit  *int   `json:"limit,omitempty" jsonschema:"how many notes at most, 50 when left out, 0 for all of them"`
	Offset int    `json:"offset,omitempty" jsonschema:"how many notes to skip, by path: the next page starts at offset + limit"`
}

type saveSearchArgs struct {
	Search      string   `json:"search,omitempty" jsonschema:"the saved search to replace: its id, or its name; a new one when empty"`
	Name        string   `json:"name" jsonschema:"the name the app shows"`
	Tags        []string `json:"tags,omitempty" jsonschema:"tags the notes carry, without the #"`
	TagMode     string   `json:"tag_mode,omitempty" jsonschema:"any (default): one of the tags is enough; all: every tag"`
	Folders     []string `json:"folders,omitempty" jsonschema:"folders, with their subfolders: their uuid, or their path from the root (/Folder)"`
	FolderMode  string   `json:"folder_mode,omitempty" jsonschema:"include (default): only the notes of the folders; exclude: every note but theirs"`
	CreatedFrom string   `json:"created_from,omitempty" jsonschema:"notes created on or after this day (2026-01-31) or instant (RFC 3339)"`
	CreatedTo   string   `json:"created_to,omitempty" jsonschema:"notes created on or before this day (2026-01-31, the whole day) or instant (RFC 3339)"`
	EditedFrom  string   `json:"edited_from,omitempty" jsonschema:"notes changed on or after this day (2026-01-31) or instant (RFC 3339)"`
	EditedTo    string   `json:"edited_to,omitempty" jsonschema:"notes changed on or before this day (2026-01-31, the whole day) or instant (RFC 3339)"`
	Text        string   `json:"text,omitempty" jsonschema:"a text the title or the content holds"`
}

type searchNameArgs struct {
	Search string `json:"search" jsonschema:"the saved search to remove: its id, or its name"`
}

// savedSearch is a domain.SavedSearch as an agent reads it: tags by name, folders by path.
type savedSearch struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Tags        []string `json:"tags,omitempty"`
	TagMode     string   `json:"tag_mode,omitempty"`
	Folders     []string `json:"folders,omitempty"`
	FolderMode  string   `json:"folder_mode,omitempty"`
	CreatedFrom string   `json:"created_from,omitempty"`
	CreatedTo   string   `json:"created_to,omitempty"`
	EditedFrom  string   `json:"edited_from,omitempty"`
	EditedTo    string   `json:"edited_to,omitempty"`
	Text        string   `json:"text,omitempty"`
}

// searches is the account's saved searches, or one run with a page of its notes and their total.
type searches struct {
	Searches []savedSearch                   `json:"searches,omitempty"`
	Search   *savedSearch                    `json:"search,omitempty"`
	Notes    []apiclient.RunsavedsearchMatch `json:"notes,omitempty"`
	Total    int                             `json:"total,omitempty"`
}

var (
	errNoPro    = errors.New("saved searches (smart folders) need the Pro plan, which this account does not have")
	errNoSearch = errors.New("no saved search of that id or name")
	errNoTag    = errors.New("no tag of that name")
)

// Serve speaks MCP on stdin and stdout: nothing else may be printed there.
func Serve(cobra *cobra.Command, _ []string) error {
	// The account is the session's, as for every client of the API: the handlers act for it.
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	server := mcp.NewServer(&mcp.Implementation{Name: config.Current.Name, Title: config.Title(), Version: cobra.Root().Version}, &mcp.ServerOptions{Instructions: Instructions(false)})
	AddTools(server, client, false)
	return server.Run(cobra.Context(), &mcp.StdioTransport{})
}

// Instructions tells an agent the cheap way through a big account, which its client puts in its system prompt: in the
// benchmark of #198, reading whole notes and rewriting them to tag them made most of the cost. The writing tips are
// left out when readOnly, whose tools cannot write.
func Instructions(readOnly bool) string {
	text := "The account's markdown notes, in folders; name a note or folder by its uuid or its path from the root (/Folder/Note).\n" +
		"Work through many notes reading as little as possible:\n" +
		"- Call tags once for the account's tags (prefix or min_notes to narrow it) and keep the list.\n" +
		"- Find the candidates on the server instead of reading: search_notes with untagged, max_length (empty or title-only notes), updated_before (stale notes) or a text.\n" +
		"- Read up to 50 notes per call with read_notes and max_chars first; in full only those that need it.\n" +
		"- Lists give 50 items and the total: split a big folder into list_folder pages (limit, offset), one per subagent if you have them.\n" +
		"- Subagents do not receive these instructions: put the lines they need in their prompt."
	if !readOnly {
		text += "\n- Tag with tag_notes, given the notes or the filters of search_notes (dry_run first), never by rewriting a note with edit_note, which replaces all of it.\n" +
			"- Fold a tag into another with rename_tag and merge; replace a text in every note with replace_notes (dry_run first).\n" +
			"- Rewrites keep the old text in note_history, deletions go to the trash."
	}
	return text
}

// AddTools gives the server the tools acting through client, for the account of its session; the reading ones only
// when readOnly.
func AddTools(server *mcp.Server, client *apiclient.ClientWithResponses, readOnly bool) {
	addTool(server, readOnly, &mcp.Tool{
		Name:        "search_notes",
		Annotations: reads(),
		Description: "Find the notes whose title or content contains a text, with the line where it was found, and their total: the first 50, title matches first, or limit and offset. max_length, untagged and updated_before keep only the short, untagged or stale notes, with or without a text, so that sorting the account reads only those.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args searchArgs) (*mcp.CallToolResult, searchPage, error) {
		limit := limitOf(args.Limit)
		params := &apiclient.GetApiSearchParams{Q: &args.Text, MaxLength: args.MaxLength, Untagged: &args.Untagged, Limit: &limit, Offset: &args.Offset}
		if args.UpdatedBefore != "" {
			params.UpdatedBefore = &args.UpdatedBefore
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
		Name:        "read_note",
		Annotations: reads(),
		Description: "Read a note: its content, its path and its tags. max_chars gives only the start of the content, with its length and truncated.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args noteArgs) (*mcp.CallToolResult, note, error) {
		id, err := apiclient.Resolve(ctx, client, apiclient.NOTE, args.Note)
		if err != nil {
			return nil, note{}, err
		}
		read, err := readNote(decode[apiclient.ReaditemResult](client.GetApiNotesId(ctx, id)))
		return nil, preview(read, args.MaxChars), err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "read_notes",
		Annotations: reads(),
		Description: "Read several notes at once, up to 50, each as read_note gives it, in the order asked: one missing fails the call. To sort many notes, read them first with max_chars (the start of each, its length and truncated), then in full only those that need it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args notesArgs) (*mcp.CallToolResult, []note, error) {
		// One request for the batch, the preview cut on the server: the agent may be far over Tailscale (#210).
		found, err := decode[[]apiclient.ReadnotesNote](client.PostApiNotesRead(ctx, apiclient.ReadnotesBody{Notes: args.Notes, MaxChars: &args.MaxChars}))
		if err != nil {
			return nil, nil, err
		}
		read := make([]note, 0, len(found))
		for _, f := range found {
			n, _ := readNote(apiclient.ReaditemResult{Item: f.Item, Path: f.Path, Content: f.Content, Readonly: f.Readonly, Tags: f.Tags}, nil)
			n.Length, n.Truncated = f.Length, apiclient.Deref(f.Truncated)
			read = append(read, n)
		}
		return nil, read, nil
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "list_folder",
		Annotations: reads(),
		Description: "List the notes and folders directly inside a folder, folders first then by title: the first 50 and the total, unless limit says otherwise. dates adds when each was created and changed.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args listFolderArgs) (*mcp.CallToolResult, folderPage, error) {
		parentID, err := resolveFolder(ctx, client, args.Folder)
		if err != nil {
			return nil, folderPage{}, err
		}
		limit, offset := limitOf(args.Limit), max(args.Offset, 0)
		found, total, err := paged[[]apiclient.DomainItem](client.GetApiItems(ctx, &apiclient.GetApiItemsParams{Parent: &parentID, Limit: &limit, Offset: &offset}))
		_, items, err := made(found, err)
		if !args.Dates {
			for i := range items {
				items[i].CreatedAt, items[i].UpdatedAt = "", ""
			}
		}
		return nil, folderPage{Items: items, Total: total}, err
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
		Name:        "tag_notes",
		Annotations: writes(false, true),
		Description: "Add or remove tags on notes without sending their content: only their #tags change, the rest of the text stays as it is, and each note changed keeps its earlier text in note_history. A locked note is left, with the error. Instead of naming the notes, text, max_length, untagged and updated_before tag every note search_notes would find, without reading them; with dry_run, only lists those notes.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args tagNotesArgs) (*mcp.CallToolResult, []tagged, error) {
		for _, tag := range args.Add {
			if !hashtag.Valid(tag) {
				return nil, nil, fmt.Errorf("%w: %q", hashtag.ErrInvalid, tag)
			}
		}
		if searched := args.Text != "" || args.MaxLength != nil || args.Untagged || args.UpdatedBefore != ""; searched {
			if len(args.Notes) > 0 {
				return nil, nil, errors.New("give either notes or text, max_length, untagged and updated_before, not both")
			}
			all := 0
			params := &apiclient.GetApiSearchParams{Q: &args.Text, MaxLength: args.MaxLength, Untagged: &args.Untagged, Limit: &all}
			if args.UpdatedBefore != "" {
				params.UpdatedBefore = &args.UpdatedBefore
			}
			matches, _, err := paged[[]apiclient.SearchnotesMatch](client.GetApiSearch(ctx, params))
			if err != nil {
				return nil, nil, err
			}
			for _, match := range matches {
				args.Notes = append(args.Notes, match.Id)
			}
			if args.DryRun {
				out := make([]tagged, 0, len(matches))
				for _, match := range matches {
					out = append(out, tagged{Note: match.Id, ID: match.Id, Title: match.Title, Path: match.Path})
				}
				return nil, out, nil
			}
		} else if args.DryRun {
			return nil, nil, errors.New("dry_run goes with text, max_length, untagged or updated_before")
		}
		if len(args.Notes) == 0 {
			return nil, []tagged{}, nil
		}
		// The same fields: the API's answer is what the agent reads. Hundreds of notes take longer than Timeout.
		ctx, cancel := apiclient.Long(ctx)
		defer cancel()
		out, err := decode[[]tagged](client.PatchApiNotesTags(ctx, apiclient.TagnotesInput{Notes: args.Notes, Add: &args.Add, Remove: &args.Remove}))
		return nil, out, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "create_folder",
		Annotations: writes(false, false),
		Description: "Create a folder.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args createFolderArgs) (*mcp.CallToolResult, item, error) {
		parentID, err := resolveFolder(ctx, client, args.Folder)
		if err != nil {
			return nil, item{}, err
		}
		created, err := decode[apiclient.DomainItem](client.PostApiFolders(ctx, apiclient.CreatefolderInput{Title: args.Title, ParentId: &parentID}))
		return nil, one(created), err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "rename_folder",
		Annotations: writes(false, true),
		Description: "Change the title of a folder.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args renameFolderArgs) (*mcp.CallToolResult, item, error) {
		id, err := apiclient.Resolve(ctx, client, apiclient.FOLDER, args.Folder)
		if err != nil {
			return nil, item{}, err
		}
		renamed, err := decode[apiclient.DomainItem](client.PatchApiFoldersId(ctx, id, apiclient.EditfolderBody{Title: &args.Title}))
		return nil, one(renamed), err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "move_item",
		Annotations: writes(false, true),
		Description: "Move a note or a folder into another folder. A folder takes its content with it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args moveArgs) (*mcp.CallToolResult, []item, error) {
		id, err := apiclient.Resolve(ctx, client, "", args.Item)
		if err != nil {
			return nil, nil, err
		}
		parentID, err := resolveFolder(ctx, client, args.Folder)
		if err != nil {
			return nil, nil, err
		}
		return made(decode[[]apiclient.DomainItem](client.PostApiMove(ctx, apiclient.MoveitemsInput{Ids: []string{id}, ParentId: &parentID})))
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
		Name:        "delete_folder",
		Annotations: writes(true, true),
		Description: "Move a folder and everything inside it to the trash, where restore_item can take it back.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args folderArgs) (*mcp.CallToolResult, []item, error) {
		id, err := apiclient.Resolve(ctx, client, apiclient.FOLDER, args.Folder)
		if err != nil {
			return nil, nil, err
		}
		return made(decode[[]apiclient.DomainItem](client.DeleteApiFoldersId(ctx, id)))
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "list_trash",
		Annotations: reads(),
		Description: "List what is in the trash, last removed first, with the uuid restore_item takes.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.ListtrashTrashed, error) {
		trash, err := decode[[]apiclient.ListtrashTrashed](client.GetApiTrash(ctx))
		return nil, trash, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "restore_item",
		Annotations: writes(false, true),
		Description: "Take a note or a folder out of the trash, with everything removed at the same time as it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args trashedArgs) (*mcp.CallToolResult, []item, error) {
		// No resolve: a trashed item has no path to look up.
		return made(decode[[]apiclient.DomainItem](client.PostApiRestore(ctx, apiclient.RestoreBody{Ids: []string{args.Item}})))
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
				read.CreatedAt, read.UpdatedAt = found.CreatedAt, apiclient.Deref(found.UpdatedAt)
			}
			notes = append(notes, read)
		}
		return nil, tags{Tag: result.Tag.Value, Notes: notes, Total: total}, nil
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "rename_tag",
		Annotations: writes(false, true),
		Description: "Rename a tag on every note carrying it, in their text too, each note changed keeping its earlier text in note_history. A name taken by another tag refuses, unless merge folds the tag into it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args renameTagArgs) (*mcp.CallToolResult, apiclient.DomainTag, error) {
		renamed, err := decode[apiclient.DomainTag](client.PatchApiTagsValue(ctx, args.Tag, apiclient.EdittagBody{Value: args.To, Merge: &args.Merge}))
		return nil, renamed, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "replace_notes",
		Annotations: writes(true, false),
		Description: "Replace a plain text in many notes at once, in one transaction, without rewriting them: locked notes are left and counted, and each note changed keeps its earlier text in note_history. With dry_run, only counts what it would change.",
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
		Name:        "delete_tag",
		Annotations: writes(true, true),
		Description: "Remove a tag from every note carrying it, in their text too, each note changed keeping its earlier text in note_history. A note holding nothing but the tag goes to the trash.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args tagArgs) (*mcp.CallToolResult, []apiclient.DomainTag, error) {
		deleted, err := decode[[]apiclient.DomainTag](client.DeleteApiTagsValue(ctx, args.Tag))
		return nil, deleted, err
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

	// Out is any: a read answers the file as content (an image the agent sees), not as JSON.
	addTool(server, readOnly, &mcp.Tool{
		Name:        "read_file",
		Annotations: reads(),
		Description: "Read an image or attachment a note cites. Without a file, list the account's files, largest first.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args fileArgs) (*mcp.CallToolResult, any, error) {
		if args.File == "" {
			files, err := decode[[]apiclient.DomainFile](client.GetApiFiles(ctx))
			return nil, files, err
		}
		ctx, cancel := apiclient.Long(ctx)
		defer cancel()
		response, err := client.GetApiFilesIdWithResponse(ctx, args.File)
		if err != nil {
			return nil, nil, err
		}
		if response.StatusCode() != http.StatusOK {
			return nil, nil, apiclient.Fail(response.HTTPResponse, response.Body)
		}
		content := fileContent(args.File, response.HTTPResponse.Header.Get("Content-Type"), response.Body)
		return &mcp.CallToolResult{Content: []mcp.Content{content}}, nil, nil
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "attach_file",
		Annotations: writes(false, false),
		Description: "Store an image or attachment and answer the markdown citing it, to put in a note with create_note or edit_note.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args attachArgs) (*mcp.CallToolResult, attached, error) {
		data, err := base64.StdEncoding.DecodeString(args.Data)
		if err != nil {
			return nil, attached{}, fmt.Errorf("data is not base64: %w", err)
		}
		ctx, cancel := apiclient.Long(ctx)
		defer cancel()
		// The server reads the type from the bytes, not from this header.
		file, err := decode[apiclient.DomainFile](client.PostApiFilesWithBody(ctx, &apiclient.PostApiFilesParams{Name: &args.Name}, "application/octet-stream", bytes.NewReader(data)))
		if err != nil {
			return nil, attached{}, err
		}
		return nil, attached{ID: file.Id, Markdown: citation(file, args.Name)}, nil
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "export_notes",
		Annotations: reads(),
		Description: "Export notes as markdown files with their id and last change, [[links]] written back, their folders as directories: the first 50 files and the total, unless limit says otherwise. A path may not lead back to its note (a duplicate title becomes 'Title (2)', a / a -): read a note by its id.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args exportArgs) (*mcp.CallToolResult, export, error) {
		folderID, err := resolveFolder(ctx, client, args.Folder)
		if err != nil {
			return nil, export{}, err
		}
		ids := make([]string, 0, len(args.Items))
		for _, ref := range args.Items {
			id, err := apiclient.Resolve(ctx, client, "", ref)
			if err != nil {
				return nil, export{}, err
			}
			ids = append(ids, id)
		}

		withFiles := false // The images are only named here: their bytes would weigh down a big account's export.
		params := apiclient.GetApiExportParams{Parent: &folderID, Files: &withFiles}
		if len(ids) > 0 {
			params.Id = &ids // An empty list would still send id=.
		}
		ctx, cancel := apiclient.Long(ctx)
		defer cancel()
		response, err := client.GetApiExportWithResponse(ctx, &params)
		if err != nil {
			return nil, export{}, err
		}
		if response.StatusCode() != http.StatusOK {
			return nil, export{}, apiclient.Fail(response.HTTPResponse, response.Body)
		}
		archive, err := zip.NewReader(bytes.NewReader(response.Body), int64(len(response.Body)))
		if err != nil {
			return nil, export{}, err
		}
		files, err := exportedFiles(archive)
		// TODO: the API still builds the whole export, images left out, for each page; page it on the server if a
		// big account's export gets slow.
		return nil, export{Files: page(files, limitOf(args.Limit), max(args.Offset, 0)), Total: len(files)}, err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "import_notes",
		Annotations: writes(false, false),
		Description: "Import markdown files as notes, their directories as folders, all or nothing: a folder name already taken refuses.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args importArgs) (*mcp.CallToolResult, apiclient.ImportnotesResult, error) {
		parentID, err := resolveFolder(ctx, client, args.Folder)
		if err != nil {
			return nil, apiclient.ImportnotesResult{}, err
		}
		dir, err := importDir(args.Files)
		if err != nil {
			return nil, apiclient.ImportnotesResult{}, err
		}
		var body bytes.Buffer
		archive := zip.NewWriter(&body)
		if err := archive.AddFS(dir); err != nil {
			return nil, apiclient.ImportnotesResult{}, err
		}
		if err := archive.Close(); err != nil {
			return nil, apiclient.ImportnotesResult{}, err
		}
		ctx, cancel := apiclient.Long(ctx)
		defer cancel()
		result, err := decode[apiclient.ImportnotesResult](client.PostApiImportWithBody(ctx, &apiclient.PostApiImportParams{Parent: &parentID}, "application/zip", &body))
		return nil, result, err
	})

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
		Description: "Create a snippet, or replace the one of the same name, the others kept. Answers the account's snippets.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args snippetArgs) (*mcp.CallToolResult, []apiclient.DomainSnippet, error) {
		me, err := editMe(ctx, client, func(me apiclient.DomainUser) (apiclient.EdituserInput, error) {
			snippets := slices.Clone(me.Snippets)
			saved := apiclient.DomainSnippet{Name: args.Name, Content: args.Content}
			if i := slices.IndexFunc(snippets, func(s apiclient.DomainSnippet) bool { return s.Name == args.Name }); i >= 0 {
				snippets[i] = saved
			} else {
				snippets = append(snippets, saved)
			}
			return apiclient.EdituserInput{Snippets: &snippets}, nil
		})
		return nil, append([]apiclient.DomainSnippet{}, me.Snippets...), err
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

	addTool(server, readOnly, &mcp.Tool{
		Name:        "saved_searches",
		Annotations: reads(),
		Description: "List the account's saved searches (the app's smart folders), or run one given its id or name: the notes it matches with their path, the first 50 and the total, unless limit says otherwise. Pro plan only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args savedSearchesArgs) (*mcp.CallToolResult, searches, error) {
		list, err := readSearches(ctx, client)
		if err != nil {
			return nil, searches{}, err
		}
		names, err := searchNames(ctx, client)
		if err != nil {
			return nil, searches{}, err
		}
		if args.Search == "" {
			out := make([]savedSearch, 0, len(list))
			for _, search := range list {
				out = append(out, names.view(search))
			}
			return nil, searches{Searches: out}, nil
		}
		i, err := findSearch(list, args.Search)
		if err != nil {
			return nil, searches{}, err
		}
		limit, offset := limitOf(args.Limit), max(args.Offset, 0)
		notes, total, err := paged[[]apiclient.RunsavedsearchMatch](client.GetApiSearchesId(ctx, list[i].Id, &apiclient.GetApiSearchesIdParams{Limit: &limit, Offset: &offset}))
		if err != nil {
			return nil, searches{}, err
		}
		view := names.view(list[i])
		return nil, searches{Search: &view, Notes: notes, Total: total}, nil
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "save_search",
		Annotations: writes(true, false),
		Description: "Create a saved search (a smart folder of the app), or replace one given its id or name, the others kept: its notes match every criterion given, though one tag_mode holds for all its tags (a AND (b OR c) cannot be said). Answers it as saved_searches lists it. Pro plan only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args saveSearchArgs) (*mcp.CallToolResult, savedSearch, error) {
		if _, err := readSearches(ctx, client); err != nil {
			return nil, savedSearch{}, err
		}
		search, err := searchOf(ctx, client, args)
		if err != nil {
			return nil, savedSearch{}, err
		}
		if args.Search == "" {
			search.Id = rand.Text()
		}
		_, err = editMe(ctx, client, func(me apiclient.DomainUser) (apiclient.EdituserInput, error) {
			list := slices.Clone(me.SavedSearches)
			if args.Search == "" {
				list = append(list, search)
			} else {
				i, err := findSearch(list, args.Search)
				if err != nil {
					return apiclient.EdituserInput{}, err
				}
				search.Id = list[i].Id
				list[i] = search
			}
			return apiclient.EdituserInput{SavedSearches: &list}, nil
		})
		if err != nil {
			return nil, savedSearch{}, err
		}
		names, err := searchNames(ctx, client)
		return nil, names.view(search), err
	})

	addTool(server, readOnly, &mcp.Tool{
		Name:        "delete_search",
		Annotations: writes(true, true),
		Description: "Remove a saved search by its id or name, the others kept; its notes stay as they are. Answers the account's saved searches. Pro plan only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args searchNameArgs) (*mcp.CallToolResult, []savedSearch, error) {
		if _, err := readSearches(ctx, client); err != nil {
			return nil, nil, err
		}
		me, err := editMe(ctx, client, func(me apiclient.DomainUser) (apiclient.EdituserInput, error) {
			i, err := findSearch(me.SavedSearches, args.Search)
			if err != nil {
				return apiclient.EdituserInput{}, err
			}
			list := slices.Delete(slices.Clone(me.SavedSearches), i, i+1)
			return apiclient.EdituserInput{SavedSearches: &list}, nil
		})
		if err != nil {
			return nil, nil, err
		}
		list := me.SavedSearches
		names, err := searchNames(ctx, client)
		out := make([]savedSearch, 0, len(list))
		for _, search := range list {
			out = append(out, names.view(search))
		}
		return nil, out, err
	})
}

// readSearches reads the account's saved searches, refusing an account whose plan lacks them (as PATCH /api/me does,
// though with a clearer error for an agent).
func readSearches(ctx context.Context, client *apiclient.ClientWithResponses) ([]apiclient.DomainSavedSearch, error) {
	me, err := decode[apiclient.DomainUser](client.GetApiMe(ctx))
	if err != nil {
		return nil, err
	}
	if !slices.Contains(me.Features, "savedSearches") {
		return nil, errNoPro
	}
	return append([]apiclient.DomainSavedSearch{}, me.SavedSearches...), nil
}

// findSearch gives the index of the saved search of id ref, or else of the one named ref: two of that name refuse.
func findSearch(list []apiclient.DomainSavedSearch, ref string) (int, error) {
	if i := slices.IndexFunc(list, func(s apiclient.DomainSavedSearch) bool { return s.Id == ref }); i >= 0 {
		return i, nil
	}
	found := -1
	for i, search := range list {
		if strings.EqualFold(search.Name, ref) {
			if found >= 0 {
				return 0, fmt.Errorf("several saved searches are named %q: give its id", ref)
			}
			found = i
		}
	}
	if found < 0 {
		return 0, fmt.Errorf("%w: %q", errNoSearch, ref)
	}
	return found, nil
}

// searchOf turns the criteria an agent gives into those the API stores: tags into their id, folders into their uuid,
// a day into its first or last instant in UTC, as the app's dialog does.
func searchOf(ctx context.Context, client *apiclient.ClientWithResponses, args saveSearchArgs) (apiclient.DomainSavedSearch, error) {
	search := apiclient.DomainSavedSearch{Name: strings.TrimSpace(args.Name), TagMode: &args.TagMode, FolderMode: &args.FolderMode, Text: &args.Text}
	if len(args.Tags) > 0 {
		all, err := decode[[]apiclient.DomainTag](client.GetApiTags(ctx))
		if err != nil {
			return search, err
		}
		ids := make([]string, 0, len(args.Tags))
		for _, name := range args.Tags {
			name = strings.TrimPrefix(name, "#")
			i := slices.IndexFunc(all, func(tag apiclient.DomainTag) bool { return strings.EqualFold(tag.Value, name) })
			if i < 0 {
				return search, fmt.Errorf("%w: %q", errNoTag, name)
			}
			ids = append(ids, all[i].Id)
		}
		search.TagIds = &ids
	}
	if len(args.Folders) > 0 {
		ids := make([]string, 0, len(args.Folders))
		for _, ref := range args.Folders {
			id, err := apiclient.Resolve(ctx, client, apiclient.FOLDER, ref)
			if err != nil {
				return search, fmt.Errorf("%s: %w", ref, err)
			}
			ids = append(ids, id)
		}
		search.FolderIds = &ids
	}
	for _, date := range []struct {
		value string
		end   bool
		to    **string
	}{{args.CreatedFrom, false, &search.CreatedFrom}, {args.CreatedTo, true, &search.CreatedTo}, {args.EditedFrom, false, &search.EditedFrom}, {args.EditedTo, true, &search.EditedTo}} {
		instant, err := instantOf(date.value, date.end)
		if err != nil {
			return search, err
		}
		*date.to = instant
	}
	return search, nil
}

// instantOf reads a day (its first instant, or its last when end) or an RFC 3339 instant; nil when empty.
func instantOf(value string, end bool) (*string, error) {
	if value == "" {
		return nil, nil
	}
	if day, err := time.Parse(time.DateOnly, value); err == nil {
		if end {
			day = day.Add(24*time.Hour - time.Millisecond)
		}
		instant := day.Format(time.RFC3339Nano)
		return &instant, nil
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return nil, fmt.Errorf("%q is neither a day (2026-01-31) nor an RFC 3339 instant", value)
	}
	return &value, nil
}

// names gives the tags and folders of saved searches by name and path, an id left as it is once its tag or folder is gone.
type names map[string]string

func searchNames(ctx context.Context, client *apiclient.ClientWithResponses) (names, error) {
	tags, err := decode[[]apiclient.DomainTag](client.GetApiTags(ctx))
	if err != nil {
		return nil, err
	}
	folders, err := decode[[]apiclient.ListfoldersFolder](client.GetApiFolders(ctx))
	if err != nil {
		return nil, err
	}
	out := names{}
	for _, tag := range tags {
		out[tag.Id] = tag.Value
	}
	for _, folder := range folders {
		out[folder.Id] = folder.Path
	}
	return out, nil
}

func (n names) of(ids *[]string) []string {
	out := make([]string, 0, len(apiclient.Deref(ids)))
	for _, id := range apiclient.Deref(ids) {
		out = append(out, cmp.Or(n[id], id))
	}
	return out
}

// view turns a saved search the API stores into what the agent reads.
func (n names) view(search apiclient.DomainSavedSearch) savedSearch {
	return savedSearch{
		ID:          search.Id,
		Name:        search.Name,
		Tags:        n.of(search.TagIds),
		TagMode:     apiclient.Deref(search.TagMode),
		Folders:     n.of(search.FolderIds),
		FolderMode:  apiclient.Deref(search.FolderMode),
		CreatedFrom: apiclient.Deref(search.CreatedFrom),
		CreatedTo:   apiclient.Deref(search.CreatedTo),
		EditedFrom:  apiclient.Deref(search.EditedFrom),
		EditedTo:    apiclient.Deref(search.EditedTo),
		Text:        apiclient.Deref(search.Text),
	}
}

// readSnippets reads the account's snippets, [] when it has none.
func readSnippets(ctx context.Context, client *apiclient.ClientWithResponses) ([]apiclient.DomainSnippet, error) {
	me, err := decode[apiclient.DomainUser](client.GetApiMe(ctx))
	return append([]apiclient.DomainSnippet{}, me.Snippets...), err
}

// errAccountStale is the text of edituser.ErrStale the API answers: the account changed since it was read.
const errAccountStale = "account settings changed elsewhere since they were read"

// editMe reads the account, has edit build the whole lists to save from it, and saves them through PATCH /api/me with
// the updated_at read: a write of the app or another agent in between is read again and edit run over it, once.
func editMe(ctx context.Context, client *apiclient.ClientWithResponses, edit func(apiclient.DomainUser) (apiclient.EdituserInput, error)) (apiclient.DomainUser, error) {
	for try := 0; ; try++ {
		me, err := decode[apiclient.DomainUser](client.GetApiMe(ctx))
		if err != nil {
			return me, err
		}
		input, err := edit(me)
		if err != nil {
			return me, err
		}
		input.UpdatedAt = &me.UpdatedAt
		me, err = decode[apiclient.DomainUser](client.PatchApiMe(ctx, input))
		if err == nil || err.Error() != errAccountStale || try > 0 {
			return me, err
		}
	}
}

// addTool adds tool to server, unless readOnly and the tool changes something: its annotations tell it.
func addTool[In, Out any](server *mcp.Server, readOnly bool, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	if readOnly && !tool.Annotations.ReadOnlyHint {
		return
	}
	mcp.AddTool(server, tool, func(ctx context.Context, request *mcp.CallToolRequest, args In) (*mcp.CallToolResult, Out, error) {
		result, out, err := handler(ctx, request, args)
		return result, out, kindHint(err)
	})
}

// kindHint names the tool for a path of the other kind, which the API only tells as text: without it an agent takes
// the path for wrong and searches for it.
func kindHint(err error) error {
	switch {
	case err == nil:
		return nil
	case strings.HasSuffix(err.Error(), "is a folder, not a note"):
		return fmt.Errorf("%w: use list_folder", err)
	case strings.HasSuffix(err.Error(), "is a note, not a folder"):
		return fmt.Errorf("%w: use read_note", err)
	}
	return err
}

// reads marks a tool that changes nothing, so a client runs it without asking. Every tool stays in the account's notes.
func reads() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)}
}

// writes marks a tool that changes the notes: destructive when it deletes or overwrites (the client asks first, though
// the trash and the history keep what it took), idempotent when calling it again with the same arguments changes nothing more.
func writes(destructive, idempotent bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{DestructiveHint: &destructive, IdempotentHint: idempotent, OpenWorldHint: new(false)}
}

// decode reads a JSON answer into the generated type of the route; any status but 2xx is the API's error.
func decode[T any](response *http.Response, err error) (T, error) {
	var out T
	if err != nil {
		return out, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return out, err
	}
	if response.StatusCode/100 != 2 {
		return out, apiclient.Fail(response, body)
	}
	return out, json.Unmarshal(body, &out)
}

// paged decodes a page of a list, and the X-Total-Count of the whole list.
func paged[T any](response *http.Response, err error) (T, int, error) {
	out, err := decode[T](response, err)
	if err != nil {
		return out, 0, err
	}
	total, err := strconv.Atoi(response.Header.Get("X-Total-Count"))
	return out, total, err
}

// page is domain.Page, which the CLI cannot import: limit items from offset, all of them from offset when limit is 0.
func page[T any](list []T, limit, offset int) []T {
	offset = min(offset, len(list))
	if limit == 0 || limit > len(list)-offset {
		return list[offset:]
	}
	return list[offset : offset+limit]
}

// fileContent shows an image or plays an audio to the agent; anything else comes as a resource of its type.
func fileContent(id, contentType string, data []byte) mcp.Content {
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return &mcp.ImageContent{Data: data, MIMEType: contentType}
	case strings.HasPrefix(contentType, "audio/"):
		return &mcp.AudioContent{Data: data, MIMEType: contentType}
	}
	return &mcp.EmbeddedResource{Resource: &mcp.ResourceContents{URI: fileURL(id), MIMEType: contentType, Blob: data}}
}

// citation is the markdown of a file in a note, under the name given (the stored one is sealed): an image shown, anything else linked.
func citation(file apiclient.DomainFile, name string) string {
	link := "[" + name + "](" + fileURL(file.Id) + ")"
	if strings.HasPrefix(file.ContentType, "image/") {
		return "!" + link
	}
	return link
}

// exportedFiles reads the zip of GET /api/export: the directories left out, implied by the paths of what they hold,
// a note with its text, and a file of filesDir named by the id read_file takes, its bytes left out.
func exportedFiles(archive *zip.Reader) ([]exported, error) {
	out := []exported{}
	for _, entry := range archive.File {
		name := entry.Name
		switch {
		case strings.HasSuffix(name, "/"):
		case path.Dir(name) == filesDir && path.Ext(name) != ".md":
			// exportnotes names it <id><ext> in its FilesDir; a note is always a .md.
			out = append(out, exported{Path: name, File: strings.TrimSuffix(path.Base(name), path.Ext(name))})
		default:
			file, err := entry.Open()
			if err != nil {
				return nil, err
			}
			content, err := io.ReadAll(file)
			file.Close()
			if err != nil {
				return nil, err
			}
			// GET /api/export gives a note's id as the entry's comment and its last change as its date.
			md := exported{Path: name, ID: entry.Comment, Content: string(content)}
			if entry.Comment != "" { // an entry without a date reads as 1979: only those with an id have one
				md.UpdatedAt = entry.Modified.UTC().Format(time.RFC3339)
			}
			out = append(out, md)
		}
	}
	return out, nil
}

// importDir makes a directory of the notes, refusing a path that would climb out of it: an agent sends text, not the
// bytes of an image.
func importDir(files []importFile) (fstest.MapFS, error) {
	dir := fstest.MapFS{}
	for _, file := range files {
		if !fs.ValidPath(file.Path) || !strings.EqualFold(path.Ext(file.Path), ".md") {
			return nil, fmt.Errorf("%w: %q", errBadPath, file.Path)
		}
		dir[file.Path] = &fstest.MapFile{Data: []byte(file.Content)}
	}
	return dir, nil
}

// one turns an item the API answers into what the agent reads.
func one(found apiclient.DomainItem) item {
	return item{ID: found.Id, Kind: string(found.Kind), Title: found.Title, CreatedAt: found.CreatedAt, UpdatedAt: apiclient.Deref(found.UpdatedAt)}
}

// made turns the items of an answer into what the agent reads, for a tool answering them.
func made(found []apiclient.DomainItem, err error) (*mcp.CallToolResult, []item, error) {
	if err != nil {
		return nil, nil, err
	}
	items := make([]item, 0, len(found))
	for _, found := range found {
		items = append(items, one(found))
	}
	return nil, items, nil
}

// resolveFolder keeps "" as "": the API reads it as the root.
func resolveFolder(ctx context.Context, client *apiclient.ClientWithResponses, ref string) (string, error) {
	if ref == "" {
		return "", nil
	}
	return apiclient.Resolve(ctx, client, apiclient.FOLDER, ref)
}

// preview cuts the content to its first maxChars characters and says how long it was, the note left whole when
// maxChars is not positive.
func preview(read note, maxChars int) note {
	if maxChars <= 0 {
		return read
	}
	content := []rune(read.Content)
	length := len(content)
	read.Length, read.Truncated = &length, length > maxChars
	if read.Truncated {
		read.Content = string(content[:maxChars])
	}
	return read
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
		UpdatedAt: apiclient.Deref(result.Item.UpdatedAt),
	}, nil
}
