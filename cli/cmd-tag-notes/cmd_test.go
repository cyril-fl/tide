package cmdtagnotes

import (
	"encoding/json"
	"errors"
	"testing"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/cyril-fl/tide/core/services/process/readitem"
	"github.com/spf13/cobra"
)

// Named notes are tagged and an unknown one reported; --dry-run on the filters of search prints each note's diff and
// writes nothing, then the real call tags what they find; notes with filters, no change and no target are refused.
func TestTag(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	ids := map[string]string{}
	for _, content := range []string{"# Plan\n#go #todo", "# Tarte\n#Cuisine", "# Soupe\n#Cuisine"} {
		note := createnote.Process(ctx, tx, createnote.Input{Content: content})
		note.Run()
		if note.HasErr() {
			t.Fatal(note.GetErr())
		}
		ids[string(note.GetResult().Title)] = note.GetResult().ID
	}

	run := func(flags map[string][]string, args ...string) (string, error) {
		command := &cobra.Command{}
		plumcliutilsargs.Makeflags(command, Flags())
		command.SetContext(ctx)
		for name, values := range flags {
			for _, value := range values {
				if err := command.Flags().Set(name, value); err != nil {
					t.Fatal(err)
				}
			}
		}
		return apitest.Stdout(t, func() error { return Tag(command, args) })
	}
	content := func(title string) string {
		read := readitem.Process(ctx, tx, domain.NOTE, ids[title])
		read.Run()
		if read.HasErr() {
			t.Fatal(read.GetErr())
		}
		return read.GetResult().Content
	}

	printed, err := run(map[string][]string{"add": {"done", "GO"}, "remove": {"todo", "nope"}}, "/Plan", "/Nowhere")
	want := "/Plan\n  added: done\n  already there: GO\n  removed: todo\n  absent: nope\n/Nowhere\n  error: "
	if err != nil || len(printed) < len(want) || printed[:len(want)] != want || content("Plan") != "# Plan\n#go #done" {
		t.Errorf("named: printed %q, err = %v, content %q", printed, err, content("Plan"))
	}

	printed, err = run(map[string][]string{"tag": {"cuisine"}, "add": {"recette"}, "dry-run": {"true"}, "json": {"true"}})
	var tagged []apiclient.TagnotesTagged
	if err != nil || json.Unmarshal([]byte(printed), &tagged) != nil || len(tagged) != 2 || len(apiclient.Deref(tagged[0].Diff)) == 0 || content("Tarte") != "# Tarte\n#Cuisine" {
		t.Errorf("dry run: printed %q, err = %v, content %q", printed, err, content("Tarte"))
	}
	printed, err = run(map[string][]string{"text": {"soupe"}, "add": {"recette"}})
	if err != nil || printed != "/Soupe\n  added: recette\n1 changed, 0 unchanged, 0 failed\n" || content("Soupe") != "# Soupe\n#Cuisine #recette" {
		t.Errorf("by text: printed %q, err = %v, content %q", printed, err, content("Soupe"))
	}

	for _, refused := range []struct {
		flags map[string][]string
		args  []string
		want  error
	}{
		{map[string][]string{"tag": {"cuisine"}, "add": {"x"}}, []string{"/Plan"}, apiclient.ErrNotesOrFilters},
		{map[string][]string{}, []string{"/Plan"}, errNoChange},
		{map[string][]string{"add": {"x"}}, nil, errNoTarget},
		{map[string][]string{"tag": {"nope"}, "add": {"x"}}, nil, apiclient.ErrNoTag},
	} {
		if _, err := run(refused.flags, refused.args...); !errors.Is(err, refused.want) {
			t.Errorf("%v %v: err = %v, want %v", refused.flags, refused.args, err, refused.want)
		}
	}
}
