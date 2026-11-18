package cmdsearch

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/config"
	"github.com/cyril-fl/tide/core/services/process/createfolder"
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
	for _, content := range []string{"# Tarte\n#Idée #Cuisine", "# Punch\n#Idée #Boisson", "# Pain\n#Idée #Cuisine\nÉtapes", "# Soupe\n#Cuisine", "# Bœuf\nviande hachée et oignons\n## Étapes"} {
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
	if matches, err := search(map[string][]string{"empty-sections": {"true"}}); err != nil || len(matches) != 4 || matches[0].Title != "Bœuf" ||
		!slices.Equal(matches[0].EmptySections, []string{"## Étapes"}) {
		t.Errorf("--empty-sections = %+v, err = %v; want Bœuf and its ## Étapes, then the notes of a title alone but Pain", matches, err)
	}
	if _, err := search(map[string][]string{}); !errors.Is(err, errNoFilter) {
		t.Errorf("no text nor filter: err = %v, want errNoFilter", err)
	}
	if _, err := search(map[string][]string{"tag": {"Nowhere"}}); !errors.Is(err, apiclient.ErrNoTag) {
		t.Errorf("unknown tag: err = %v, want ErrNoTag", err)
	}
}

// --duplicates groups the notes sharing a title anywhere, --same-path those sharing a path, as text or JSON; a text,
// a filter or --limit beside them are refused.
func TestSearchDuplicates(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	folder := createfolder.Process(ctx, tx, createfolder.Input{Title: "Work"})
	folder.Run()
	if folder.HasErr() {
		t.Fatal(folder.GetErr())
	}
	for _, input := range []createnote.Input{{Content: "# Pizza\none"}, {Content: "# Pizza\ntwo"}, {Content: "# pizza\nthree", ParentID: folder.GetResult().ID}, {Content: "# Alone"}} {
		note := createnote.Process(ctx, tx, input)
		note.Run()
		if note.HasErr() {
			t.Fatal(note.GetErr())
		}
	}

	list := func(args []string, flags ...string) (string, error) {
		command := &cobra.Command{}
		plumcliutilsargs.Makeflags(command, config.Current.Commands.Search.Flags)
		command.SetContext(ctx)
		for i := 0; i < len(flags); i += 2 {
			if err := command.Flags().Set(flags[i], flags[i+1]); err != nil {
				t.Fatal(err)
			}
		}
		return apitest.Stdout(t, func() error { return Search(command, args) })
	}

	printed, err := list(nil, "duplicates", "true", "json", "true")
	var groups []apiclient.ListduplicatesGroup
	if err != nil || json.Unmarshal([]byte(printed), &groups) != nil || len(groups) != 1 || len(groups[0].Notes) != 3 {
		t.Errorf("--duplicates --json = %q, err = %v; want one group of 3 Pizza", printed, err)
	}
	printed, err = list(nil, "same-path", "true")
	if err != nil || !strings.HasPrefix(printed, "/Pizza\n") || strings.Count(printed, " /Pizza ") != 2 || strings.Contains(printed, "Work") {
		t.Errorf("--same-path = %q, err = %v; want the 2 notes of /Pizza", printed, err)
	}
	if _, err := list([]string{"pizza"}, "duplicates", "true"); !errors.Is(err, errListAlone) {
		t.Errorf("--duplicates with a text: err = %v, want errListAlone", err)
	}
	if _, err := list(nil, "same-path", "true", "limit", "5"); !errors.Is(err, errListAlone) {
		t.Errorf("--same-path --limit: err = %v, want errListAlone", err)
	}
}
