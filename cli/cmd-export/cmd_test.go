package cmdexport

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/cyril-fl/tide/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/core/services/process/createfolder"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/spf13/cobra"
)

// The zip of the API lands as files; a folder given by its path exports what it holds, without itself.
func TestExport(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	folder := createfolder.Process(ctx, tx, createfolder.Input{Title: "Projects"})
	folder.Run()
	if folder.HasErr() {
		t.Fatal(folder.GetErr())
	}
	for _, input := range []createnote.Input{{Content: "# Root"}, {Content: "# Idea\n\ntext", ParentID: folder.GetResult().ID}} {
		note := createnote.Process(ctx, tx, input)
		note.Run()
		if note.HasErr() {
			t.Fatal(note.GetErr())
		}
	}
	command := &cobra.Command{}
	command.SetContext(ctx)

	dir := filepath.Join(t.TempDir(), "all")
	printed, err := apitest.Stdout(t, func() error { return Export(command, []string{dir}) })
	if err != nil || printed != "Successfully exported 2 note(s) to "+dir+"\n" {
		t.Fatalf("printed %q, err = %v; want 2 notes exported", printed, err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "Projects", "Idea.md")); err != nil || string(data) != "# Idea\n\ntext" {
		t.Errorf("Projects/Idea.md = %q, %v", data, err)
	}

	if _, err := apitest.Stdout(t, func() error { return Export(command, []string{dir}) }); !errors.Is(err, ErrNotEmpty) {
		t.Errorf("into a full directory: err = %v, want %v", err, ErrNotEmpty)
	}

	dir = filepath.Join(t.TempDir(), "folder")
	printed, err = apitest.Stdout(t, func() error { return Export(command, []string{dir, "/Projects"}) })
	if err != nil || printed != "Successfully exported 1 note(s) to "+dir+"\n" {
		t.Fatalf("printed %q, err = %v; want the folder's note exported", printed, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Idea.md")); err != nil {
		t.Errorf("Idea.md at the top: %v", err)
	}

	if _, err := apitest.Stdout(t, func() error { return Export(command, []string{t.TempDir(), "/Nowhere"}) }); err == nil {
		t.Error("an unknown folder exported")
	}
}
