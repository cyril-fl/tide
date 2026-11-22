package cmdmcp

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/process/exportnotes"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
	var files struct{ Files []map[string]any }
	call(t, agent, "read_file", map[string]any{}, &files)
	if len(files.Files) != 1 {
		t.Errorf("read_file list = %v", files)
	}

	var imported struct{ Folders, Notes int }
	call(t, agent, "import_notes", map[string]any{"files": []importFile{{Path: "Trip/Day.md", Content: "# Day\n" + file.Markdown}}}, &imported)
	if imported.Folders != 1 || imported.Notes != 1 {
		t.Errorf("import_notes = %+v", imported)
	}
	var out export
	call(t, agent, "export_notes", map[string]any{"folder": "/Trip"}, &out)
	if out.Total != 2 || len(out.Files) != 2 || out.Files[0].Path != "Day.md" || !strings.Contains(out.Files[0].Content, "# Day") || out.Files[1].File != file.ID {
		t.Errorf("export_notes = %+v", out)
	}
	// The path is not enough to read a note back: its id is.
	var day batchNote
	day = readOne(t, agent, out.Files[0].ID)
	if day.Path != "/Trip/Day" || out.Files[0].UpdatedAt == "" {
		t.Errorf("export_notes id = %+v, read_notes = %+v", out.Files[0], day)
	}
	call(t, agent, "export_notes", map[string]any{"folder": "/Trip", "offset": 1}, &out)
	if out.Total != 2 || len(out.Files) != 1 || out.Files[0].File != file.ID {
		t.Errorf("export_notes, offset 1 = %+v", out)
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
// named like filesDir stays a note.
func TestExportedFiles(t *testing.T) {
	id := "0b6f8e2a-1c3d-4e5f-8a9b-0c1d2e3f4a5b"
	var body bytes.Buffer
	archive := zip.NewWriter(&body)
	modified := time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC)
	for name, content := range map[string]string{"Folder/": "", "Folder/Note.md": "text", filesDir + "/" + id + ".png": "bytes", filesDir + "/Other.md": "note"} {
		header := &zip.FileHeader{Name: name}
		if name == "Folder/Note.md" {
			header.Comment, header.Modified = "note-id", modified
		}
		entry, _ := archive.CreateHeader(header)
		entry.Write([]byte(content))
	}
	archive.Close()
	reader, err := zip.NewReader(bytes.NewReader(body.Bytes()), int64(body.Len()))
	if err != nil {
		t.Fatal(err)
	}

	got, err := exportedFiles(reader)
	slices.SortFunc(got, func(a, b exported) int { return strings.Compare(a.Path, b.Path) })
	want := []exported{{Path: "Folder/Note.md", ID: "note-id", UpdatedAt: "2026-11-02T09:30:00Z", Content: "text"}, {Path: filesDir + "/" + id + ".png", File: id}, {Path: filesDir + "/Other.md", Content: "note"}}
	slices.SortFunc(want, func(a, b exported) int { return strings.Compare(a.Path, b.Path) })
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("exportedFiles = %v, %v, want %v", got, err, want)
	}
}

// The CLI has no kernel to import them from: they must stay those of the server.
func TestServerNames(t *testing.T) {
	if filesDir != exportnotes.FilesDir || fileURL("id") != domain.FileURL("id") {
		t.Errorf("filesDir %q, fileURL %q; want %q, %q", filesDir, fileURL("id"), exportnotes.FilesDir, domain.FileURL("id"))
	}
}
