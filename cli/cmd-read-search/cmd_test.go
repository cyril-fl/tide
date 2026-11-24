package cmdreadsearch

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	plumcliconfig "github.com/cyril-fl/plum/cli/config"
	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	cmdcreatesearch "github.com/cyril-fl/tide/cli/cmd-create-search"
	cmddeletesearch "github.com/cyril-fl/tide/cli/cmd-delete-search"
	cmdeditsearch "github.com/cyril-fl/tide/cli/cmd-edit-search"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/config"
	"github.com/cyril-fl/tide/core/services/process/createfolder"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/cyril-fl/tide/core/services/process/deletetag"
	"github.com/spf13/cobra"
)

// add search saves the criteria by name and path, show search lists and runs it, edit search changes only the flags
// given, remove search deletes it; a plan without saved searches is refused.
func TestSearches(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	folder := createfolder.Process(ctx, tx, createfolder.Input{Title: "Archives"})
	folder.Run()
	if folder.HasErr() {
		t.Fatal(folder.GetErr())
	}
	for _, content := range []string{"# Old\n#go", "# Other\n#go"} {
		note := createnote.Process(ctx, tx, createnote.Input{Content: content, ParentID: folder.GetResult().ID})
		if note.Run(); note.HasErr() {
			t.Fatal(note.GetErr())
		}
	}
	exec := func(flags []plumcliconfig.Cmdflags, do func(*cobra.Command, []string) error, args []string, parse ...string) (string, error) {
		t.Helper()
		command := &cobra.Command{}
		plumcliutilsargs.Makeflags(command, flags)
		if err := command.ParseFlags(parse); err != nil {
			t.Fatal(err)
		}
		command.SetContext(ctx)
		return apitest.Stdout(t, func() error { return do(command, args) })
	}
	commands := config.Current.Commands

	printed, err := exec(commands.CreateSearch.Flags, cmdcreatesearch.Create, []string{"Go"}, "--tag", "#go", "--folder", "/Archives", "--created-from", "2000-01-31", "--json")
	var saved apiclient.SavedSearch
	if err == nil {
		err = json.Unmarshal([]byte(printed), &saved)
	}
	if err != nil || saved.Name != "Go" || !slices.Equal(saved.TagsAll, []string{"go"}) || !slices.Equal(saved.Folders, []string{"/Archives"}) ||
		!strings.HasPrefix(saved.CreatedFrom, "2000-01-31T00:00:00") {
		t.Fatalf("add = %s, %v", printed, err)
	}

	if printed, err := exec(commands.ReadSearch.Flags, Read, nil); err != nil || !strings.Contains(printed, "Go") || !strings.Contains(printed, "all of go") {
		t.Errorf("show = %q, %v", printed, err)
	}
	printed, err = exec(commands.ReadSearch.Flags, Read, []string{"go"}, "--limit", "1", "--json")
	var got run
	if err == nil {
		err = json.Unmarshal([]byte(printed), &got)
	}
	if err != nil || got.Search.ID != saved.ID || got.Total != 2 || len(got.Notes) != 1 {
		t.Errorf("show go --limit 1 --json = %s, %v", printed, err)
	}

	printed, err = exec(commands.EditSearch.Flags, cmdeditsearch.Edit, []string{"Go"}, "--name", "Golang", "--text", "Old", "--json")
	var edited apiclient.SavedSearch
	if err == nil {
		err = json.Unmarshal([]byte(printed), &edited)
	}
	if err != nil || edited.ID != saved.ID || edited.Name != "Golang" || edited.Text != "Old" || !slices.Equal(edited.TagsAll, []string{"go"}) || edited.CreatedFrom != saved.CreatedFrom {
		t.Errorf("edit = %s, %v; want the name and text changed, the rest kept", printed, err)
	}

	if _, err := exec(commands.DeleteSearch.Flags, cmddeletesearch.Delete, []string{"Golang"}); err != nil {
		t.Fatal(err)
	}
	if printed, err := exec(commands.ReadSearch.Flags, Read, nil); err != nil || !strings.Contains(printed, "No saved search") {
		t.Errorf("show after remove = %q, %v", printed, err)
	}

	apitest.Plan(t, ctx, tx, "base")
	if _, err := exec(commands.ReadSearch.Flags, Read, nil); !errors.Is(err, apiclient.ErrNoPro) {
		t.Errorf("base plan: err = %v, want ErrNoPro", err)
	}
}

// A saved search keeps the id of a tag deleted since; edit search saves it again as it is instead of refusing it as an
// unknown tag name.
func TestEditDeletedTag(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	note := createnote.Process(ctx, tx, createnote.Input{Content: "# Old\n#gone #kept"})
	if note.Run(); note.HasErr() {
		t.Fatal(note.GetErr())
	}
	exec := func(flags []plumcliconfig.Cmdflags, do func(*cobra.Command, []string) error, args []string, parse ...string) (string, error) {
		t.Helper()
		command := &cobra.Command{}
		plumcliutilsargs.Makeflags(command, flags)
		if err := command.ParseFlags(parse); err != nil {
			t.Fatal(err)
		}
		command.SetContext(ctx)
		return apitest.Stdout(t, func() error { return do(command, args) })
	}
	commands := config.Current.Commands
	if _, err := exec(commands.CreateSearch.Flags, cmdcreatesearch.Create, []string{"Mixed"}, "--tag", "gone,kept"); err != nil {
		t.Fatal(err)
	}
	gone := deletetag.Process(ctx, tx, []string{"gone"})
	if gone.Run(); gone.HasErr() {
		t.Fatal(gone.GetErr())
	}

	printed, err := exec(commands.EditSearch.Flags, cmdeditsearch.Edit, []string{"Mixed"}, "--name", "Renamed", "--json")
	var edited apiclient.SavedSearch
	if err == nil {
		err = json.Unmarshal([]byte(printed), &edited)
	}
	if err != nil || edited.Name != "Renamed" || len(edited.TagsAll) != 2 || !slices.Contains(edited.TagsAll, "kept") ||
		!slices.Contains(edited.TagsAll, gone.GetResult().Tags[0].ID) {
		t.Errorf("edit = %s, %v; want the name changed, kept by name and the deleted tag by its id", printed, err)
	}
}
