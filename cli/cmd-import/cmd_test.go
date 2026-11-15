package cmdimport

import (
	"os"
	"path/filepath"
	"testing"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	"github.com/cyril-fl/tide/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/config"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/process/createfolder"
	"github.com/spf13/cobra"
)

// The directory goes up as a zip: its notes, its empty directories as folders, never its hidden entries; --parent
// takes a path.
func TestImport(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	archive := createfolder.Process(ctx, tx, createfolder.Input{Title: "Archive"})
	archive.Run()
	if archive.HasErr() {
		t.Fatal(archive.GetErr())
	}
	dir := t.TempDir()
	for name, content := range map[string]string{"Root.md": "# Root", "Projects/Idea.md": "# Idea", ".obsidian/Hidden.md": "# Hidden"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "Empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	command := &cobra.Command{}
	plumcliutilsargs.Makeflags(command, config.Current.Commands.Import.Flags)
	command.SetContext(ctx)
	if err := command.Flags().Set("parent", "/Archive"); err != nil {
		t.Fatal(err)
	}
	printed, err := apitest.Stdout(t, func() error { return Import(command, []string{dir}) })
	if want := "Successfully imported 2 note(s) and 2 folder(s)\n"; err != nil || printed != want {
		t.Fatalf("printed %q, err = %v; want %q", printed, err, want)
	}
	count, err := tx.NewSelect().Model((*domain.Item)(nil)).Where("parent_id = ? AND user_id = ?", archive.GetResult().ID, domain.UserID(ctx)).Count(ctx)
	if err != nil || count != 3 {
		t.Errorf("items in Archive = %d, %v; want Root, Projects and Empty", count, err)
	}

	// A folder name taken in the same place stops the import: the API's message.
	if _, err := apitest.Stdout(t, func() error { return Import(command, []string{dir}) }); err == nil {
		t.Error("the same directory imported twice in the same place")
	}
	if _, err := apitest.Stdout(t, func() error { return Import(command, []string{filepath.Join(dir, "Root.md")}) }); err == nil {
		t.Error("a file imported as a directory")
	}
}
