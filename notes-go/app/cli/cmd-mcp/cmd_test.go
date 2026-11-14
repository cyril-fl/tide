package cmdmcp

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient"
	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient/apitest"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/exportnotes"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connect serves the tools on the API of apitest to an MCP client in memory.
func connect(t *testing.T) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	apitest.Serve(t)
	client, err := apiclient.Read().Client()
	if err != nil {
		t.Fatal(err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "notes"}, nil)
	AddTools(server, client)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	session, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	agent, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { agent.Close() })
	return agent
}

// call runs a tool and decodes its structured answer into out, failing the test on an error.
func call(t *testing.T, agent *mcp.ClientSession, name string, args map[string]any, out any) *mcp.CallToolResult {
	t.Helper()
	result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s: %v", name, result.Content[0].(*mcp.TextContent).Text)
	}
	if out != nil {
		data, _ := json.Marshal(result.StructuredContent)
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("%s: %v: %s", name, err, data)
		}
	}
	return result
}

// TestAddTools catches a schema the SDK cannot infer from the arguments: the tools would be
// announced broken, and only an agent calling them would find out.
func TestAddTools(t *testing.T) {
	agent := connect(t)
	result, err := agent.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
		if tool.InputSchema == nil || tool.Description == "" {
			t.Errorf("tool %q: schema %v, description %q", tool.Name, tool.InputSchema, tool.Description)
		}
	}
	for _, want := range []string{"search_notes", "recent_notes", "read_note", "list_folder", "create_note", "edit_note", "create_folder", "rename_folder", "move_item", "delete_note", "delete_folder", "list_trash", "restore_item", "tags", "rename_tag", "delete_tag", "note_history", "note_links", "read_file", "attach_file", "export_notes", "import_notes"} {
		if !slices.Contains(names, want) {
			t.Errorf("tools = %v, missing %q", names, want)
		}
	}
}

// TestNotes writes, reads, moves, trashes and restores through the API, by path as an agent does.
func TestNotes(t *testing.T) {
	agent := connect(t)

	var folder, inbox item
	call(t, agent, "create_folder", map[string]any{"title": "Work"}, &folder)
	call(t, agent, "create_folder", map[string]any{"title": "Inbox"}, &inbox)
	call(t, agent, "rename_folder", map[string]any{"folder": "/Work", "title": "Projects"}, &folder)
	if folder.Title != "Projects" {
		t.Errorf("rename_folder = %+v", folder)
	}

	var created note
	call(t, agent, "create_note", map[string]any{"content": "# Plan\nship it #go", "folder": "/Projects", "tags": []string{"mcp"}}, &created)
	if created.Path != "/Projects/Plan" || !slices.Equal(slices.Sorted(slices.Values(created.Tags)), []string{"go", "mcp"}) {
		t.Errorf("create_note = %+v", created)
	}

	// The #tags of the content stay, the tags given are added.
	var edited note
	call(t, agent, "edit_note", map[string]any{"note": "/Projects/Plan", "content": "# Plan\nshipped #go", "tags": []string{"done"}}, &edited)
	if !strings.Contains(edited.Content, "shipped") || !slices.Equal(slices.Sorted(slices.Values(edited.Tags)), []string{"done", "go"}) {
		t.Errorf("edit_note = %+v", edited)
	}
	var read note
	call(t, agent, "read_note", map[string]any{"note": created.ID}, &read)
	if read.Content != edited.Content {
		t.Errorf("read_note = %+v", read)
	}

	var matches []map[string]any
	call(t, agent, "search_notes", map[string]any{"text": "shipped"}, &matches)
	var recent []map[string]any
	call(t, agent, "recent_notes", map[string]any{}, &recent)
	if len(matches) != 1 || len(recent) != 1 {
		t.Errorf("search_notes = %v, recent_notes = %v", matches, recent)
	}

	var moved, listed []item
	call(t, agent, "move_item", map[string]any{"item": "/Projects/Plan", "folder": "/Inbox"}, &moved)
	call(t, agent, "list_folder", map[string]any{"folder": "/Inbox"}, &listed)
	if len(moved) != 1 || len(listed) != 1 || listed[0].ID != created.ID {
		t.Errorf("move_item = %+v, list_folder = %+v", moved, listed)
	}

	// A folder goes to the trash with its content, and comes back with it.
	var removed, restored []item
	call(t, agent, "delete_folder", map[string]any{"folder": "/Inbox"}, &removed)
	if len(removed) != 2 {
		t.Errorf("delete_folder = %+v", removed)
	}
	var trash []map[string]any
	call(t, agent, "list_trash", map[string]any{}, &trash)
	call(t, agent, "restore_item", map[string]any{"item": inbox.ID}, &restored)
	if len(trash) != 1 || len(restored) != 2 {
		t.Errorf("list_trash = %v, restore_item = %+v", trash, restored)
	}
	call(t, agent, "delete_note", map[string]any{"note": "/Inbox/Plan"}, &removed)
	if len(removed) != 1 || removed[0].ID != created.ID {
		t.Errorf("delete_note = %+v", removed)
	}

	// A folder's path given to delete_note is refused, not its subtree trashed.
	result, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: "delete_note", Arguments: map[string]any{"note": "/Projects"}})
	if err != nil || !result.IsError {
		t.Errorf("delete_note of a folder = %+v, %v", result, err)
	}
}

// TestTagsHistoryLinks reads what surrounds a note: its tags, its versions, its links.
func TestTagsHistoryLinks(t *testing.T) {
	agent := connect(t)

	var target, source note
	call(t, agent, "create_note", map[string]any{"content": "# Target\n#go"}, &target)
	call(t, agent, "create_note", map[string]any{"content": "# Source\nsee [[Target]] and [[Nowhere]] #go"}, &source)

	var counted, tagged tags
	call(t, agent, "tags", map[string]any{}, &counted)
	call(t, agent, "tags", map[string]any{"tag": "go"}, &tagged)
	if len(counted.Tags) != 1 || counted.Tags[0].Notes != 2 || len(tagged.Notes) != 2 {
		t.Errorf("tags = %+v, %+v", counted, tagged)
	}

	var linked, broken links
	call(t, agent, "note_links", map[string]any{"note": "/Target"}, &linked)
	call(t, agent, "note_links", map[string]any{}, &broken)
	if len(linked.Backlinks) != 1 || len(linked.Neighbors) != 1 || len(broken.Broken) != 1 {
		t.Errorf("note_links = %+v, broken = %+v", linked, broken)
	}

	// The first save of a session keeps the text it replaces.
	call(t, agent, "edit_note", map[string]any{"note": "/Target", "content": "# Target\nnew #go"}, nil)
	var versions, version history
	call(t, agent, "note_history", map[string]any{"note": "/Target"}, &versions)
	if len(versions.Versions) != 1 {
		t.Fatalf("note_history = %+v", versions)
	}
	call(t, agent, "note_history", map[string]any{"note": "/Target", "version": versions.Versions[0].ID}, &version)
	if version.Version == nil || string(version.Version.Content) != target.Content {
		t.Errorf("note_history version = %+v", version)
	}

	var renamed struct {
		Value string `json:"value"`
	}
	call(t, agent, "rename_tag", map[string]any{"tag": "go", "to": "golang"}, &renamed)
	var deleted []map[string]any
	call(t, agent, "delete_tag", map[string]any{"tag": "golang"}, &deleted)
	if renamed.Value != "golang" || len(deleted) != 1 {
		t.Errorf("rename_tag = %+v, delete_tag = %v", renamed, deleted)
	}
}

// TestFiles attaches an image, reads it back, then exports and imports notes.
func TestFiles(t *testing.T) {
	agent := connect(t)

	// The smallest GIF, which the server recognizes from its bytes.
	gif := []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff!\xf9\x04\x01\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;")
	var file attached
	call(t, agent, "attach_file", map[string]any{"name": "dot.gif", "data": base64.StdEncoding.EncodeToString(gif)}, &file)
	if file.Markdown != "![dot.gif](/api/files/"+file.ID+")" {
		t.Errorf("attach_file = %+v", file)
	}
	result := call(t, agent, "read_file", map[string]any{"file": file.ID}, nil)
	if image, ok := result.Content[0].(*mcp.ImageContent); !ok || !bytes.Equal(image.Data, gif) || image.MIMEType != "image/gif" {
		t.Errorf("read_file = %+v", result.Content[0])
	}
	var files []map[string]any
	call(t, agent, "read_file", map[string]any{}, &files)
	if len(files) != 1 {
		t.Errorf("read_file list = %v", files)
	}

	var imported struct{ Folders, Notes int }
	call(t, agent, "import_notes", map[string]any{"files": []importFile{{Path: "Trip/Day.md", Content: "# Day\n" + file.Markdown}}}, &imported)
	if imported.Folders != 1 || imported.Notes != 1 {
		t.Errorf("import_notes = %+v", imported)
	}
	var out []exported
	call(t, agent, "export_notes", map[string]any{"folder": "/Trip"}, &out)
	if len(out) != 2 || out[0].Path != "Day.md" || !strings.Contains(out[0].Content, "# Day") || out[1].File != file.ID {
		t.Errorf("export_notes = %+v", out)
	}

	// A path climbing out of the import is refused before anything is sent.
	bad, err := agent.CallTool(context.Background(), &mcp.CallToolParams{Name: "import_notes", Arguments: map[string]any{"files": []importFile{{Path: "../x.md", Content: "x"}}}})
	if err != nil || !bad.IsError {
		t.Errorf("import_notes ../x.md = %+v, %v", bad, err)
	}
}

// TestImportDir refuses what would climb out of the import or is no note.
func TestImportDir(t *testing.T) {
	dir, err := importDir([]importFile{{Path: "Folder/Note.md", Content: "# Note"}})
	if err != nil || string(dir["Folder/Note.md"].Data) != "# Note" {
		t.Fatalf("importDir = %v, %v", dir, err)
	}
	for _, bad := range []string{"../Note.md", "/Note.md", "Folder/../Note.md", "image.png", ""} {
		if _, err := importDir([]importFile{{Path: bad}}); !errors.Is(err, errBadPath) {
			t.Errorf("importDir(%q) = %v, want errBadPath", bad, err)
		}
	}
}

// TestExportedFiles drops the directories, keeps a note's text and names a file by its id; a note of a folder
// named like exportnotes.FilesDir stays a note.
func TestExportedFiles(t *testing.T) {
	id := "0b6f8e2a-1c3d-4e5f-8a9b-0c1d2e3f4a5b"
	var body bytes.Buffer
	archive := zip.NewWriter(&body)
	for name, content := range map[string]string{"Folder/": "", "Folder/Note.md": "text", exportnotes.FilesDir + "/" + id + ".png": "bytes", exportnotes.FilesDir + "/Other.md": "note"} {
		entry, _ := archive.Create(name)
		entry.Write([]byte(content))
	}
	archive.Close()
	reader, err := zip.NewReader(bytes.NewReader(body.Bytes()), int64(body.Len()))
	if err != nil {
		t.Fatal(err)
	}

	got, err := exportedFiles(reader)
	slices.SortFunc(got, func(a, b exported) int { return strings.Compare(a.Path, b.Path) })
	want := []exported{{Path: "Folder/Note.md", Content: "text"}, {Path: exportnotes.FilesDir + "/" + id + ".png", File: id}, {Path: exportnotes.FilesDir + "/Other.md", Content: "note"}}
	slices.SortFunc(want, func(a, b exported) int { return strings.Compare(a.Path, b.Path) })
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("exportedFiles = %v, %v, want %v", got, err, want)
	}
}
