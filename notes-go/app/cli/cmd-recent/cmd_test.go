package cmdrecent

import (
	"encoding/json"
	"testing"

	"github.com/cyril-fl/notes/notes-go/app/cli/services/apiclient/apitest"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/createnote"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/listrecent"
	"github.com/cyril-fl/notes/notes-go/config"
	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	"github.com/spf13/cobra"
)

// The last changed notes come through the API, as JSON and as the navigator's page.
func TestRecent(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	note := createnote.Process(ctx, tx, createnote.Input{Content: "# Fresh"})
	note.Run()
	if note.HasErr() {
		t.Fatal(note.GetErr())
	}

	command := &cobra.Command{}
	plumcliutilsargs.Makeflags(command, config.Current.Commands.Recent.Flags)
	command.SetContext(ctx)
	if err := command.Flags().Set("json", "true"); err != nil {
		t.Fatal(err)
	}
	printed, err := apitest.Stdout(t, func() error { return Recent(command, nil) })
	var notes []listrecent.Recent
	if err != nil || json.Unmarshal([]byte(printed), &notes) != nil || len(notes) != 1 || notes[0].Title != "Fresh" {
		t.Errorf("printed %q, err = %v; want the note as JSON", printed, err)
	}

	page, err := recentPage(ctx)
	if err != nil || page.Info != "1 note" || len(page.Rows) != 1 || page.Rows[0].Target.Key != note.GetResult().ID {
		t.Errorf("page = %+v, err = %v; want the note", page, err)
	}
}
