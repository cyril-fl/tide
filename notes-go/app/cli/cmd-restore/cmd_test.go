package cmdrestore

import (
	"testing"

	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient/apitest"
	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/createfolder"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/createnote"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/deleteitems"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/listtrash"
	"github.com/spf13/cobra"
)

// A folder comes back through the API with the note removed with it.
func TestRestore(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	folder := createfolder.Process(ctx, tx, createfolder.Input{Title: "Projects"})
	folder.Run()
	if folder.HasErr() {
		t.Fatal(folder.GetErr())
	}
	note := createnote.Process(ctx, tx, createnote.Input{Content: "# One", ParentID: folder.GetResult().ID})
	note.Run()
	removed := deleteitems.Process(ctx, tx, domain.FOLDER, []string{folder.GetResult().ID})
	removed.Run()
	if note.HasErr() || removed.HasErr() {
		t.Fatal(note.GetErr(), removed.GetErr())
	}

	command := &cobra.Command{}
	command.SetContext(ctx)
	printed, err := apitest.Stdout(t, func() error { return Restore(command, []string{folder.GetResult().ID}) })
	if err != nil || printed != "Successfully restored 2 item(s)\n" {
		t.Errorf("printed %q, err = %v; want 2 items", printed, err)
	}
	trash := listtrash.Process(ctx, tx)
	trash.Run()
	if trash.HasErr() || len(trash.GetResult()) != 0 {
		t.Errorf("trash = %+v, err = %v; want empty", trash.GetResult(), trash.GetErr())
	}
}
