package cmdcreatenote

import (
	"slices"
	"strings"
	"testing"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	"github.com/cyril-fl/tide/notes-go/app/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/notes-go/app/shared/domain"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createfolder"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/readitem"
	"github.com/cyril-fl/tide/notes-go/config"
	"github.com/spf13/cobra"
)

// The flags give everything, so no form opens: the note is created through the API, in its folder, with its tags.
func TestCreate(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	folder := createfolder.Process(ctx, tx, createfolder.Input{Title: "Projects"})
	folder.Run()
	if folder.HasErr() {
		t.Fatal(folder.GetErr())
	}

	command := &cobra.Command{}
	plumcliutilsargs.Makeflags(command, config.Current.Commands.CreateNote.Flags)
	command.SetContext(ctx)
	if err := command.ParseFlags([]string{"--content", "# Ideas", "--tag", "go", "--parent", folder.GetResult().ID}); err != nil {
		t.Fatal(err)
	}
	printed, err := apitest.Stdout(t, func() error { return Create(command, nil) })
	id := strings.TrimPrefix(strings.TrimSpace(printed), "Successfully created note ")
	if err != nil || id == strings.TrimSpace(printed) {
		t.Fatalf("printed %q, err = %v", printed, err)
	}

	note := readitem.Process(ctx, tx, domain.NOTE, id)
	note.Run()
	if note.HasErr() || note.GetResult().Path != "/Projects/Ideas" || !slices.Equal(note.GetResult().Tags, []string{"go"}) {
		t.Errorf("note = %+v, err = %v; want /Projects/Ideas tagged go", note.GetResult(), note.GetErr())
	}
}
