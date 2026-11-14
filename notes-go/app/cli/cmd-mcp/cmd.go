// Package cmdmcp serves the account's notes to an agent over MCP, a client of the API like the rest of the CLI:
// the rules (titles, tags, links, trash, encryption) are those of the server.
package cmdmcp

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"testing/fstest"

	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient"
	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/hashtag"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/counttags"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/exportnotes"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/importnotes"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/listbrokenlinks"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/listrecent"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/listtrash"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/readitem"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/readneighbors"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/readtag"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/searchnotes"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

var errBadPath = errors.New("not the relative path of a .md file")

// item is a note or a folder as an agent needs it: no timestamps, no model, nothing to read twice.
type item struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
}

// note is what the read and the writes answer, the content left out when the agent did not ask for it.
type note struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Path     string   `json:"path"`
	Content  string   `json:"content,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Readonly bool     `json:"readonly,omitempty"`
}

// searchLimit is how many notes search_notes gives when the agent names no limit: a common word matches tens of
// thousands of them, megabytes an agent cannot read.
const searchLimit = 50

type searchArgs struct {
	Text  string `json:"text" jsonschema:"the text to find in the title or the content of the notes"`
	Limit *int   `json:"limit,omitempty" jsonschema:"how many notes at most, 50 when left out, 0 for all of them"`
}

type noteArgs struct {
	Note string `json:"note" jsonschema:"the note: its uuid, or its path from the root (/Folder/Note)"`
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
	Tag string `json:"tag,omitempty" jsonschema:"the tag, without the #; the list of tags when empty"`
}

// tags is the account's tags with their count, or one tag with the notes carrying it.
type tags struct {
	Tags  []counttags.TagCount `json:"tags,omitempty"`
	Tag   *domain.Tag          `json:"tag,omitempty"`
	Notes []readtag.Note       `json:"notes,omitempty"`
}

type renameTagArgs struct {
	Tag string `json:"tag" jsonschema:"the tag to rename, without the #"`
	To  string `json:"to" jsonschema:"its new name, without the #"`
}

type historyArgs struct {
	Note    string `json:"note" jsonschema:"the note: its uuid, or its path from the root (/Folder/Note)"`
	Version string `json:"version,omitempty" jsonschema:"the uuid of a version to read, as the list gives it; the list of versions when empty"`
}

// history is the list of versions, their texts left out, or the one version asked with its text.
type history struct {
	Versions []domain.NoteVersion `json:"versions,omitempty"`
	Version  *domain.NoteVersion  `json:"version,omitempty"`
}

type linksArgs struct {
	Note string `json:"note,omitempty" jsonschema:"the note: its uuid, or its path from the root (/Folder/Note); the broken links of every note when empty"`
}

// links joins a note to the others, the note itself left out (read_note reads it), or lists the broken links of every note.
type links struct {
	Links     []readneighbors.Linked   `json:"links,omitempty"`
	Backlinks []readneighbors.Linked   `json:"backlinks,omitempty"`
	Neighbors []readneighbors.Neighbor `json:"neighbors,omitempty"`
	Broken    []listbrokenlinks.Link   `json:"broken,omitempty"`
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
}

// exported is a note as a markdown file, or a file it cites, whose bytes read_file gives.
type exported struct {
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
	File    string `json:"file,omitempty"`
}

type importArgs struct {
	Files  []importFile `json:"files" jsonschema:"the markdown files; their directories become folders"`
	Folder string       `json:"folder,omitempty" jsonschema:"the folder to import into: its uuid, or its path from the root (/Folder); the root itself when empty"`
}

type importFile struct {
	Path    string `json:"path" jsonschema:"the file path, relative (Folder/Note.md)"`
	Content string `json:"content" jsonschema:"the markdown content"`
}

// Serve speaks MCP on stdin and stdout: nothing else may be printed there.
func Serve(cobra *cobra.Command, _ []string) error {
	// The account is the session's, as for every client of the API: the handlers act for it.
	client, err := apiclient.Read().Client()
	if err != nil {
		return err
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "notes", Title: "Notes", Version: cobra.Root().Version}, nil)
	AddTools(server, client)
	return server.Run(cobra.Context(), &mcp.StdioTransport{})
}

// AddTools gives the server the tools acting through client, for the account of its session.
func AddTools(server *mcp.Server, client *apiclient.ClientWithResponses) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_notes",
		Description: "Find the notes whose title or content contains a text, with the line where it was found: the first 50, title matches first, unless limit says otherwise.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args searchArgs) (*mcp.CallToolResult, []searchnotes.Match, error) {
		limit := searchLimit
		if args.Limit != nil {
			limit = max(*args.Limit, 0)
		}
		matches, err := decode[[]searchnotes.Match](client.GetApiSearch(ctx, &apiclient.GetApiSearchParams{Q: args.Text, Limit: &limit}))
		return nil, matches, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "recent_notes",
		Description: "List the notes changed most recently, newest first.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []listrecent.Recent, error) {
		recent, err := decode[[]listrecent.Recent](client.GetApiRecent(ctx))
		return nil, recent, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "read_note",
		Description: "Read a note: its content, its path and its tags.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args noteArgs) (*mcp.CallToolResult, note, error) {
		id, err := apiclient.Resolve(ctx, client, apiclient.NOTE, args.Note)
		if err != nil {
			return nil, note{}, err
		}
		read, err := readNote(decode[readitem.Result](client.GetApiNotesId(ctx, id)))
		return nil, read, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_folder",
		Description: "List the notes and folders directly inside a folder.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args folderArgs) (*mcp.CallToolResult, []item, error) {
		parentID, err := resolveFolder(ctx, client, args.Folder)
		if err != nil {
			return nil, nil, err
		}
		return made(decode[[]domain.Item](client.GetApiItems(ctx, &apiclient.GetApiItemsParams{Parent: &parentID})))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_note",
		Description: "Create a note from markdown content.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args createArgs) (*mcp.CallToolResult, note, error) {
		parentID, err := resolveFolder(ctx, client, args.Folder)
		if err != nil {
			return nil, note{}, err
		}
		created, err := decode[domain.Item](client.PostApiNotes(ctx, apiclient.CreatenoteInput{Content: args.Content, ParentId: &parentID, Tags: &args.Tags}))
		if err != nil {
			return nil, note{}, err
		}
		// Read back: the API answers the item alone, without the path nor the tags it parsed.
		read, err := readNote(decode[readitem.Result](client.GetApiNotesId(ctx, created.ID)))
		return nil, read, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "edit_note",
		Description: "Replace the content of a note. Read it first: nothing is kept of the old content. A locked note refuses.",
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
		edited, err := readNote(decode[readitem.Result](client.PatchApiNotesId(ctx, id, input)))
		return nil, edited, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_folder",
		Description: "Create a folder.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args createFolderArgs) (*mcp.CallToolResult, item, error) {
		parentID, err := resolveFolder(ctx, client, args.Folder)
		if err != nil {
			return nil, item{}, err
		}
		created, err := decode[domain.Item](client.PostApiFolders(ctx, apiclient.CreatefolderInput{Title: args.Title, ParentId: &parentID}))
		return nil, one(created), err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rename_folder",
		Description: "Change the title of a folder.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args renameFolderArgs) (*mcp.CallToolResult, item, error) {
		id, err := apiclient.Resolve(ctx, client, apiclient.FOLDER, args.Folder)
		if err != nil {
			return nil, item{}, err
		}
		renamed, err := decode[domain.Item](client.PatchApiFoldersId(ctx, id, apiclient.EditfolderBody{Title: &args.Title}))
		return nil, one(renamed), err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "move_item",
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
		return made(decode[[]domain.Item](client.PostApiMove(ctx, apiclient.MoveitemsInput{Ids: []string{id}, ParentId: &parentID})))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_note",
		Description: "Move a note to the trash, where restore_item can take it back.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args noteArgs) (*mcp.CallToolResult, []item, error) {
		id, err := apiclient.Resolve(ctx, client, apiclient.NOTE, args.Note)
		if err != nil {
			return nil, nil, err
		}
		return made(decode[[]domain.Item](client.DeleteApiNotesId(ctx, id)))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_folder",
		Description: "Move a folder and everything inside it to the trash, where restore_item can take it back.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args folderArgs) (*mcp.CallToolResult, []item, error) {
		id, err := apiclient.Resolve(ctx, client, apiclient.FOLDER, args.Folder)
		if err != nil {
			return nil, nil, err
		}
		return made(decode[[]domain.Item](client.DeleteApiFoldersId(ctx, id)))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_trash",
		Description: "List what is in the trash, last removed first, with the uuid restore_item takes.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []listtrash.Trashed, error) {
		trash, err := decode[[]listtrash.Trashed](client.GetApiTrash(ctx))
		return nil, trash, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "restore_item",
		Description: "Take a note or a folder out of the trash, with everything removed at the same time as it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args trashedArgs) (*mcp.CallToolResult, []item, error) {
		// No resolve: a trashed item has no path to look up.
		return made(decode[[]domain.Item](client.PostApiRestore(ctx, apiclient.RestoreBody{Ids: []string{args.Item}})))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "tags",
		Description: "List the notes carrying a tag, with their path. Without a tag, the account's tags, most used first. Tags of a note are set by create_note and edit_note.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args tagsArgs) (*mcp.CallToolResult, tags, error) {
		if args.Tag == "" {
			counts, err := decode[[]counttags.TagCount](client.GetApiGraph(ctx))
			return nil, tags{Tags: counts}, err
		}
		result, err := decode[readtag.Result](client.GetApiTagsValue(ctx, args.Tag, nil))
		if err != nil {
			return nil, tags{}, err
		}
		return nil, tags{Tag: &result.Tag, Notes: result.Notes}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rename_tag",
		Description: "Rename a tag on every note carrying it, in their text too.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args renameTagArgs) (*mcp.CallToolResult, domain.Tag, error) {
		renamed, err := decode[domain.Tag](client.PatchApiTagsValue(ctx, args.Tag, apiclient.EdittagBody{Value: args.To}))
		return nil, renamed, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_tag",
		Description: "Remove a tag from every note carrying it, in their text too. A note holding nothing but the tag goes to the trash.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args tagArgs) (*mcp.CallToolResult, []domain.Tag, error) {
		deleted, err := decode[[]domain.Tag](client.DeleteApiTagsValue(ctx, args.Tag))
		return nil, deleted, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "note_history",
		Description: "List the earlier texts of a note, newest first, or read one: what edit_note replaced.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args historyArgs) (*mcp.CallToolResult, history, error) {
		id, err := apiclient.Resolve(ctx, client, apiclient.NOTE, args.Note)
		if err != nil {
			return nil, history{}, err
		}
		if args.Version == "" {
			versions, err := decode[[]domain.NoteVersion](client.GetApiNotesIdVersions(ctx, id))
			return nil, history{Versions: versions}, err
		}
		version, err := decode[domain.NoteVersion](client.GetApiNotesIdVersionsVersion(ctx, id, args.Version))
		if err != nil {
			return nil, history{}, err
		}
		return nil, history{Version: &version}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "note_links",
		Description: "List the notes a note links to, those linking to it, and those sharing its tags. Without a note, the broken links of every note.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args linksArgs) (*mcp.CallToolResult, links, error) {
		if args.Note == "" {
			broken, err := decode[[]listbrokenlinks.Link](client.GetApiLinksBroken(ctx))
			return nil, links{Broken: broken}, err
		}
		id, err := apiclient.Resolve(ctx, client, apiclient.NOTE, args.Note)
		if err != nil {
			return nil, links{}, err
		}
		result, err := decode[readneighbors.Result](client.GetApiNotesIdNeighbors(ctx, id))
		if err != nil {
			return nil, links{}, err
		}
		return nil, links{Links: result.Links, Backlinks: result.Backlinks, Neighbors: result.Neighbors}, nil
	})

	// Out is any: a read answers the file as content (an image the agent sees), not as JSON.
	mcp.AddTool(server, &mcp.Tool{
		Name:        "read_file",
		Description: "Read an image or attachment a note cites. Without a file, list the account's files, largest first.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args fileArgs) (*mcp.CallToolResult, any, error) {
		if args.File == "" {
			files, err := decode[[]domain.File](client.GetApiFiles(ctx))
			return nil, files, err
		}
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

	mcp.AddTool(server, &mcp.Tool{
		Name:        "attach_file",
		Description: "Store an image or attachment and answer the markdown citing it, to put in a note with create_note or edit_note.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args attachArgs) (*mcp.CallToolResult, attached, error) {
		data, err := base64.StdEncoding.DecodeString(args.Data)
		if err != nil {
			return nil, attached{}, fmt.Errorf("data is not base64: %w", err)
		}
		// The server reads the type from the bytes, not from this header.
		file, err := decode[domain.File](client.PostApiFilesWithBody(ctx, &apiclient.PostApiFilesParams{Name: &args.Name}, "application/octet-stream", bytes.NewReader(data)))
		if err != nil {
			return nil, attached{}, err
		}
		return nil, attached{ID: file.ID, Markdown: citation(file, args.Name)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "export_notes",
		Description: "Export notes as markdown files, [[links]] written back, their folders as directories.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args exportArgs) (*mcp.CallToolResult, []exported, error) {
		folderID, err := resolveFolder(ctx, client, args.Folder)
		if err != nil {
			return nil, nil, err
		}
		ids := make([]string, 0, len(args.Items))
		for _, ref := range args.Items {
			id, err := apiclient.Resolve(ctx, client, "", ref)
			if err != nil {
				return nil, nil, err
			}
			ids = append(ids, id)
		}

		// TODO: the zip carries the bytes of every image the notes cite, only to be named here; an export without
		// them when an agent exports a big account.
		params := apiclient.GetApiExportParams{Parent: &folderID}
		if len(ids) > 0 {
			params.Id = &ids // An empty list would still send id=.
		}
		response, err := client.GetApiExportWithResponse(ctx, &params)
		if err != nil {
			return nil, nil, err
		}
		if response.StatusCode() != http.StatusOK {
			return nil, nil, apiclient.Fail(response.HTTPResponse, response.Body)
		}
		archive, err := zip.NewReader(bytes.NewReader(response.Body), int64(len(response.Body)))
		if err != nil {
			return nil, nil, err
		}
		files, err := exportedFiles(archive)
		return nil, files, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "import_notes",
		Description: "Import markdown files as notes, their directories as folders, all or nothing: a folder name already taken refuses.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args importArgs) (*mcp.CallToolResult, importnotes.Result, error) {
		parentID, err := resolveFolder(ctx, client, args.Folder)
		if err != nil {
			return nil, importnotes.Result{}, err
		}
		dir, err := importDir(args.Files)
		if err != nil {
			return nil, importnotes.Result{}, err
		}
		var body bytes.Buffer
		archive := zip.NewWriter(&body)
		if err := archive.AddFS(dir); err != nil {
			return nil, importnotes.Result{}, err
		}
		if err := archive.Close(); err != nil {
			return nil, importnotes.Result{}, err
		}
		result, err := decode[importnotes.Result](client.PostApiImportWithBody(ctx, &apiclient.PostApiImportParams{Parent: &parentID}, "application/zip", &body))
		return nil, result, err
	})
}

// decode reads a JSON answer into the type of the process behind the route, the result shapes staying those of the
// processes; any status but 2xx is the API's error.
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

// fileContent shows an image or plays an audio to the agent; anything else comes as a resource of its type.
func fileContent(id, contentType string, data []byte) mcp.Content {
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return &mcp.ImageContent{Data: data, MIMEType: contentType}
	case strings.HasPrefix(contentType, "audio/"):
		return &mcp.AudioContent{Data: data, MIMEType: contentType}
	}
	return &mcp.EmbeddedResource{Resource: &mcp.ResourceContents{URI: domain.FileURL(id), MIMEType: contentType, Blob: data}}
}

// citation is the markdown of a file in a note, under the name given (the stored one is sealed): an image shown, anything else linked.
func citation(file domain.File, name string) string {
	link := "[" + name + "](" + domain.FileURL(file.ID) + ")"
	if strings.HasPrefix(file.ContentType, "image/") {
		return "!" + link
	}
	return link
}

// exportedFiles reads the zip of GET /api/export: the directories left out, implied by the paths of what they hold,
// a note with its text, and a file of exportnotes.FilesDir named by the id read_file takes, its bytes left out.
func exportedFiles(archive *zip.Reader) ([]exported, error) {
	out := []exported{}
	for _, entry := range archive.File {
		name := entry.Name
		switch {
		case strings.HasSuffix(name, "/"):
		case path.Dir(name) == exportnotes.FilesDir && path.Ext(name) != ".md":
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
			out = append(out, exported{Path: name, Content: string(content)})
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
func one(found domain.Item) item {
	return item{ID: found.ID, Kind: string(found.Kind), Title: string(found.Title)}
}

// made turns the items of an answer into what the agent reads, for a tool answering them.
func made(found []domain.Item, err error) (*mcp.CallToolResult, []item, error) {
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

// readNote turns the note the API answers into what the agent reads.
func readNote(result readitem.Result, err error) (note, error) {
	if err != nil {
		return note{}, err
	}
	return note{
		ID:       result.Item.ID,
		Title:    string(result.Item.Title),
		Path:     result.Path,
		Content:  result.Content,
		Tags:     result.Tags,
		Readonly: result.Readonly,
	}, nil
}
