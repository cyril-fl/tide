package cmdsearch

import (
	"encoding/json"
	"errors"
	"testing"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/config"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/cyril-fl/tide/core/services/process/searchnotes"
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

	page, err := searchPage(ctx, apiclient.SearchFilter{Text: "recette"}, 1)
	if err != nil || len(page.Rows) != 1 || page.Info != "1 of about 2 notes (--limit)" {
		t.Errorf("page = %+v, err = %v; want 1 of 2", page, err)
	}
}

// The filters go to the API by the ids of the tags, Idée AND (Cuisine OR Boisson) without Étapes; no text nor filter
// and an unknown tag are refused.
func TestSearchFilters(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	for _, content := range []string{"# Tarte\n#Idée #Cuisine", "# Punch\n#Idée #Boisson", "# Pain\n#Idée #Cuisine\nÉtapes", "# Soupe\n#Cuisine"} {
		note := createnote.Process(ctx, tx, createnote.Input{Content: content})
		note.Run()
		if note.HasErr() {
			t.Fatal(note.GetErr())
		}
	}

	search := func(flags map[string][]string, args ...string) ([]searchnotes.Match, error) {
		command := &cobra.Command{}
		plumcliutilsargs.Makeflags(command, config.Current.Commands.Search.Flags)
		command.SetContext(ctx)
		flags["json"] = []string{"true"}
		for name, values := range flags {
			for _, value := range values {
				if err := command.Flags().Set(name, value); err != nil {
					t.Fatal(err)
				}
			}
		}
		var matches []searchnotes.Match
		printed, err := apitest.Stdout(t, func() error { return Search(command, args) })
		if err == nil {
			err = json.Unmarshal([]byte(printed), &matches)
		}
		return matches, err
	}

	matches, err := search(map[string][]string{"tag": {"Idée"}, "any-tag": {"Cuisine", "#boisson"}, "not": {"étapes"}})
	titles := map[string]bool{}
	for _, match := range matches {
		titles[match.Title] = true
	}
	if err != nil || len(matches) != 2 || !titles["Tarte"] || !titles["Punch"] {
		t.Errorf("matches = %+v, err = %v; want Tarte and Punch", matches, err)
	}
	if matches, err := search(map[string][]string{"max-length": {"20"}}); err != nil || len(matches) != 1 || matches[0].Title != "Soupe" {
		t.Errorf("--max-length 20 = %+v, err = %v; want Soupe alone", matches, err)
	}
	if _, err := search(map[string][]string{}); !errors.Is(err, errNoFilter) {
		t.Errorf("no text nor filter: err = %v, want errNoFilter", err)
	}
	if _, err := search(map[string][]string{"tag": {"Nowhere"}}); !errors.Is(err, apiclient.ErrNoTag) {
		t.Errorf("unknown tag: err = %v, want ErrNoTag", err)
	}
}
