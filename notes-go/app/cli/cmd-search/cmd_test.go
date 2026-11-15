package cmdsearch

import (
	"encoding/json"
	"testing"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	"github.com/cyril-fl/tide/notes-go/app/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createnote"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/searchnotes"
	"github.com/cyril-fl/tide/notes-go/config"
	"github.com/spf13/cobra"
)

// The matches come through the API, --limit of them, with the total past it.
func TestSearch(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	for _, content := range []string{"# Tarte\nrecette", "# Pain\nrecette", "# Other"} {
		note := createnote.Process(ctx, tx, createnote.Input{Content: content})
		note.Run()
		if note.HasErr() {
			t.Fatal(note.GetErr())
		}
	}

	command := &cobra.Command{}
	plumcliutilsargs.Makeflags(command, config.Current.Commands.Search.Flags)
	command.SetContext(ctx)
	if err := command.Flags().Set("json", "true"); err != nil {
		t.Fatal(err)
	}
	printed, err := apitest.Stdout(t, func() error { return Search(command, []string{"recette"}) })
	var matches []searchnotes.Match
	if err != nil || json.Unmarshal([]byte(printed), &matches) != nil || len(matches) != 2 {
		t.Errorf("printed %q, err = %v; want the 2 notes as JSON", printed, err)
	}

	page, err := searchPage(ctx, "recette", 1)
	if err != nil || len(page.Rows) != 1 || page.Info != "1 of about 2 notes (--limit)" {
		t.Errorf("page = %+v, err = %v; want 1 of 2", page, err)
	}
}
