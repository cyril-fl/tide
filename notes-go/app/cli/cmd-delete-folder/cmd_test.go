package cmddeletefolder

import (
	"errors"
	"testing"

	"github.com/cyril-fl/tide/notes-go/app/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/notes-go/app/shared/domain"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createfolder"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createnote"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/readitem"
	"github.com/spf13/cobra"
)

// The folder and its content go to the trash through the API; an unknown uuid is skipped.
func TestDelete(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	folder := createfolder.Process(ctx, tx, createfolder.Input{Title: "Projects"})
	folder.Run()
	if folder.HasErr() {
		t.Fatal(folder.GetErr())
	}
	note := createnote.Process(ctx, tx, createnote.Input{Content: "# Ideas", ParentID: folder.GetResult().ID})
	note.Run()
	if note.HasErr() {
		t.Fatal(note.GetErr())
	}

	command := &cobra.Command{}
	command.SetContext(ctx)
	printed, err := apitest.Stdout(t, func() error {
		return Delete(command, []string{folder.GetResult().ID, "00000000-0000-0000-0000-000000000000"})
	})
	if err != nil || printed != "Successfully deleted 2 item(s): folders and their content\n" {
		t.Errorf("printed %q, err = %v; want the folder and its note", printed, err)
	}
	inside := readitem.Process(ctx, tx, domain.NOTE, note.GetResult().ID)
	inside.Run()
	if !errors.Is(inside.GetErr(), readitem.ErrNotFound) {
		t.Errorf("the note inside: err = %v, want %v", inside.GetErr(), readitem.ErrNotFound)
	}
}
