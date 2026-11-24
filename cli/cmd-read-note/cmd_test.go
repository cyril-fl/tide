package cmdreadnote

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	plumcliutilsargs "github.com/cyril-fl/plum/cli/utils/args"
	"github.com/cyril-fl/tide/cli/services/apiclient"
	"github.com/cyril-fl/tide/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/config"
	"github.com/cyril-fl/tide/core/services/process/createnote"
	"github.com/cyril-fl/tide/core/services/process/patchnote"
	"github.com/spf13/cobra"
)

// --json through the API prints the note flat, found by its path, its tags [] when it has none.
func TestReadJSON(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	tagged := createnote.Process(ctx, tx, createnote.Input{Content: "# Ideas\n\nship it #go"})
	tagged.Run()
	bare := createnote.Process(ctx, tx, createnote.Input{Content: "# Bare"})
	bare.Run()
	if tagged.HasErr() || bare.HasErr() {
		t.Fatal(tagged.GetErr(), bare.GetErr())
	}

	for ref, want := range map[string]noteJSON{
		"/Ideas": {ID: tagged.GetResult().ID, Title: "Ideas", Path: "/Ideas", Content: "# Ideas\n\nship it #go", Tags: []string{"go"}},
		"/Bare":  {ID: bare.GetResult().ID, Title: "Bare", Path: "/Bare", Content: "# Bare", Tags: []string{}},
	} {
		command := &cobra.Command{}
		command.Flags().Bool("json", true, "")
		command.Flags().Bool("history", false, "")
		command.Flags().String("version", "", "")
		command.Flags().Int("max-chars", 0, "")
		command.SetContext(ctx)
		printed, err := apitest.Stdout(t, func() error { return Read(command, []string{ref}) })
		var got noteJSON
		if err == nil {
			err = json.Unmarshal([]byte(printed), &got)
		}
		if err != nil || got.ID != want.ID || got.Title != want.Title || got.Path != want.Path || got.Content != want.Content ||
			!slices.Equal(got.Tags, want.Tags) || got.Tags == nil || got.Readonly || got.CreatedAt == "" || got.UpdatedAt != nil {
			t.Errorf("%s: printed %s, err = %v; want %+v", ref, printed, err, want)
		}
	}
}

// --history and --version through the API: the list of versions, then one with its text.
func TestReadHistory(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	seed := createnote.Process(ctx, tx, createnote.Input{Content: "# Ideas\n\nold"})
	seed.Run()
	if seed.HasErr() {
		t.Fatal(seed.GetErr())
	}
	content := "# Ideas\n\nnew"
	edit := patchnote.Process(ctx, tx, patchnote.Input{ID: seed.GetResult().ID, Content: &content, Restore: true})
	edit.Run()
	if edit.HasErr() {
		t.Fatal(edit.GetErr())
	}
	asJSON := true
	read := func(history bool, version string) string {
		command := &cobra.Command{}
		command.Flags().Bool("json", asJSON, "")
		command.Flags().Bool("history", history, "")
		command.Flags().String("version", version, "")
		command.Flags().Int("max-chars", 0, "")
		command.SetContext(ctx)
		printed, err := apitest.Stdout(t, func() error { return Read(command, []string{"/Ideas"}) })
		if err != nil {
			t.Fatal(err)
		}
		return printed
	}

	var versions []apiclient.DomainNoteVersion
	if err := json.Unmarshal([]byte(read(true, "")), &versions); err != nil || len(versions) != 1 || versions[0].Content != nil {
		t.Fatalf("--history: %+v, err = %v; want one version, its text left out", versions, err)
	}
	var version apiclient.DomainNoteVersion
	if err := json.Unmarshal([]byte(read(false, versions[0].Id)), &version); err != nil || apiclient.Deref(version.Content) != "# Ideas\n\nold" {
		t.Errorf("--version: %+v, err = %v; want the old text", version, err)
	}

	asJSON = false
	if got := read(true, ""); !strings.Contains(got, versions[0].Id) {
		t.Errorf("--history table = %q, want the version's id", got)
	}
	if got := read(false, versions[0].Id); !strings.Contains(got, "old") || strings.Contains(got, "new") {
		t.Errorf("--version text = %q, want the old text rendered", got)
	}

	apitest.Plan(t, ctx, tx, "base")
	command := &cobra.Command{}
	command.Flags().Bool("json", false, "")
	command.Flags().Bool("history", true, "")
	command.Flags().String("version", "", "")
	command.Flags().Int("max-chars", 0, "")
	command.SetContext(ctx)
	if _, err := apitest.Stdout(t, func() error { return Read(command, []string{"/Ideas"}) }); err == nil || !strings.Contains(err.Error(), "noteHistory") {
		t.Errorf("--history on the base plan: err = %v, want the plan refusal", err)
	}
}

func TestInfo(t *testing.T) {
	created := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	updated := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	tags := []string{"go", "sql"}
	locked := true

	tests := map[string]struct {
		note apiclient.ReaditemResult
		want string
	}{
		"tagged, edited, locked": {apiclient.ReaditemResult{Item: apiclient.DomainItem{CreatedAt: created, UpdatedAt: &updated}, Tags: &tags, Readonly: &locked}, "#go #sql · created 1 Nov 2026 · updated 2 Nov 2026 · locked"},
		"never edited":           {apiclient.ReaditemResult{Item: apiclient.DomainItem{CreatedAt: created}}, "created 1 Nov 2026"},
	}

	for name, test := range tests {
		if got := info(test.note); got != test.want {
			t.Errorf("%s: info() = %q, want %q", name, got, test.want)
		}
	}
}

// Several notes, or --max-chars, are read in one request: an array for several, an object for one, with the length
// and truncated that --max-chars adds; plain text prints them one after the other.
func TestReadSeveral(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	for _, content := range []string{"# Long\n\none two three four five six", "# Short"} {
		note := createnote.Process(ctx, tx, createnote.Input{Content: content})
		if note.Run(); note.HasErr() {
			t.Fatal(note.GetErr())
		}
	}
	both := []string{"/Long", "/Short"}
	read := func(refs []string, flags ...string) (string, error) {
		t.Helper()
		command := &cobra.Command{}
		plumcliutilsargs.Makeflags(command, config.Current.Commands.ReadNote.Flags)
		if err := command.ParseFlags(flags); err != nil {
			t.Fatal(err)
		}
		command.SetContext(ctx)
		return apitest.Stdout(t, func() error { return Read(command, refs) })
	}

	printed, err := read(both, "--max-chars", "12", "--json")
	var notes []noteJSON
	if err == nil {
		err = json.Unmarshal([]byte(printed), &notes)
	}
	if err != nil || len(notes) != 2 || notes[0].Path != "/Long" || !*notes[0].Truncated || *notes[0].Length != 35 || len(notes[0].Content) > 12 ||
		notes[1].Path != "/Short" || *notes[1].Truncated || notes[1].Content != "# Short" {
		t.Errorf("two notes --max-chars --json: printed %s, err = %v", printed, err)
	}

	printed, err = read([]string{"/Long"}, "--max-chars", "12", "--json")
	var one noteJSON
	if err == nil {
		err = json.Unmarshal([]byte(printed), &one)
	}
	if err != nil || one.Path != "/Long" || one.Truncated == nil || !*one.Truncated {
		t.Errorf("one note --max-chars --json: printed %s, err = %v", printed, err)
	}

	if printed, err := read(both); err != nil || !strings.Contains(printed, "/Long") || !strings.Contains(printed, "six") || !strings.Contains(printed, "/Short") {
		t.Errorf("two notes: printed %q, err = %v", printed, err)
	}
	if _, err := read(both, "--history"); !errors.Is(err, errOneNote) {
		t.Errorf("two notes --history: err = %v, want errOneNote", err)
	}
}
