package cmdcreatefolder

import (
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

// The flags give everything, so no form opens: the folder is created through the API, in its parent; the same title
// there again is refused.
func TestCreate(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	parent := createfolder.Process(ctx, tx, createfolder.Input{Title: "Projects"})
	parent.Run()
	if parent.HasErr() {
		t.Fatal(parent.GetErr())
	}

	create := func() (string, error) {
		command := &cobra.Command{}
		plumcliutilsargs.Makeflags(command, config.Current.Commands.CreateFolder.Flags)
		command.SetContext(ctx)
		if err := command.ParseFlags([]string{"--title", "Go", "--parent", parent.GetResult().ID}); err != nil {
			t.Fatal(err)
		}
		return apitest.Stdout(t, func() error { return Create(command, nil) })
	}
	printed, err := create()
	id := strings.TrimPrefix(strings.TrimSpace(printed), "Successfully created folder ")
	if err != nil || id == strings.TrimSpace(printed) {
		t.Fatalf("printed %q, err = %v", printed, err)
	}
	folder := readitem.Process(ctx, tx, domain.FOLDER, id).SkipChildren()
	folder.Run()
	if folder.HasErr() || folder.GetResult().Path != "/Projects/Go" {
		t.Errorf("folder = %+v, err = %v; want /Projects/Go", folder.GetResult(), folder.GetErr())
	}

	if _, err := create(); err == nil || !strings.HasPrefix(err.Error(), domain.ErrFolderExists.Error()) {
		t.Errorf("same title: err = %v, want %v", err, domain.ErrFolderExists)
	}
}
