package cmddeletenote

import (
	"testing"

	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient/apitest"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/createfolder"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/createnote"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/listtrash"
	"github.com/spf13/cobra"
)

// The notes go to the trash through the API; an unknown uuid and a folder are skipped and left out of the count.
func TestDelete(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	first := createnote.Process(ctx, tx, createnote.Input{Content: "# One"})
	first.Run()
	second := createnote.Process(ctx, tx, createnote.Input{Content: "# Two"})
	second.Run()
	folder := createfolder.Process(ctx, tx, createfolder.Input{Title: "Projects"})
	folder.Run()
	if first.HasErr() || second.HasErr() || folder.HasErr() {
		t.Fatal(first.GetErr(), second.GetErr(), folder.GetErr())
	}

	command := &cobra.Command{}
	command.SetContext(ctx)
	args := []string{first.GetResult().ID, "00000000-0000-0000-0000-000000000000", folder.GetResult().ID, second.GetResult().ID}
	printed, err := apitest.Stdout(t, func() error { return Delete(command, args) })
	if err != nil || printed != "Successfully deleted 2 note(s)\n" {
		t.Errorf("printed %q, err = %v; want 2 notes", printed, err)
	}
	trash := listtrash.Process(ctx, tx)
	trash.Run()
	if trash.HasErr() || len(trash.GetResult()) != 2 {
		t.Errorf("trash = %+v, err = %v; want the 2 notes", trash.GetResult(), trash.GetErr())
	}
}
