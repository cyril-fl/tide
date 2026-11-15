package cmdeditnote

import (
	"slices"
	"testing"

	"github.com/cyril-fl/tide/notes-go/app/cli/services/apiclient/apitest"
	"github.com/cyril-fl/tide/notes-go/app/shared/domain"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/createnote"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/patchnote"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/readitem"
	"github.com/spf13/pflag"
)

// The CLI has no kernel to import ErrStale from: its text must stay the one the API answers.
func TestErrStale(t *testing.T) {
	if errStale != patchnote.ErrStale.Error() {
		t.Errorf("errStale = %q, want %q", errStale, patchnote.ErrStale.Error())
	}
}

// Interactive relies on it: a changed flag would skip the form.
func TestNoFlags(t *testing.T) {
	flags := noFlags()
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Changed {
			t.Errorf("--%s is changed", flag.Name)
		}
	})
	if flags.Lookup("content") == nil {
		t.Error("--content is not declared")
	}
}

// Through the API: --pin and --lock alone leave the text and its last change date, --tag rewrites them, and a locked
// note refuses it.
func TestEdit(t *testing.T) {
	ctx, tx := apitest.Serve(t)
	seed := createnote.Process(ctx, tx, createnote.Input{Content: "# Ideas"})
	seed.Run()
	if seed.HasErr() {
		t.Fatal(seed.GetErr())
	}
	id := seed.GetResult().ID
	edit := func(args ...string) (string, error) {
		flags := noFlags()
		if err := flags.Parse(args); err != nil {
			t.Fatal(err)
		}
		return apitest.Stdout(t, func() error { return run(ctx, id, flags) })
	}
	read := func() readitem.Result {
		note := readitem.Process(ctx, tx, domain.NOTE, id)
		note.Run()
		if note.HasErr() {
			t.Fatal(note.GetErr())
		}
		return note.GetResult()
	}

	printed, err := edit("--pin", "--lock")
	if note := read(); err != nil || !note.Pinned || !note.Readonly || note.Content != "# Ideas" || !note.Item.UpdatedAt.IsZero() {
		t.Errorf("--pin --lock: note = %+v, printed %q, err = %v; want pinned, locked, text untouched", note, printed, err)
	}
	if _, err := edit("--tag", "go"); err == nil {
		t.Error("--tag on a locked note: no error")
	}
	printed, err = edit("--unlock", "--tag", "go")
	if note := read(); err != nil || note.Readonly || !slices.Equal(note.Tags, []string{"go"}) || note.Item.UpdatedAt.IsZero() {
		t.Errorf("--unlock --tag go: note = %+v, printed %q, err = %v; want unlocked and tagged", note, printed, err)
	}
}
