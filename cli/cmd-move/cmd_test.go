package cmdmove

import (
	"strings"
	"testing"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	"github.com/cyril-fl/tide/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/config"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/process/createfolder"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/cyril-fl/tide/core/services/process/moveitems"
	"github.com/cyril-fl/tide/core/services/process/readitem"
	"github.com/spf13/cobra"
)

// The note goes into the folder through the API, then back to the root without --parent; a folder into itself is
// refused.
func TestMove(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	folder := createfolder.Process(ctx, tx, createfolder.Input{Title: "Projects"})
	folder.Run()
	note := createnote.Process(ctx, tx, createnote.Input{Content: "# One"})
	note.Run()
	if folder.HasErr() || note.HasErr() {
		t.Fatal(folder.GetErr(), note.GetErr())
	}

	move := func(args ...string) (string, error) {
		command := &cobra.Command{}
		plumcliutilsargs.Makeflags(command, config.Current.Commands.Move.Flags)
		command.SetContext(ctx)
		if err := command.ParseFlags(args); err != nil {
			t.Fatal(err)
		}
		return apitest.Stdout(t, func() error { return Move(command, command.Flags().Args()) })
	}
	path := func() string {
		read := readitem.Process(ctx, tx, domain.NOTE, note.GetResult().ID)
		read.Run()
		return read.GetResult().Path
	}

	if printed, err := move(note.GetResult().ID, "--parent", folder.GetResult().ID); err != nil || printed != "Successfully moved 1 item(s)\n" || path() != "/Projects/One" {
		t.Errorf("printed %q, err = %v, path %q; want /Projects/One", printed, err, path())
	}
	if _, err := move(note.GetResult().ID); err != nil || path() != "/One" {
		t.Errorf("err = %v, path %q; want /One", err, path())
	}
	if _, err := move(folder.GetResult().ID, "--parent", folder.GetResult().ID); err == nil || !strings.HasPrefix(err.Error(), moveitems.ErrCycle.Error()) {
		t.Errorf("into itself: err = %v, want %v", err, moveitems.ErrCycle)
	}
}
