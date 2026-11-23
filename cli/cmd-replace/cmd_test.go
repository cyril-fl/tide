package cmdreplace

import (
	"strings"
	"testing"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	"github.com/cyril-fl/tide/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/config"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/process/createfolder"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/cyril-fl/tide/core/services/process/readitem"
	"github.com/spf13/cobra"
)

// --dry-run prints each note's occurrences and writes nothing, --summary --json only the counts; --folder narrows
// the real call to that folder's notes.
func TestReplace(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	folder := createfolder.Process(ctx, tx, createfolder.Input{Title: "Projects"})
	folder.Run()
	if folder.HasErr() {
		t.Fatal(folder.GetErr())
	}
	ids := map[string]string{}
	for _, input := range []createnote.Input{{Content: "# Plan\na teh b", ParentID: folder.GetResult().ID}, {Content: "# Inbox\nteh"}} {
		note := createnote.Process(ctx, tx, input)
		note.Run()
		if note.HasErr() {
			t.Fatal(note.GetErr())
		}
		ids[string(note.GetResult().Title)] = note.GetResult().ID
	}

	run := func(flags map[string]string) (string, error) {
		command := &cobra.Command{}
		plumcliutilsargs.Makeflags(command, config.Current.Commands.Replace.Flags)
		command.SetContext(ctx)
		for name, value := range flags {
			if err := command.Flags().Set(name, value); err != nil {
				t.Fatal(err)
			}
		}
		return apitest.Stdout(t, func() error { return Replace(command, []string{"teh", "the"}) })
	}
	content := func(title string) string {
		read := readitem.Process(ctx, tx, domain.NOTE, ids[title])
		read.Run()
		if read.HasErr() {
			t.Fatal(read.GetErr())
		}
		return read.GetResult().Content
	}

	printed, err := run(map[string]string{"dry-run": "true", "folder": "/Projects"})
	if want := "/Projects/Plan\n    2 - a teh b\n    2 + a the b\nWould replace 1 occurrence(s) in 1 note(s) (dry run, nothing written)\n"; err != nil || printed != want || content("Plan") != "# Plan\na teh b" {
		t.Errorf("dry run: printed %q, err = %v, content %q", printed, err, content("Plan"))
	}
	printed, err = run(map[string]string{"dry-run": "true", "summary": "true", "json": "true"})
	if err != nil || printed != "{\n  \"locked\": 0,\n  \"notes\": 2,\n  \"occurrences\": 2\n}\n" {
		t.Errorf("summary json: printed %q, err = %v", printed, err)
	}

	printed, err = run(map[string]string{"folder": "/Projects"})
	if err != nil || printed != "Successfully replaced 1 occurrence(s) in 1 note(s)\n" || content("Plan") != "# Plan\na the b" || content("Inbox") != "# Inbox\nteh" {
		t.Errorf("replace: printed %q, err = %v, contents %q %q", printed, err, content("Plan"), content("Inbox"))
	}
}

// replaceEverywhere is of the ultra plan: a pro account gets the API's refusal.
func TestReplaceRefused(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	apitest.Plan(t, ctx, tx, "pro")
	command := &cobra.Command{}
	plumcliutilsargs.Makeflags(command, config.Current.Commands.Replace.Flags)
	command.SetContext(ctx)
	if _, err := apitest.Stdout(t, func() error { return Replace(command, []string{"teh", "the"}) }); err == nil || !strings.Contains(err.Error(), "replaceEverywhere") {
		t.Errorf("err = %v, want the plan refusal", err)
	}
}
